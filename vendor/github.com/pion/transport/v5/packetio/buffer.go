// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package packetio provides packet buffer
package packetio

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/pion/transport/v5/deadline"
)

var errPacketTooBig = errors.New("packet too big")

// BufferPacketType allow the Buffer to know which packet protocol is writing.
type BufferPacketType int

const (
	// RTPBufferPacket indicates the Buffer that is handling RTP packets.
	RTPBufferPacket BufferPacketType = 1
	// RTCPBufferPacket indicates the Buffer that is handling RTCP packets.
	RTCPBufferPacket BufferPacketType = 2
)

// Attribute is a packet metadata entry.
type Attribute struct {
	Key, Value any
}

// Attributes is a reusable slice of packet metadata entries.
// Keys must be comparable.
type Attributes []Attribute

// Get returns the first value associated with key, or nil if it is absent.
func (a Attributes) Get(key any) any {
	for _, entry := range a {
		if entry.Key == key {
			return entry.Value
		}
	}

	return nil
}

// Set replaces the first entry for key, or appends an entry if key is absent.
func (a *Attributes) Set(key, value any) {
	for i := range *a {
		if (*a)[i].Key == key {
			(*a)[i].Value = value

			return
		}
	}
	*a = append(*a, Attribute{Key: key, Value: value})
}

// Buffer queues copies of packets without combining writes into a single read.
// Its methods may be called concurrently. Use NewBuffer to
// initialize it.
type Buffer struct {
	mutex sync.Mutex

	// this is a circular buffer.  If head <= tail, then the useful
	// data is in the interval [head, tail[.  If tail < head, then
	// the useful data is the union of [head, len[ and [0, tail[.
	// In order to avoid ambiguity when head = tail, we always leave
	// an unused byte in the buffer.
	data       []byte
	head, tail int

	notify chan struct{}
	closed bool

	count                 int
	limitCount, limitSize int
	attributes            []Attributes
	attributesHead        int

	readDeadline *deadline.Deadline
}

const (
	minSize    = 2048
	cutoffSize = 128 * 1024
	maxSize    = 4 * 1024 * 1024
	headerSize = 2
)

// NewBuffer creates a buffer for packets with optional attributes.
func NewBuffer() *Buffer {
	return &Buffer{
		notify:       make(chan struct{}, 1),
		readDeadline: deadline.New(),
	}
}

// available returns true if the buffer is large enough to fit a packet
// of the given size, taking overhead into account.
func (b *Buffer) available(size int) bool {
	available := b.head - b.tail
	if available <= 0 {
		available += len(b.data)
	}
	// we interpret head=tail as empty, so always keep a byte free
	return size+headerSize+1 <= available
}

// grow increases the size of the buffer.  If it returns nil, then the
// buffer has been grown.  It returns ErrFull if hits a limit.
func (b *Buffer) grow() error {
	var newSize int
	if len(b.data) < cutoffSize {
		newSize = 2 * len(b.data)
	} else {
		newSize = 5 * len(b.data) / 4
	}
	if newSize < minSize {
		newSize = minSize
	}
	if (b.limitSize <= 0 || sizeHardLimit) && newSize > maxSize {
		newSize = maxSize
	}

	// one byte slack
	if b.limitSize > 0 && newSize-1 > b.limitSize {
		newSize = b.limitSize + 1
	}

	if newSize <= len(b.data) {
		return ErrFull
	}

	newData := make([]byte, newSize)

	var n int
	if b.head <= b.tail {
		// data was contiguous
		n = copy(newData, b.data[b.head:b.tail])
	} else {
		// data was discontinuous
		n = copy(newData, b.data[b.head:])
		n += copy(newData[n:], b.data[:b.tail])
	}
	b.head = 0
	b.tail = n
	b.data = newData

	return nil
}

// Write copies the payload and attributes and queues them atomically.
// Pass nil for a packet without attributes. The payload can contain at most 65535
// bytes. Attribute keys and values are shallow-copied so referenced data must
// remain immutable while queued.
// The returned count includes payload bytes only.
// Returns ErrFull if the packet exceeds the queue's size or count limit, and
// io.ErrClosedPipe if the buffer is closed. Failed writes queue nothing.
func (b *Buffer) Write(payload []byte, attributes Attributes) (int, error) { //nolint:cyclop
	if len(payload) >= 0x10000 {
		return 0, errPacketTooBig
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.closed {
		return 0, io.ErrClosedPipe
	}

	size := len(payload) + headerSize
	limit := b.limitSize
	if limit <= 0 || (sizeHardLimit && limit >= maxSize) {
		limit = maxSize - 1
	}
	if (b.limitCount > 0 && b.count >= b.limitCount) ||
		size > limit-b.size() {
		return 0, ErrFull
	}

	// grow the buffer until the packet fits
	for !b.available(len(payload)) {
		if err := b.grow(); err != nil {
			return 0, err
		}
	}

	var header [headerSize]byte
	binary.BigEndian.PutUint16(header[:], uint16(len(payload))) //nolint:gosec // bounded above
	b.growAttributes()
	b.writeBytes(header[:])
	b.writeBytes(payload)
	tail := b.attributesHead + b.count
	if tail >= len(b.attributes) {
		tail -= len(b.attributes)
	}
	b.attributes[tail] = append(b.attributes[tail][:0], attributes...)
	b.count++
	b.notifyReader()

	return len(payload), nil
}

// Read consumes one packet into payloadBuf, replacing attributes with its metadata.
// Save the returned slice, its length may change, and if insufficient capacity, it allocates
// new storage. this avoids allocations when reading multiple packets with the same attributes length.
// A short payloadBuf consumes the packet and returns complete attributes with io.ErrShortBuffer.
// Read blocks until data, a deadline, or close; EOF and deadline errors return zero bytes
// and nil attributes. EOF is returned only after the closed buffer is drained.
func (b *Buffer) Read(payloadBuf []byte, attributes Attributes) (int, Attributes, error) {
	clear(attributes)
	// Return immediately if the deadline is already exceeded.
	select {
	case <-b.readDeadline.Done():
		return 0, nil, &netError{ErrTimeout, true, true}
	default:
	}

	for {
		b.mutex.Lock()

		if b.head != b.tail {
			n, attrs, err := b.consumePacket(payloadBuf, attributes)
			b.mutex.Unlock()

			return n, attrs, err
		}

		if b.closed {
			b.mutex.Unlock()

			return 0, nil, io.EOF
		}
		b.mutex.Unlock()

		select {
		case <-b.readDeadline.Done():
			return 0, nil, &netError{ErrTimeout, true, true}
		case <-b.notify:
		}
	}
}

// consumePacket requires the mutex and a nonempty buffer.
func (b *Buffer) consumePacket(payloadBuf []byte, attributes Attributes) (int, Attributes, error) {
	var header [headerSize]byte
	b.readBytes(header[:], headerSize)
	payloadSize := int(binary.BigEndian.Uint16(header[:]))
	n := b.readBytes(payloadBuf, payloadSize)
	queued := b.attributes[b.attributesHead]
	attributes = append(attributes[:0], queued...)
	clear(queued)
	b.attributes[b.attributesHead] = queued[:0]
	b.attributesHead++
	if b.attributesHead == len(b.attributes) {
		b.attributesHead = 0
	}
	b.count--
	if b.head == b.tail {
		b.head, b.tail = 0, 0
		b.attributesHead = 0
	} else if !b.closed {
		b.notifyReader()
	}

	if n < payloadSize {
		return n, attributes, io.ErrShortBuffer
	}

	return n, attributes, nil
}

// growAttributes requires the mutex.
func (b *Buffer) growAttributes() {
	if b.count < len(b.attributes) {
		return
	}
	attributes := make([]Attributes, max(1, 2*len(b.attributes)))
	n := copy(attributes, b.attributes[b.attributesHead:])
	copy(attributes[n:], b.attributes[:b.attributesHead])
	b.attributes = attributes
	b.attributesHead = 0
}

// writeBytes requires the mutex and enough free space in the ring.
func (b *Buffer) writeBytes(src []byte) {
	n := copy(b.data[b.tail:], src)
	b.tail += n
	if b.tail >= len(b.data) {
		b.tail = copy(b.data, src[n:])
	}
}

// readBytes copies as much of size as fits in dst and discards the remainder.
// It requires the mutex and at least size bytes in the ring.
func (b *Buffer) readBytes(dst []byte, size int) int {
	copied := min(len(dst), size)
	if b.head+copied <= len(b.data) {
		copy(dst, b.data[b.head:b.head+copied])
	} else {
		n := copy(dst[:copied], b.data[b.head:])
		copy(dst[n:copied], b.data[:copied-n])
	}
	b.head += size
	if b.head >= len(b.data) {
		b.head -= len(b.data)
	}

	return copied
}

// notifyReader requires the mutex and an open buffer.
func (b *Buffer) notifyReader() {
	select {
	case b.notify <- struct{}{}:
	default:
	}
}

// Close the buffer, unblocking any pending reads.
// Queued packets remain readable; reads return io.EOF only when empty.
func (b *Buffer) Close() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if !b.closed {
		b.closed = true
		close(b.notify)
	}

	return nil
}

// Count returns the number of packets in the buffer.
func (b *Buffer) Count() int {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.count
}

// SetLimitCount controls the maximum number of packets that can be buffered.
// Causes writes to return ErrFull when this limit is reached.
// A zero value will disable this limit.
func (b *Buffer) SetLimitCount(limit int) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.limitCount = limit
}

// Size returns queued payload bytes plus two framing bytes per packet.
// It measures logical packet bytes. attributes and spare capacity
// are excluded.
func (b *Buffer) Size() int {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.size()
}

func (b *Buffer) size() int {
	size := b.tail - b.head
	if size < 0 {
		size += len(b.data)
	}

	return size
}

// SetLimitSize controls the maximum Size. Writes that would exceed it return ErrFull.
// Attributes are excluded.
// A nonpositive limit uses the default capacity of 4 MiB, with one byte reserved.
//
// The packetioSizeHardlimit build tag caps the limit at the default capacity.
func (b *Buffer) SetLimitSize(limit int) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.limitSize = limit
}

// SetReadDeadline sets the deadline for reads, including pending reads.
// Setting to zero means no deadline.
func (b *Buffer) SetReadDeadline(t time.Time) error {
	b.readDeadline.Set(t)

	return nil
}

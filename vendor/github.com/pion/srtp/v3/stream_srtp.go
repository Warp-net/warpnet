// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package srtp

import (
	"errors"
	"io"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/transport/v5/deadline"
	"github.com/pion/transport/v5/packetio"
)

// Limit the buffer size to 1MB.
const srtpBufferSize = 1000 * 1000

// packetBuffer wraps packetio.Buffer to implement io.ReadWriteCloser.
type packetBuffer struct {
	*packetio.Buffer
}

func (b *packetBuffer) Read(buf []byte) (int, error) {
	n, _, err := b.Buffer.Read(buf, nil)

	return n, err
}

func (b *packetBuffer) Write(buf []byte) (int, error) {
	return b.Buffer.Write(buf, nil)
}

func (b *packetBuffer) ReadWithAttributes(buf []byte, attrs packetio.Attributes) (int, packetio.Attributes, error) {
	return b.Buffer.Read(buf, attrs)
}

func (b *packetBuffer) WriteWithAttributes(buf []byte, attrs packetio.Attributes) (int, error) {
	return b.Buffer.Write(buf, attrs)
}

type peekedPacket struct {
	payload    []byte
	attributes packetio.Attributes
}

// ReadStreamSRTP receives decrypted RTP for an SSRC and its optional RTX SSRC.
type ReadStreamSRTP struct {
	mu sync.Mutex

	isClosed bool

	session  *SessionSRTP
	ssrc     uint32
	isInited bool

	rtxSSRC   *uint32
	rtxStream atomic.Pointer[ReadStreamSRTP]
	primary   atomic.Pointer[ReadStreamSRTP]

	pending      int
	notify       chan struct{}
	readDeadline *deadline.Deadline

	buffer        io.ReadWriteCloser
	peekedPackets []peekedPacket
}

// Used by getOrCreateReadStream.
func newReadStreamSRTP() readStream {
	return &ReadStreamSRTP{}
}

func (r *ReadStreamSRTP) init(child streamSession, ssrc uint32) error {
	sessionSRTP, ok := child.(*SessionSRTP)

	r.mu.Lock()
	defer r.mu.Unlock()

	if !ok {
		return errFailedTypeAssertion
	} else if r.isInited {
		return errStreamAlreadyInited
	}

	r.session = sessionSRTP
	r.ssrc = ssrc
	r.isInited = true
	r.notify = make(chan struct{}, 1)
	r.readDeadline = deadline.New()

	// Create a buffer with a 1MB limit
	if r.session.bufferFactory != nil {
		r.buffer = r.session.bufferFactory(packetio.RTPBufferPacket, ssrc)
	} else {
		buff := &packetBuffer{Buffer: packetio.NewBuffer()}
		buff.SetLimitSize(srtpBufferSize)
		r.buffer = buff
	}

	return nil
}

func (r *ReadStreamSRTP) write(buf []byte, attrs packetio.Attributes) error {
	// Notify before Write, so custom buffers with blocking writes can be read.
	r.mu.Lock()
	r.pending++
	r.mu.Unlock()
	r.notifyReader()
	if primary := r.primary.Load(); primary != nil {
		primary.notifyReader()
	}
	_, err := writeWithAttributes(r.buffer, buf, attrs)
	if err != nil {
		r.consumed()
	}
	if errors.Is(err, packetio.ErrFull) {
		return nil
	}

	return err
}

func (r *ReadStreamSRTP) notifyReader() {
	select {
	case r.notify <- struct{}{}:
	default:
	}
}

func (r *ReadStreamSRTP) consumed() {
	r.mu.Lock()
	if r.pending > 0 {
		r.pending--
	}
	r.mu.Unlock()
}

// NextStream waits for a packet on this stream or its associated RTX stream,
// without consuming it. Read the returned stream's SourceReader to consume it.
// Reads and source selection must be serialized by the caller.
func (r *ReadStreamSRTP) NextStream() (*ReadStreamSRTP, error) {
	for {
		select {
		case <-r.readDeadline.Done():
			return nil, os.ErrDeadlineExceeded
		default:
		}
		// Service queued repair packets before waiting for more primary RTP.
		if repair := r.rtxStream.Load(); repair != nil {
			repair.mu.Lock()
			ready := repair.pending > 0
			repair.mu.Unlock()
			if ready {
				return repair, nil
			}
		}
		r.mu.Lock()
		ready, closed := r.pending > 0, r.isClosed
		r.mu.Unlock()
		if ready {
			return r, nil
		}
		if closed {
			return nil, io.EOF
		}
		select {
		case <-r.notify:
		case <-r.readDeadline.Done():
			return nil, os.ErrDeadlineExceeded
		}
	}
}

// SourceReader returns a reader for only this stream's SSRC, excluding RTX.
// It consumes the same buffered packets as Read, including packets saved by Peek.
// Reads through these handles must be serialized by the caller.
func (r *ReadStreamSRTP) SourceReader() io.Reader {
	return sourceReader{r}
}

type sourceReader struct {
	stream *ReadStreamSRTP
}

func (r sourceReader) Read(buf []byte) (int, error) {
	n, _, err := r.stream.readPacket(buf, nil)

	return n, err
}

// Peek reads and decrypts full RTP packet from the nextConn.
// It is then buffered so that a call to `Read` will return it.
func (r *ReadStreamSRTP) Peek(buf []byte) (n int, err error) {
	var attrs packetio.Attributes
	n, attrs, err = readWithAttributes(r.buffer, buf, nil)
	if err == nil {
		r.peekedPackets = append(r.peekedPackets, peekedPacket{slices.Clone(buf[:n]), attrs})
	} else if errors.Is(err, io.ErrShortBuffer) {
		r.consumed()
	}

	return
}

// Read reads and decrypts full RTP packet from the nextConn.
func (r *ReadStreamSRTP) Read(buf []byte) (int, error) {
	n, _, err := r.ReadWithAttributes(buf, nil)

	return n, err
}

// ReadWithAttributes reads a decrypted RTP packet and its attributes.
// It replaces attrs with the packet's attributes, reusing its storage when possible.
func (r *ReadStreamSRTP) ReadWithAttributes(buf []byte, attrs packetio.Attributes) (int, packetio.Attributes, error) {
	stream, err := r.NextStream()
	if err != nil {
		return 0, nil, err
	}

	return stream.readPacket(buf, attrs)
}

func (r *ReadStreamSRTP) readPacket(buf []byte, attrs packetio.Attributes) (int, packetio.Attributes, error) {
	if len(r.peekedPackets) != 0 {
		clear(attrs)
		packet := r.peekedPackets[0]
		if len(packet.payload) > len(buf) {
			return 0, attrs[:0], io.ErrShortBuffer
		}

		n := copy(buf, packet.payload)
		attrs = append(attrs[:0], packet.attributes...)
		r.peekedPackets[0] = peekedPacket{}
		r.peekedPackets = r.peekedPackets[1:]
		r.consumed()

		return n, attrs, nil
	}

	n, attrs, err := readWithAttributes(r.buffer, buf, attrs)
	if err == nil || errors.Is(err, io.ErrShortBuffer) {
		r.consumed()
	}

	return n, attrs, err
}

// ReadRTP reads and decrypts full RTP packet and its header from the nextConn.
func (r *ReadStreamSRTP) ReadRTP(buf []byte) (int, *rtp.Header, error) {
	n, err := r.Read(buf)
	if err != nil {
		return 0, nil, err
	}

	header := &rtp.Header{}

	_, err = header.Unmarshal(buf[:n])
	if err != nil {
		return 0, nil, err
	}

	return n, header, nil
}

// SetReadDeadline sets the deadline for the Read operation.
// Setting to zero means no deadline.
func (r *ReadStreamSRTP) SetReadDeadline(t time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.setReadDeadline(t); err != nil {
		return err
	}
	if repair := r.rtxStream.Load(); repair != nil {
		return repair.setReadDeadline(t)
	}

	return nil
}

func (r *ReadStreamSRTP) setReadDeadline(t time.Time) error {
	r.readDeadline.Set(t)
	if b, ok := r.buffer.(interface {
		SetReadDeadline(time.Time) error
	}); ok {
		return b.SetReadDeadline(t)
	}

	return nil
}

// SetRTX routes one repair SSRC to this stream without rewriting its packets.
// Authentication and replay protection remain separate for each SSRC.
// Repeating the same SSRC is a no-op; changing it is not supported.
// If the repair stream already exists, stop reading it (including Peek) first.
// Both sources retain their own buffers. Reads pull from either source without
// starting a forwarding goroutine. The primary read deadline applies to both.
// Closing this stream also closes the repair stream.
func (r *ReadStreamSRTP) SetRTX(ssrc uint32) error { //nolint:cyclop
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isInited {
		return errStreamNotInited
	}
	session := r.session
	session.readStreamsLock.Lock()
	defer session.readStreamsLock.Unlock()

	if session.readStreamsClosed || session.readStreams[r.ssrc] != r {
		return errStreamAlreadyClosed
	}
	if r.rtxSSRC != nil && *r.rtxSSRC == ssrc {
		return nil
	}
	if r.primary.Load() != nil || r.rtxSSRC != nil || ssrc == r.ssrc {
		return errStreamAlreadyInited
	}
	existing := session.readStreams[ssrc]
	rtx, ok := existing.(*ReadStreamSRTP)
	if existing != nil && (!ok || rtx.primary.Load() != nil || rtx.rtxSSRC != nil) {
		return errStreamAlreadyInited
	}
	if rtx == nil {
		rtx = &ReadStreamSRTP{}
		if err := rtx.init(session, ssrc); err != nil {
			return err
		}
		session.readStreams[ssrc] = rtx
	}
	readDeadline, _ := r.readDeadline.Deadline()
	if err := rtx.setReadDeadline(readDeadline); err != nil {
		return err
	}
	r.rtxSSRC = &ssrc
	rtx.primary.Store(r)
	r.rtxStream.Store(rtx)
	r.notifyReader()

	return nil
}

// Close removes the ReadStream from the session and cleans up any associated state.
func (r *ReadStreamSRTP) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isInited {
		return errStreamNotInited
	}

	if r.isClosed {
		return nil
	}

	err := r.buffer.Close()
	if err != nil {
		return err
	}
	if repair := r.rtxStream.Load(); repair != nil {
		if err = repair.Close(); err != nil {
			return err
		}
	}

	r.session.removeReadStream(r.ssrc)
	r.isClosed = true
	r.notifyReader()

	return nil
}

// GetSSRC returns the SSRC we are demuxing for.
func (r *ReadStreamSRTP) GetSSRC() uint32 {
	return r.ssrc
}

// WriteStreamSRTP is stream for a single Session that is used to encrypt RTP.
type WriteStreamSRTP struct {
	session *SessionSRTP
}

// WriteRTP encrypts a RTP packet and writes to the connection.
func (w *WriteStreamSRTP) WriteRTP(header *rtp.Header, payload []byte) (int, error) {
	return w.session.writeRTP(header, payload)
}

// Write encrypts and writes a full RTP packets to the nextConn.
func (w *WriteStreamSRTP) Write(b []byte) (int, error) {
	return w.session.write(b)
}

// SetWriteDeadline sets the deadline for the Write operation.
// Setting to zero means no deadline.
func (w *WriteStreamSRTP) SetWriteDeadline(t time.Time) error {
	return w.session.setWriteDeadline(t)
}

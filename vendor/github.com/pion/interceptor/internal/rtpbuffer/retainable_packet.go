// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtpbuffer

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"
)

// RetainablePacket is a referenced counted RTP packet.
type RetainablePacket struct {
	onRelease func(*rtp.Header, *[]byte)

	countMu sync.Mutex
	count   int

	header  *rtp.Header
	buffer  *[]byte
	payload []byte

	sequenceNumber uint16
	lastResend     time.Time
	pending        bool
}

// BeginResend reserves a packet unless it is pending or was recently resent.
func (p *RetainablePacket) BeginResend(now time.Time, interval time.Duration) bool {
	p.countMu.Lock()
	defer p.countMu.Unlock()
	if p.pending || (!p.lastResend.IsZero() && now.Sub(p.lastResend) < interval) {
		return false
	}
	p.pending = true

	return true
}

// EndResend clears pending and records successful retransmissions.
func (p *RetainablePacket) EndResend(now time.Time, sent bool) {
	p.countMu.Lock()
	defer p.countMu.Unlock()
	p.pending = false
	if sent {
		p.lastResend = now
	}
}

// Header returns the RTP Header of the RetainablePacket.
func (p *RetainablePacket) Header() *rtp.Header {
	return p.header
}

// Payload returns the RTP Payload of the RetainablePacket.
func (p *RetainablePacket) Payload() []byte {
	return p.payload
}

// Retain increases the reference count of the RetainablePacket.
func (p *RetainablePacket) Retain() error {
	p.countMu.Lock()
	defer p.countMu.Unlock()
	if p.count == 0 {
		// already released
		return errPacketReleased
	}
	p.count++

	return nil
}

// Release decreases the reference count of the RetainablePacket and frees if needed.
func (p *RetainablePacket) Release() {
	p.countMu.Lock()
	defer p.countMu.Unlock()
	p.count--

	if p.count == 0 {
		// release back to pool
		p.onRelease(p.header, p.buffer)
		p.header = nil
		p.buffer = nil
		p.payload = nil
	}
}

// ResendKey identifies retransmission completion in interceptor attributes.
type ResendKey struct{}

// Resend tracks a retransmission through asynchronous writers.
type Resend struct {
	owners   atomic.Int32
	canceled atomic.Bool
	finish   func(bool)
}

// NewResend creates a resend with one owner and a completion callback.
func NewResend(complete func(bool)) *Resend {
	resend := &Resend{finish: complete}
	resend.owners.Store(1)

	return resend
}

// Retain adds an owner before passing the resend to another writer.
func (r *Resend) Retain() {
	if r != nil {
		r.owners.Add(1)
	}
}

// Release completes the resend when its last owner finishes.
func (r *Resend) Release(sent bool) {
	if r != nil && !sent {
		r.canceled.Store(true)
	}
	if r != nil && r.owners.Add(-1) == 0 {
		r.finish(!r.canceled.Load())
	}
}

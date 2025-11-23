package backoff

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p/core/peer"
)

const ErrBackoffEnabled warpnet.WarpError = "backoff enabled"

const (
	// BackoffCleanupInterval = 1 * time.Minute


	MinBackoffDelay = 100 * time.Millisecond
	MaxBackoffDelay = 5 * time.Minute
	TimeToLive      = 10 * time.Minute
	BasicBackoffMultiplier = 2
	MaxBackoffJitterCoff   = 100
	MaxBackoffAttempts     = 5
)

type backoffHistory struct {
	duration  time.Duration
	lastTried time.Time
	attempts  int
}

type backoff struct {
	mx          sync.Mutex
	history     map[peer.ID]backoffHistory
	interval    time.Duration
	maxAttempts int
}

func NewSimpleBackoff(ctx context.Context, cleanupInterval time.Duration, maxAttempts int) *backoff {
	b := &backoff{
		mx:          sync.Mutex{},
		interval:    cleanupInterval,
		maxAttempts: maxAttempts,
		history:     make(map[peer.ID]backoffHistory),
	}

	go b.cleanupLoop(ctx)

	return b
}

func (b *backoff) IsBackoffEnabled(id peer.ID) bool {
	b.mx.Lock()
	defer b.mx.Unlock()

	h, ok := b.history[id]
	switch {
	case !ok || time.Since(h.lastTried) > TimeToLive: // first request
		h = backoffHistory{
			duration: MinBackoffDelay,
			attempts: 0,
		}
	case time.Since(h.lastTried) < h.duration:
		return true
	case h.attempts >= b.maxAttempts:
		return true
	case h.duration < MaxBackoffDelay:
		jitter := rand.IntN(MaxBackoffJitterCoff) //#nosec
		h.duration = (BasicBackoffMultiplier * h.duration) + time.Duration(jitter)*time.Millisecond
		if h.duration > MaxBackoffDelay || h.duration < 0 {
			h.duration = MaxBackoffDelay
		}
	}

	h.attempts += 1
	h.lastTried = time.Now()
	b.history[id] = h
	return false
}

func (b *backoff) Reset(id peer.ID) {
	b.mx.Lock()
	delete(b.history, id)
	b.mx.Unlock()
}

func (b *backoff) cleanup() {
	b.mx.Lock()

	for id, h := range b.history {
		if time.Since(h.lastTried) > TimeToLive {
			delete(b.history, id)
		}
	}
	b.mx.Unlock()
}

func (b *backoff) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.cleanup()
		}
	}
}

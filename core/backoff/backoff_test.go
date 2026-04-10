package backoff

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
)

func TestNewSimpleBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := NewSimpleBackoff(ctx, time.Minute, 5)
	assert.NotNil(t, b)
	assert.Equal(t, 5, b.maxAttempts)
	assert.Equal(t, time.Minute, b.interval)
}

func TestIsBackoffEnabled_FirstRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := NewSimpleBackoff(ctx, time.Minute, 5)

	id := peer.ID("peer1")
	enabled := b.IsBackoffEnabled(id)
	assert.False(t, enabled, "first request should not be backoffed")
}

func TestIsBackoffEnabled_TooSoonAfterLastTry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := NewSimpleBackoff(ctx, time.Minute, 5)

	id := peer.ID("peer1")
	b.IsBackoffEnabled(id) // first call sets history

	enabled := b.IsBackoffEnabled(id)
	assert.True(t, enabled, "second immediate request should be backoffed")
}

func TestIsBackoffEnabled_MaxAttemptsReached(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := NewSimpleBackoff(ctx, time.Minute, 2)

	id := peer.ID("peer1")

	// Exhaust attempts: lastTried must be recent enough (within TTL)
	// and backoff duration must have passed
	b.mx.Lock()
	b.history[id] = backoffHistory{
		duration:  MinBackoffDelay,
		lastTried: time.Now().Add(-time.Second), // recent, within TTL, past backoff duration
		attempts:  2,
	}
	b.mx.Unlock()

	enabled := b.IsBackoffEnabled(id)
	assert.True(t, enabled, "should be backoffed when max attempts reached")
}

func TestReset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := NewSimpleBackoff(ctx, time.Minute, 5)

	id := peer.ID("peer1")
	b.IsBackoffEnabled(id) // first call
	b.IsBackoffEnabled(id) // second call (backoffed)

	b.Reset(id)

	enabled := b.IsBackoffEnabled(id)
	assert.False(t, enabled, "after reset, first request should not be backoffed")
}

func TestCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := NewSimpleBackoff(ctx, time.Minute, 5)

	id := peer.ID("peer1")
	b.mx.Lock()
	b.history[id] = backoffHistory{
		duration:  MinBackoffDelay,
		lastTried: time.Now().Add(-TimeToLive - time.Minute),
		attempts:  1,
	}
	b.mx.Unlock()

	b.cleanup()

	b.mx.Lock()
	_, exists := b.history[id]
	b.mx.Unlock()
	assert.False(t, exists, "expired entries should be cleaned up")
}

func TestCleanupLoop_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b := NewSimpleBackoff(ctx, 50*time.Millisecond, 5)

	_ = b // cleanup loop is already running
	cancel()
	// Give goroutine time to exit
	time.Sleep(100 * time.Millisecond)
}

func TestIsBackoffEnabled_ExpiredTTL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := NewSimpleBackoff(ctx, time.Minute, 5)

	id := peer.ID("peer1")
	b.mx.Lock()
	b.history[id] = backoffHistory{
		duration:  MinBackoffDelay,
		lastTried: time.Now().Add(-TimeToLive - time.Minute),
		attempts:  3,
	}
	b.mx.Unlock()

	enabled := b.IsBackoffEnabled(id)
	assert.False(t, enabled, "expired TTL should reset, so not backoffed")
}

func TestErrBackoffEnabled(t *testing.T) {
	assert.Equal(t, "backoff enabled", string(ErrBackoffEnabled))
}

package discovery

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewRateLimiter(t *testing.T) {
	rl := newRateLimiter(10, 2)
	assert.NotNil(t, rl)
	assert.Equal(t, int64(10), rl.capacity.Load())
	assert.Equal(t, int64(0), rl.remaining.Load())
}

func TestNewRateLimiter_ZeroLeakRate(t *testing.T) {
	rl := newRateLimiter(10, 0)
	assert.NotNil(t, rl)
	// leakPer10Sec should default to 1
	assert.Equal(t, (time.Second*10)/time.Duration(1), rl.leakInterval)
}

func TestNewRateLimiter_NegativeLeakRate(t *testing.T) {
	rl := newRateLimiter(10, -5)
	assert.NotNil(t, rl)
	assert.Equal(t, (time.Second*10)/time.Duration(1), rl.leakInterval)
}

func TestAllow_UnderCapacity(t *testing.T) {
	rl := newRateLimiter(10, 2)
	for range 10 {
		assert.True(t, rl.Allow())
	}
}

func TestAllow_OverCapacity(t *testing.T) {
	rl := newRateLimiter(5, 1)
	for range 5 {
		rl.Allow()
	}
	assert.False(t, rl.Allow(), "should be rate limited after capacity reached")
}

func TestAllow_LeaksOverTime(t *testing.T) {
	rl := newRateLimiter(2, 100) // high leak rate
	rl.Allow()
	rl.Allow()
	assert.False(t, rl.Allow(), "should be rate limited")

	// Simulate time passing by manipulating lastLeak
	rl.lastLeak.Store(time.Now().Add(-time.Second * 11).UnixMilli())

	assert.True(t, rl.Allow(), "should allow after leaking")
}

func TestAllow_RemainingNeverNegative(t *testing.T) {
	rl := newRateLimiter(5, 100)
	// Set lastLeak to long ago so many leaks would happen
	rl.lastLeak.Store(time.Now().Add(-time.Minute).UnixMilli())
	rl.Allow()
	assert.True(t, rl.remaining.Load() >= 0)
}

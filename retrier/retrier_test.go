package retrier

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetrierError(t *testing.T) {
	assert.Equal(t, "retrier: deadline reached", ErrDeadlineReached.Error())
	assert.Equal(t, "retrier: stop trying", ErrStopTrying.Error())
}

func TestNew(t *testing.T) {
	r := New(100*time.Millisecond, 3, NoBackoff)
	assert.NotNil(t, r)
}

func TestTry_SuccessOnFirstAttempt(t *testing.T) {
	r := New(10*time.Millisecond, 3, NoBackoff)
	calls := 0
	err := r.Try(context.Background(), func() error {
		calls++
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestTry_SuccessAfterRetries(t *testing.T) {
	r := New(10*time.Millisecond, 5, NoBackoff)
	calls := 0
	err := r.Try(context.Background(), func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestTry_DeadlineReached(t *testing.T) {
	r := New(10*time.Millisecond, 3, NoBackoff)
	calls := 0
	err := r.Try(context.Background(), func() error {
		calls++
		return errors.New("always fails")
	})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrDeadlineReached))
	assert.Equal(t, 3, calls)
}

func TestTry_StopTrying(t *testing.T) {
	r := New(10*time.Millisecond, 5, NoBackoff)
	calls := 0
	err := r.Try(context.Background(), func() error {
		calls++
		return ErrStopTrying
	})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrStopTrying))
	assert.Equal(t, 1, calls)
}

func TestTry_ContextCancelled(t *testing.T) {
	r := New(10*time.Millisecond, 10, NoBackoff)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := r.Try(ctx, func() error {
		return errors.New("should not reach")
	})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestTry_FixedBackoff(t *testing.T) {
	r := New(10*time.Millisecond, 3, FixedBackoff)
	calls := 0
	start := time.Now()
	err := r.Try(context.Background(), func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	elapsed := time.Since(start)
	assert.NoError(t, err)
	assert.Equal(t, 3, calls)
	assert.True(t, elapsed >= 20*time.Millisecond, "fixed backoff should add delay")
}

func TestTry_ExponentialBackoff(t *testing.T) {
	r := New(10*time.Millisecond, 3, ExponentialBackoff)
	calls := 0
	start := time.Now()
	err := r.Try(context.Background(), func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	elapsed := time.Since(start)
	assert.NoError(t, err)
	assert.Equal(t, 3, calls)
	assert.True(t, elapsed >= 20*time.Millisecond, "exponential backoff should add delay")
}

func TestJitter(t *testing.T) {
	d := jitter(100 * time.Millisecond)
	assert.True(t, d >= 0)
	assert.True(t, d < 50*time.Millisecond)
}

//nolint:all
package selfupdate

import (
	"context"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gateWait = 2 * time.Second

// waitPending blocks until a release shows up in the gate, so a test never races
// the goroutine that put it there.
func waitPending(t *testing.T, g *UpdateGate) domain.UpdateInfo {
	t.Helper()
	deadline := time.Now().Add(gateWait)
	for time.Now().Before(deadline) {
		if info := g.Pending(); info.NewVersion != "" {
			return info
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no release is waiting in the gate")
	return domain.UpdateInfo{}
}

func TestUpdateGateRoundTrip(t *testing.T) {
	for _, isAllowed := range []bool{true, false} {
		g := NewUpdateGate(context.Background())

		verdicts := make(chan bool, 1)
		go func() {
			verdicts <- g.IsUpdateAllowed(domain.UpdateInfo{
				CurrentVersion: "0.7.1",
				NewVersion:     "0.7.2",
			})
		}()

		pending := waitPending(t, g)
		assert.Equal(t, "0.7.1", pending.CurrentVersion)
		assert.Equal(t, "0.7.2", pending.NewVersion)

		g.Answer(isAllowed)

		select {
		case got := <-verdicts:
			assert.Equal(t, isAllowed, got)
		case <-time.After(gateWait):
			t.Fatal("the answer never reached the update service")
		}

		assert.Empty(t, g.Pending().NewVersion, "an answered release must stop waiting")
	}
}

func TestUpdateGateRefusesOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	g := NewUpdateGate(ctx)

	verdicts := make(chan bool, 1)
	go func() {
		verdicts <- g.IsUpdateAllowed(domain.UpdateInfo{NewVersion: "0.7.2"})
	}()

	waitPending(t, g)
	cancel()

	select {
	case got := <-verdicts:
		assert.False(t, got, "a node shutting down must not install anything")
	case <-time.After(gateWait):
		t.Fatal("shutdown left the update service waiting")
	}
}

// An answer nobody waits for must be dropped rather than kept for the next
// release: a stale dashboard tab would otherwise decide the following update.
func TestUpdateGateDropsUnexpectedAnswer(t *testing.T) {
	g := NewUpdateGate(context.Background())
	require.NotPanics(t, func() { g.Answer(true) })

	verdicts := make(chan bool, 1)
	go func() {
		verdicts <- g.IsUpdateAllowed(domain.UpdateInfo{NewVersion: "0.7.2"})
	}()

	waitPending(t, g)
	select {
	case <-verdicts:
		t.Fatal("the dropped answer was served to the next release")
	case <-time.After(100 * time.Millisecond):
	}

	g.Answer(false)
	select {
	case got := <-verdicts:
		assert.False(t, got)
	case <-time.After(gateWait):
		t.Fatal("the answer never reached the update service")
	}
}

func TestUpdateGateNilIsInert(t *testing.T) {
	var g *UpdateGate
	assert.False(t, g.IsUpdateAllowed(domain.UpdateInfo{NewVersion: "0.7.2"}))
	assert.Empty(t, g.Pending().NewVersion)
	assert.NotPanics(t, func() { g.Answer(true) })
}

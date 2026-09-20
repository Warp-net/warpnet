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

const approverWait = 2 * time.Second

func waitPendingUpdate(t *testing.T, a *UserApprover) domain.UpdateInfo {
	t.Helper()
	deadline := time.Now().Add(approverWait)
	for time.Now().Before(deadline) {
		if info := a.GetPendingUpdate(); info.NewVersion != "" {
			return info
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no release is waiting for an answer")
	return domain.UpdateInfo{}
}

func TestUserApproverRoundTrip(t *testing.T) {
	for _, isAllowed := range []bool{true, false} {
		approver := NewUserApprover(context.Background())

		verdicts := make(chan bool, 1)
		go func() {
			verdicts <- approver.IsUpdateAllowed(domain.UpdateInfo{
				CurrentVersion: "0.7.1",
				NewVersion:     "0.7.2",
			})
		}()

		pending := waitPendingUpdate(t, approver)
		assert.Equal(t, "0.7.1", pending.CurrentVersion)
		assert.Equal(t, "0.7.2", pending.NewVersion)

		approver.AnswerUpdate(isAllowed)

		select {
		case got := <-verdicts:
			assert.Equal(t, isAllowed, got)
		case <-time.After(approverWait):
			t.Fatal("the answer never reached the update service")
		}

		assert.Empty(t, approver.GetPendingUpdate().NewVersion, "an answered release must stop waiting")
	}
}

func TestUserApproverRefusesOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	approver := NewUserApprover(ctx)

	verdicts := make(chan bool, 1)
	go func() {
		verdicts <- approver.IsUpdateAllowed(domain.UpdateInfo{NewVersion: "0.7.2"})
	}()

	waitPendingUpdate(t, approver)
	cancel()

	select {
	case got := <-verdicts:
		assert.False(t, got, "a node shutting down must not install anything")
	case <-time.After(approverWait):
		t.Fatal("shutdown left the update service waiting")
	}
}

func TestUserApproverDropsUnexpectedAnswer(t *testing.T) {
	approver := NewUserApprover(context.Background())
	require.NotPanics(t, func() { approver.AnswerUpdate(true) })

	verdicts := make(chan bool, 1)
	go func() {
		verdicts <- approver.IsUpdateAllowed(domain.UpdateInfo{NewVersion: "0.7.2"})
	}()

	waitPendingUpdate(t, approver)
	select {
	case <-verdicts:
		t.Fatal("the dropped answer was served to the next release")
	case <-time.After(100 * time.Millisecond):
	}

	approver.AnswerUpdate(false)
	select {
	case got := <-verdicts:
		assert.False(t, got)
	case <-time.After(approverWait):
		t.Fatal("the answer never reached the update service")
	}
}

func TestUserApproverNilIsInert(t *testing.T) {
	var approver *UserApprover
	assert.False(t, approver.IsUpdateAllowed(domain.UpdateInfo{NewVersion: "0.7.2"}))
	assert.Empty(t, approver.GetPendingUpdate().NewVersion)
	assert.NotPanics(t, func() { approver.AnswerUpdate(true) })
}

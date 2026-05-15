//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/event"
)

type stubSubsRepo struct {
	addFn    func(ownerId, targetId string) error
	removeFn func(ownerId, targetId string) error
}

func (s stubSubsRepo) Add(o, t string) error {
	if s.addFn != nil {
		return s.addFn(o, t)
	}
	return nil
}

func (s stubSubsRepo) Remove(o, t string) error {
	if s.removeFn != nil {
		return s.removeFn(o, t)
	}
	return nil
}

func TestStreamSubscribeUserHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamSubscribeUserHandler(stubSubsRepo{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty self", func(t *testing.T) {
		_, err := StreamSubscribeUserHandler(stubSubsRepo{})(marshal(t, event.SubscribeUserEvent{TargetId: "b"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty target", func(t *testing.T) {
		_, err := StreamSubscribeUserHandler(stubSubsRepo{})(marshal(t, event.SubscribeUserEvent{SelfId: "a"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("self subscribe", func(t *testing.T) {
		_, err := StreamSubscribeUserHandler(stubSubsRepo{})(marshal(t, event.SubscribeUserEvent{SelfId: "a", TargetId: "a"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("boom")
		_, err := StreamSubscribeUserHandler(stubSubsRepo{addFn: func(_, _ string) error { return repoErr }})(marshal(t, event.SubscribeUserEvent{SelfId: "a", TargetId: "b"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamSubscribeUserHandler(stubSubsRepo{})(marshal(t, event.SubscribeUserEvent{SelfId: "a", TargetId: "b"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
	})
}

func TestStreamUnsubscribeUserHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamUnsubscribeUserHandler(stubSubsRepo{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamUnsubscribeUserHandler(stubSubsRepo{})(marshal(t, event.UnsubscribeUserEvent{SelfId: "a", TargetId: "b"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
	})
}

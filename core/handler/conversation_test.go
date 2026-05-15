//nolint:all
package handler

import (
	"errors"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/event"
)

type stubConvoRepo struct {
	touchFn func(userId, rootId string, at time.Time) error
	hideFn  func(userId, rootId string) error
	listFn  func(userId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubConvoRepo) Touch(userId, rootId string, at time.Time) error {
	if s.touchFn != nil {
		return s.touchFn(userId, rootId, at)
	}
	return nil
}

func (s stubConvoRepo) Hide(userId, rootId string) error {
	if s.hideFn != nil {
		return s.hideFn(userId, rootId)
	}
	return nil
}

func (s stubConvoRepo) List(userId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.listFn != nil {
		return s.listFn(userId, limit, cursor)
	}
	return nil, "end", nil
}

func TestStreamGetConversationsHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamGetConversationsHandler(stubConvoRepo{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user id", func(t *testing.T) {
		_, err := StreamGetConversationsHandler(stubConvoRepo{})(marshal(t, event.GetConversationsEvent{}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamGetConversationsHandler(stubConvoRepo{listFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
			return []string{"r1", "r2"}, "end", nil
		}})(marshal(t, event.GetConversationsEvent{UserId: "u"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		r := resp.(event.GetConversationsResponse)
		if len(r.RootTweetIds) != 2 {
			t.Fatalf("expected 2 ids, got %d", len(r.RootTweetIds))
		}
	})
}

func TestStreamDeleteConversationHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamDeleteConversationHandler(stubConvoRepo{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user id", func(t *testing.T) {
		_, err := StreamDeleteConversationHandler(stubConvoRepo{})(marshal(t, event.DeleteConversationEvent{RootTweetId: "r"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty tweet id", func(t *testing.T) {
		_, err := StreamDeleteConversationHandler(stubConvoRepo{})(marshal(t, event.DeleteConversationEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("boom")
		_, err := StreamDeleteConversationHandler(stubConvoRepo{hideFn: func(_, _ string) error {
			return repoErr
		}})(marshal(t, event.DeleteConversationEvent{UserId: "u", RootTweetId: "r"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamDeleteConversationHandler(stubConvoRepo{})(marshal(t, event.DeleteConversationEvent{UserId: "u", RootTweetId: "r"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
	})
}

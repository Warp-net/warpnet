//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubPinRepo struct {
	getFn   func(userId, tweetId string) (domain.Tweet, error)
	pinFn   func(userId, tweetId string) (domain.Tweet, error)
	unpinFn func(userId, tweetId string) (domain.Tweet, error)
}

func (s stubPinRepo) Get(userId, tweetId string) (domain.Tweet, error) {
	if s.getFn != nil {
		return s.getFn(userId, tweetId)
	}
	return domain.Tweet{Id: tweetId, UserId: userId}, nil
}

func (s stubPinRepo) Pin(userId, tweetId string) (domain.Tweet, error) {
	if s.pinFn != nil {
		return s.pinFn(userId, tweetId)
	}
	return domain.Tweet{Id: tweetId, UserId: userId, Pinned: true}, nil
}

func (s stubPinRepo) Unpin(userId, tweetId string) (domain.Tweet, error) {
	if s.unpinFn != nil {
		return s.unpinFn(userId, tweetId)
	}
	return domain.Tweet{Id: tweetId, UserId: userId, Pinned: false}, nil
}

func TestStreamPinTweetHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		h := StreamPinTweetHandler(stubPinRepo{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user id", func(t *testing.T) {
		h := StreamPinTweetHandler(stubPinRepo{})
		_, err := h(marshal(t, event.PinTweetEvent{TweetId: "t"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty tweet id", func(t *testing.T) {
		h := StreamPinTweetHandler(stubPinRepo{})
		_, err := h(marshal(t, event.PinTweetEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("not author", func(t *testing.T) {
		h := StreamPinTweetHandler(stubPinRepo{getFn: func(_, tweetId string) (domain.Tweet, error) {
			return domain.Tweet{Id: tweetId, UserId: "other"}, nil
		}})
		_, err := h(marshal(t, event.PinTweetEvent{UserId: "u", TweetId: "t"}), nil)
		if err == nil {
			t.Fatal("expected author check to fail")
		}
	})
	t.Run("get error", func(t *testing.T) {
		repoErr := errors.New("not found")
		h := StreamPinTweetHandler(stubPinRepo{getFn: func(_, _ string) (domain.Tweet, error) {
			return domain.Tweet{}, repoErr
		}})
		_, err := h(marshal(t, event.PinTweetEvent{UserId: "u", TweetId: "t"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
	t.Run("happy path", func(t *testing.T) {
		h := StreamPinTweetHandler(stubPinRepo{})
		resp, err := h(marshal(t, event.PinTweetEvent{UserId: "u", TweetId: "t"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		tw := resp.(domain.Tweet)
		if !tw.Pinned {
			t.Fatal("expected Pinned=true")
		}
	})
}

func TestStreamUnpinTweetHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		h := StreamUnpinTweetHandler(stubPinRepo{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user id", func(t *testing.T) {
		h := StreamUnpinTweetHandler(stubPinRepo{})
		_, err := h(marshal(t, event.UnpinTweetEvent{TweetId: "t"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty tweet id", func(t *testing.T) {
		h := StreamUnpinTweetHandler(stubPinRepo{})
		_, err := h(marshal(t, event.UnpinTweetEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("not author", func(t *testing.T) {
		h := StreamUnpinTweetHandler(stubPinRepo{getFn: func(_, tweetId string) (domain.Tweet, error) {
			return domain.Tweet{Id: tweetId, UserId: "other"}, nil
		}})
		_, err := h(marshal(t, event.UnpinTweetEvent{UserId: "u", TweetId: "t"}), nil)
		if err == nil {
			t.Fatal("expected author check to fail")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		h := StreamUnpinTweetHandler(stubPinRepo{})
		resp, err := h(marshal(t, event.UnpinTweetEvent{UserId: "u", TweetId: "t"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		tw := resp.(domain.Tweet)
		if tw.Pinned {
			t.Fatal("expected Pinned=false")
		}
	})
}

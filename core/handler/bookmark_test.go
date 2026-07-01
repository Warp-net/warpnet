//nolint:all
package handler

import (
	"errors"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubBookmarkRepo struct {
	bookmarkFn   func(userId, tweetId, ownerUserId string) error
	unbookmarkFn func(userId, tweetId string) error
	listFn       func(userId string, limit *uint64, cursor *string) ([]domain.Bookmark, string, error)
}

func (s stubBookmarkRepo) Bookmark(userId, tweetId, ownerUserId string) error {
	if s.bookmarkFn != nil {
		return s.bookmarkFn(userId, tweetId, ownerUserId)
	}
	return nil
}

func (s stubBookmarkRepo) Unbookmark(userId, tweetId string) error {
	if s.unbookmarkFn != nil {
		return s.unbookmarkFn(userId, tweetId)
	}
	return nil
}

func (s stubBookmarkRepo) List(userId string, limit *uint64, cursor *string) ([]domain.Bookmark, string, error) {
	if s.listFn != nil {
		return s.listFn(userId, limit, cursor)
	}
	return nil, "end", nil
}

func TestStreamBookmarkHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		h := StreamBookmarkHandler(stubBookmarkRepo{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamBookmarkHandler(stubBookmarkRepo{})
		_, err := h(marshal(t, event.BookmarkEvent{TweetId: "t", OwnerUserId: "o"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty tweet id", func(t *testing.T) {
		h := StreamBookmarkHandler(stubBookmarkRepo{})
		_, err := h(marshal(t, event.BookmarkEvent{UserId: "u", OwnerUserId: "o"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty owner id", func(t *testing.T) {
		h := StreamBookmarkHandler(stubBookmarkRepo{})
		_, err := h(marshal(t, event.BookmarkEvent{UserId: "u", TweetId: "t"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("boom")
		h := StreamBookmarkHandler(stubBookmarkRepo{bookmarkFn: func(_, _, _ string) error { return repoErr }})
		_, err := h(marshal(t, event.BookmarkEvent{UserId: "u", TweetId: "t", OwnerUserId: "o"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("happy path", func(t *testing.T) {
		var captured event.BookmarkEvent
		h := StreamBookmarkHandler(stubBookmarkRepo{bookmarkFn: func(u, tw, o string) error {
			captured = event.BookmarkEvent{UserId: u, TweetId: tw, OwnerUserId: o}
			return nil
		}})
		resp, err := h(marshal(t, event.BookmarkEvent{UserId: "u", TweetId: "t", OwnerUserId: "o"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted body, got %v", resp)
		}
		if captured.UserId != "u" || captured.TweetId != "t" || captured.OwnerUserId != "o" {
			t.Fatalf("repo called with wrong args: %+v", captured)
		}
	})
}

func TestStreamUnbookmarkHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		h := StreamUnbookmarkHandler(stubBookmarkRepo{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user id", func(t *testing.T) {
		h := StreamUnbookmarkHandler(stubBookmarkRepo{})
		_, err := h(marshal(t, event.UnbookmarkEvent{TweetId: "t"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty tweet id", func(t *testing.T) {
		h := StreamUnbookmarkHandler(stubBookmarkRepo{})
		_, err := h(marshal(t, event.UnbookmarkEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		h := StreamUnbookmarkHandler(stubBookmarkRepo{})
		resp, err := h(marshal(t, event.UnbookmarkEvent{UserId: "u", TweetId: "t"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted body, got %v", resp)
		}
	})
}

func TestStreamGetBookmarksHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetBookmarksHandler(stubBookmarkRepo{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user id", func(t *testing.T) {
		h := StreamGetBookmarksHandler(stubBookmarkRepo{})
		_, err := h(marshal(t, event.GetBookmarksEvent{}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		now := time.Now()
		h := StreamGetBookmarksHandler(stubBookmarkRepo{listFn: func(_ string, _ *uint64, _ *string) ([]domain.Bookmark, string, error) {
			return []domain.Bookmark{
				{UserId: "u", TweetId: "t1", OwnerUserId: "o1", CreatedAt: now},
			}, "end", nil
		}})
		resp, err := h(marshal(t, event.GetBookmarksEvent{UserId: "u"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		r := resp.(event.GetBookmarksResponse)
		if len(r.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(r.Items))
		}
		if r.Items[0].TweetId != "t1" {
			t.Fatalf("expected t1, got %s", r.Items[0].TweetId)
		}
		if r.Cursor != "end" {
			t.Fatalf("expected cursor end, got %s", r.Cursor)
		}
	})
}

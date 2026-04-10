//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubTimelineFetcher struct {
	getTimelineFn func(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error)
}

func (s stubTimelineFetcher) GetTimeline(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
	if s.getTimelineFn != nil {
		return s.getTimelineFn(userId, limit, cursor)
	}
	return nil, "", nil
}

func TestStreamTimelineHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamTimelineHandler(stubTimelineFetcher{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamTimelineHandler(stubTimelineFetcher{})
		_, err := h(marshal(t, event.GetTimelineEvent{}), nil)
		if err == nil || err.Error() != "empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty timeline returns empty array not nil", func(t *testing.T) {
		h := StreamTimelineHandler(stubTimelineFetcher{getTimelineFn: func(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
			return nil, "end", nil
		}})
		resp, err := h(marshal(t, event.GetTimelineEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.TweetsResponse)
		if r.Tweets == nil {
			t.Fatal("expected non-nil empty tweets slice")
		}
		if len(r.Tweets) != 0 {
			t.Fatalf("expected 0 tweets, got %d", len(r.Tweets))
		}
	})

	t.Run("with tweets", func(t *testing.T) {
		h := StreamTimelineHandler(stubTimelineFetcher{getTimelineFn: func(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
			return []domain.Tweet{{Id: "t1", Text: "hello"}}, "end", nil
		}})
		resp, err := h(marshal(t, event.GetTimelineEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.TweetsResponse)
		if len(r.Tweets) != 1 {
			t.Fatalf("expected 1 tweet, got %d", len(r.Tweets))
		}
		if r.UserId != owner {
			t.Fatalf("expected user id %q, got %q", owner, r.UserId)
		}
		if r.Cursor != "end" {
			t.Fatalf("expected cursor 'end', got %q", r.Cursor)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamTimelineHandler(stubTimelineFetcher{getTimelineFn: func(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
			return nil, "", repoErr
		}})
		_, err := h(marshal(t, event.GetTimelineEvent{UserId: owner}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("with pagination", func(t *testing.T) {
		var capturedLimit *uint64
		var capturedCursor *string
		h := StreamTimelineHandler(stubTimelineFetcher{getTimelineFn: func(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
			capturedLimit = limit
			capturedCursor = cursor
			return []domain.Tweet{}, "next", nil
		}})
		limit := uint64(10)
		cursor := "some-cursor"
		_, err := h(marshal(t, event.GetTimelineEvent{UserId: owner, Limit: &limit, Cursor: &cursor}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if capturedLimit == nil || *capturedLimit != 10 {
			t.Fatalf("expected limit 10, got %v", capturedLimit)
		}
		if capturedCursor == nil || *capturedCursor != "some-cursor" {
			t.Fatalf("expected cursor 'some-cursor', got %v", capturedCursor)
		}
	})
}

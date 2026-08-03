//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubReactorsRepo struct {
	reactorsFn func(tweetId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubReactorsRepo) Reactors(tweetId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.reactorsFn != nil {
		return s.reactorsFn(tweetId, limit, cursor)
	}
	return nil, "end", nil
}

type stubRetweetersRepo struct {
	retweetersFn func(tweetId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubRetweetersRepo) Retweeters(tweetId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.retweetersFn != nil {
		return s.retweetersFn(tweetId, limit, cursor)
	}
	return nil, "end", nil
}

type stubReactedUserFetcher struct {
	batchFn func(ids ...string) ([]domain.User, error)
	getFn   func(id string) (domain.User, error)
}

func (s stubReactedUserFetcher) GetBatch(ids ...string) ([]domain.User, error) {
	if s.batchFn != nil {
		return s.batchFn(ids...)
	}
	out := make([]domain.User, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.User{Id: id})
	}
	return out, nil
}

func (s stubReactedUserFetcher) Get(id string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(id)
	}
	return domain.User{Id: id}, nil
}

func TestStreamGetTweetReactorsHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamGetTweetReactorsHandler(stubReactorsRepo{}, stubReactedUserFetcher{}, nil)([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty tweet id", func(t *testing.T) {
		_, err := StreamGetTweetReactorsHandler(stubReactorsRepo{}, stubReactedUserFetcher{}, nil)(marshal(t, event.GetTweetReactorsEvent{}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("boom")
		_, err := StreamGetTweetReactorsHandler(stubReactorsRepo{reactorsFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
			return nil, "", repoErr
		}}, stubReactedUserFetcher{}, nil)(marshal(t, event.GetTweetReactorsEvent{TweetId: "t"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
	t.Run("happy path hydrates users", func(t *testing.T) {
		resp, err := StreamGetTweetReactorsHandler(stubReactorsRepo{reactorsFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
			return []string{"u1", "u2"}, "end", nil
		}}, stubReactedUserFetcher{}, nil)(marshal(t, event.GetTweetReactorsEvent{TweetId: "t"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 2 {
			t.Fatalf("expected 2 users, got %d", len(r.Users))
		}
		if r.Cursor != "end" {
			t.Fatalf("expected end cursor, got %s", r.Cursor)
		}
	})
}

func TestStreamGetTweetRetweetersHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamGetTweetRetweetersHandler(stubRetweetersRepo{}, stubReactedUserFetcher{}, nil)([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty tweet id", func(t *testing.T) {
		_, err := StreamGetTweetRetweetersHandler(stubRetweetersRepo{}, stubReactedUserFetcher{}, nil)(marshal(t, event.GetTweetRetweetersEvent{}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamGetTweetRetweetersHandler(stubRetweetersRepo{retweetersFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
			return []string{"r1"}, "end", nil
		}}, stubReactedUserFetcher{}, nil)(marshal(t, event.GetTweetRetweetersEvent{TweetId: "t"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 1 {
			t.Fatalf("expected 1 user, got %d", len(r.Users))
		}
	})
}

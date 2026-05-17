//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubLikersRepo struct {
	likersFn func(tweetId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubLikersRepo) Likers(tweetId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.likersFn != nil {
		return s.likersFn(tweetId, limit, cursor)
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

type stubLikedUserFetcher struct {
	batchFn func(ids ...string) ([]domain.User, error)
	getFn   func(id string) (domain.User, error)
}

func (s stubLikedUserFetcher) GetBatch(ids ...string) ([]domain.User, error) {
	if s.batchFn != nil {
		return s.batchFn(ids...)
	}
	out := make([]domain.User, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.User{Id: id})
	}
	return out, nil
}

func (s stubLikedUserFetcher) Get(id string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(id)
	}
	return domain.User{Id: id}, nil
}

func TestStreamGetTweetLikersHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamGetTweetLikersHandler(stubLikersRepo{}, stubLikedUserFetcher{}, nil)([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty tweet id", func(t *testing.T) {
		_, err := StreamGetTweetLikersHandler(stubLikersRepo{}, stubLikedUserFetcher{}, nil)(marshal(t, event.GetTweetLikersEvent{}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("boom")
		_, err := StreamGetTweetLikersHandler(stubLikersRepo{likersFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
			return nil, "", repoErr
		}}, stubLikedUserFetcher{}, nil)(marshal(t, event.GetTweetLikersEvent{TweetId: "t"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
	t.Run("happy path hydrates users", func(t *testing.T) {
		resp, err := StreamGetTweetLikersHandler(stubLikersRepo{likersFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
			return []string{"u1", "u2"}, "end", nil
		}}, stubLikedUserFetcher{}, nil)(marshal(t, event.GetTweetLikersEvent{TweetId: "t"}), nil)
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
		_, err := StreamGetTweetRetweetersHandler(stubRetweetersRepo{}, stubLikedUserFetcher{}, nil)([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty tweet id", func(t *testing.T) {
		_, err := StreamGetTweetRetweetersHandler(stubRetweetersRepo{}, stubLikedUserFetcher{}, nil)(marshal(t, event.GetTweetRetweetersEvent{}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamGetTweetRetweetersHandler(stubRetweetersRepo{retweetersFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
			return []string{"r1"}, "end", nil
		}}, stubLikedUserFetcher{}, nil)(marshal(t, event.GetTweetRetweetersEvent{TweetId: "t"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 1 {
			t.Fatalf("expected 1 user, got %d", len(r.Users))
		}
	})
}

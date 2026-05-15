//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubQuoteRepo struct {
	getFn     func(userId, tweetId string) (domain.Tweet, error)
	createFn  func(userId string, tweet domain.Tweet) (domain.Tweet, error)
	deleteFn  func(userId, tweetId string) error
	appendFn  func(quotedId string, quoteTweet domain.Tweet) error
	quotingFn func(tweetId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error)
}

func (s stubQuoteRepo) Get(uid, tid string) (domain.Tweet, error) {
	if s.getFn != nil {
		return s.getFn(uid, tid)
	}
	return domain.Tweet{Id: tid, UserId: uid}, nil
}

func (s stubQuoteRepo) Create(uid string, t domain.Tweet) (domain.Tweet, error) {
	if s.createFn != nil {
		return s.createFn(uid, t)
	}
	if t.Id == "" {
		t.Id = "q-id"
	}
	return t, nil
}

func (s stubQuoteRepo) Delete(uid, tid string) error {
	if s.deleteFn != nil {
		return s.deleteFn(uid, tid)
	}
	return nil
}

func (s stubQuoteRepo) AppendQuoting(qid string, q domain.Tweet) error {
	if s.appendFn != nil {
		return s.appendFn(qid, q)
	}
	return nil
}

func (s stubQuoteRepo) Quoting(tid string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
	if s.quotingFn != nil {
		return s.quotingFn(tid, limit, cursor)
	}
	return nil, "end", nil
}

func quotedIdPtr(s string) *string { return &s }

func quoteTweet(userId, text, quotedId, quotedUser string) domain.Tweet {
	t := domain.Tweet{UserId: userId, Text: text}
	if quotedId != "" {
		t.QuotedTweetId = quotedIdPtr(quotedId)
	}
	if quotedUser != "" {
		t.QuotedUserId = quotedIdPtr(quotedUser)
	}
	return t
}

func TestStreamNewQuoteHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamNewQuoteHandler(stubQuoteRepo{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user", func(t *testing.T) {
		_, err := StreamNewQuoteHandler(stubQuoteRepo{})(marshal(t, quoteTweet("", "x", "t", "o")), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty quoted tweet id", func(t *testing.T) {
		_, err := StreamNewQuoteHandler(stubQuoteRepo{})(marshal(t, quoteTweet("u", "x", "", "o")), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty quoted user id", func(t *testing.T) {
		_, err := StreamNewQuoteHandler(stubQuoteRepo{})(marshal(t, quoteTweet("u", "x", "t", "")), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty text", func(t *testing.T) {
		_, err := StreamNewQuoteHandler(stubQuoteRepo{})(marshal(t, quoteTweet("u", "", "t", "o")), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path indexes the quote", func(t *testing.T) {
		var indexed string
		resp, err := StreamNewQuoteHandler(stubQuoteRepo{appendFn: func(qid string, _ domain.Tweet) error {
			indexed = qid
			return nil
		}})(marshal(t, quoteTweet("u", "interesting", "t", "o")), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		tw := resp.(domain.Tweet)
		if tw.Text != "interesting" {
			t.Fatalf("bad text: %s", tw.Text)
		}
		if indexed != "t" {
			t.Fatalf("expected quote index for 't', got %q", indexed)
		}
	})
	t.Run("create error", func(t *testing.T) {
		repoErr := errors.New("boom")
		_, err := StreamNewQuoteHandler(stubQuoteRepo{createFn: func(_ string, _ domain.Tweet) (domain.Tweet, error) {
			return domain.Tweet{}, repoErr
		}})(marshal(t, quoteTweet("u", "x", "t", "o")), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
}

func TestStreamDeleteQuoteHandler(t *testing.T) {
	t.Run("empty user", func(t *testing.T) {
		_, err := StreamDeleteQuoteHandler(stubQuoteRepo{})(marshal(t, event.DeleteQuoteEvent{TweetId: "t"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty tweet", func(t *testing.T) {
		_, err := StreamDeleteQuoteHandler(stubQuoteRepo{})(marshal(t, event.DeleteQuoteEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("not author", func(t *testing.T) {
		_, err := StreamDeleteQuoteHandler(stubQuoteRepo{getFn: func(_, tid string) (domain.Tweet, error) {
			return domain.Tweet{Id: tid, UserId: "other"}, nil
		}})(marshal(t, event.DeleteQuoteEvent{UserId: "u", TweetId: "t"}), nil)
		if err == nil {
			t.Fatal("expected author check to fail")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamDeleteQuoteHandler(stubQuoteRepo{})(marshal(t, event.DeleteQuoteEvent{UserId: "u", TweetId: "t"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
	})
}

func TestStreamGetQuotingHandler(t *testing.T) {
	t.Run("empty tweet id", func(t *testing.T) {
		_, err := StreamGetQuotingHandler(stubQuoteRepo{})(marshal(t, event.GetQuotingEvent{}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamGetQuotingHandler(stubQuoteRepo{quotingFn: func(_ string, _ *uint64, _ *string) ([]domain.Tweet, string, error) {
			return []domain.Tweet{{Id: "q1", Text: "a quote"}}, "end", nil
		}})(marshal(t, event.GetQuotingEvent{TweetId: "t", OwnerUserId: "u"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		r := resp.(event.GetQuotingResponse)
		if len(r.Tweets) != 1 || r.Tweets[0].Text != "a quote" {
			t.Fatalf("bad response: %+v", r)
		}
	})
}

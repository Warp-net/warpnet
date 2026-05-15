//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubEditTweetRepo struct {
	getFn    func(userId, tweetId string) (domain.Tweet, error)
	updateFn func(tweet domain.Tweet) error
}

func (s stubEditTweetRepo) Get(userId, tweetId string) (domain.Tweet, error) {
	if s.getFn != nil {
		return s.getFn(userId, tweetId)
	}
	return domain.Tweet{Id: tweetId, UserId: userId, Text: "old"}, nil
}

func (s stubEditTweetRepo) Update(t domain.Tweet) error {
	if s.updateFn != nil {
		return s.updateFn(t)
	}
	return nil
}

type stubEditsAppender struct {
	appendFn func(domain.TweetEdit) (domain.TweetEdit, error)
	listFn   func(tweetId string, limit *uint64, cursor *string) ([]domain.TweetEdit, string, error)
}

func (s stubEditsAppender) Append(e domain.TweetEdit) (domain.TweetEdit, error) {
	if s.appendFn != nil {
		return s.appendFn(e)
	}
	return e, nil
}

func (s stubEditsAppender) List(tweetId string, limit *uint64, cursor *string) ([]domain.TweetEdit, string, error) {
	if s.listFn != nil {
		return s.listFn(tweetId, limit, cursor)
	}
	return nil, "end", nil
}

func TestStreamEditTweetHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamEditTweetHandler(stubEditTweetRepo{}, stubEditsAppender{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user", func(t *testing.T) {
		_, err := StreamEditTweetHandler(stubEditTweetRepo{}, stubEditsAppender{})(marshal(t, event.EditTweetEvent{TweetId: "t", Text: "x"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty tweet id", func(t *testing.T) {
		_, err := StreamEditTweetHandler(stubEditTweetRepo{}, stubEditsAppender{})(marshal(t, event.EditTweetEvent{UserId: "u", Text: "x"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty text", func(t *testing.T) {
		_, err := StreamEditTweetHandler(stubEditTweetRepo{}, stubEditsAppender{})(marshal(t, event.EditTweetEvent{UserId: "u", TweetId: "t"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("not author", func(t *testing.T) {
		repo := stubEditTweetRepo{getFn: func(_, tweetId string) (domain.Tweet, error) {
			return domain.Tweet{Id: tweetId, UserId: "other", Text: "old"}, nil
		}}
		_, err := StreamEditTweetHandler(repo, stubEditsAppender{})(marshal(t, event.EditTweetEvent{UserId: "u", TweetId: "t", Text: "new"}), nil)
		if err == nil {
			t.Fatal("expected author check to fail")
		}
	})
	t.Run("no-op edit returns existing", func(t *testing.T) {
		var appendCalled bool
		repo := stubEditTweetRepo{getFn: func(uid, tid string) (domain.Tweet, error) {
			return domain.Tweet{Id: tid, UserId: uid, Text: "same"}, nil
		}}
		eds := stubEditsAppender{appendFn: func(e domain.TweetEdit) (domain.TweetEdit, error) {
			appendCalled = true
			return e, nil
		}}
		resp, err := StreamEditTweetHandler(repo, eds)(marshal(t, event.EditTweetEvent{UserId: "u", TweetId: "t", Text: "same"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if appendCalled {
			t.Fatal("no-op edit should not append a revision")
		}
		out := resp.(event.EditTweetResponse)
		if out.Text != "same" {
			t.Fatalf("unexpected text: %s", out.Text)
		}
	})
	t.Run("happy path appends prev text then updates", func(t *testing.T) {
		var stored domain.Tweet
		var revision domain.TweetEdit
		repo := stubEditTweetRepo{
			getFn: func(uid, tid string) (domain.Tweet, error) {
				if stored.Id != "" {
					return stored, nil
				}
				return domain.Tweet{Id: tid, UserId: uid, Text: "old"}, nil
			},
			updateFn: func(t domain.Tweet) error {
				stored = t
				return nil
			},
		}
		eds := stubEditsAppender{appendFn: func(e domain.TweetEdit) (domain.TweetEdit, error) {
			revision = e
			return e, nil
		}}
		resp, err := StreamEditTweetHandler(repo, eds)(marshal(t, event.EditTweetEvent{UserId: "u", TweetId: "t", Text: "new"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		out := resp.(event.EditTweetResponse)
		if out.Text != "new" {
			t.Fatalf("expected new text, got %s", out.Text)
		}
		if revision.Text != "old" {
			t.Fatalf("revision should snapshot the PREVIOUS text, got %q", revision.Text)
		}
		if revision.OriginalTweetId != "t" {
			t.Fatalf("revision should reference the tweet, got %q", revision.OriginalTweetId)
		}
	})
	t.Run("update error", func(t *testing.T) {
		repoErr := errors.New("boom")
		repo := stubEditTweetRepo{updateFn: func(_ domain.Tweet) error { return repoErr }}
		_, err := StreamEditTweetHandler(repo, stubEditsAppender{})(marshal(t, event.EditTweetEvent{UserId: "u", TweetId: "t", Text: "new"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
}

func TestStreamGetTweetEditsHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamGetTweetEditsHandler(stubEditsAppender{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty tweet id", func(t *testing.T) {
		_, err := StreamGetTweetEditsHandler(stubEditsAppender{})(marshal(t, event.GetTweetEditsEvent{}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamGetTweetEditsHandler(stubEditsAppender{listFn: func(_ string, _ *uint64, _ *string) ([]domain.TweetEdit, string, error) {
			return []domain.TweetEdit{
				{Id: "e1", OriginalTweetId: "t", Text: "v1"},
				{Id: "e2", OriginalTweetId: "t", Text: "v2"},
			}, "end", nil
		}})(marshal(t, event.GetTweetEditsEvent{TweetId: "t"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		r := resp.(event.TweetEditsResponse)
		if len(r.Edits) != 2 {
			t.Fatalf("expected 2 edits, got %d", len(r.Edits))
		}
		if r.Cursor != "end" {
			t.Fatalf("expected end cursor, got %s", r.Cursor)
		}
	})
}

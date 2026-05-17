//nolint:all
package handler

import (
	"errors"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubSearchUserFetcher struct {
	searchFn func(query string, limit *uint64, cursor *string) ([]domain.User, string, error)
}

// Satisfy UserFetcher; only Search is used by the search handler.
func (s stubSearchUserFetcher) Create(u domain.User) (domain.User, error) { return u, nil }
func (s stubSearchUserFetcher) Get(id string) (domain.User, error)        { return domain.User{Id: id}, nil }
func (s stubSearchUserFetcher) List(_ *uint64, _ *string) ([]domain.User, string, error) {
	return nil, "end", nil
}
func (s stubSearchUserFetcher) Search(q string, l *uint64, c *string) ([]domain.User, string, error) {
	if s.searchFn != nil {
		return s.searchFn(q, l, c)
	}
	return nil, "end", nil
}
func (s stubSearchUserFetcher) WhoToFollow(_ *uint64, _ *string) ([]domain.User, string, error) {
	return nil, "end", nil
}
func (s stubSearchUserFetcher) Update(_ string, u domain.User) (domain.User, error) { return u, nil }
func (s stubSearchUserFetcher) CreateWithTTL(u domain.User, _ time.Duration) (domain.User, error) {
	return u, nil
}

func TestStreamSearchUsersHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamSearchUsersHandler(stubSearchUserFetcher{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty query", func(t *testing.T) {
		_, err := StreamSearchUsersHandler(stubSearchUserFetcher{})(marshal(t, event.SearchUsersEvent{}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("boom")
		_, err := StreamSearchUsersHandler(stubSearchUserFetcher{searchFn: func(_ string, _ *uint64, _ *string) ([]domain.User, string, error) {
			return nil, "", repoErr
		}})(marshal(t, event.SearchUsersEvent{Query: "alice"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
	t.Run("happy path passes through cursor and limit", func(t *testing.T) {
		var gotQuery string
		var gotLimit *uint64
		var gotCursor *string
		limit := uint64(5)
		cursor := "abc"
		resp, err := StreamSearchUsersHandler(stubSearchUserFetcher{searchFn: func(q string, l *uint64, c *string) ([]domain.User, string, error) {
			gotQuery = q
			gotLimit = l
			gotCursor = c
			return []domain.User{{Id: "u1", Username: "alice"}}, "end", nil
		}})(marshal(t, event.SearchUsersEvent{Query: "alice", Limit: &limit, Cursor: &cursor}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		r := resp.(event.SearchUsersResponse)
		if len(r.Users) != 1 || r.Users[0].Username != "alice" {
			t.Fatalf("bad users: %+v", r.Users)
		}
		if r.Cursor != "end" {
			t.Fatalf("bad cursor: %s", r.Cursor)
		}
		if gotQuery != "alice" {
			t.Fatalf("bad query: %s", gotQuery)
		}
		if gotLimit == nil || *gotLimit != 5 {
			t.Fatalf("bad limit: %v", gotLimit)
		}
		if gotCursor == nil || *gotCursor != "abc" {
			t.Fatalf("bad cursor: %v", gotCursor)
		}
	})
}

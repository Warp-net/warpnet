//nolint:all
package node

import (
	"errors"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
)

type stubUserProvider struct {
	createErr error
}

func (s stubUserProvider) Create(user domain.User) (domain.User, error) {
	return user, s.createErr
}
func (s stubUserProvider) CreateWithTTL(user domain.User, _ time.Duration) (domain.User, error) {
	return user, s.createErr
}
func (s stubUserProvider) GetByNodeID(string) (domain.User, error)   { return domain.User{}, nil }
func (s stubUserProvider) Get(string) (domain.User, error)           { return domain.User{}, nil }
func (s stubUserProvider) GetBatch(...string) ([]domain.User, error) { return nil, nil }
func (s stubUserProvider) Update(string, domain.User) (domain.User, error) {
	return domain.User{}, nil
}
func (s stubUserProvider) List(*uint64, *string) ([]domain.User, string, error) { return nil, "", nil }
func (s stubUserProvider) Search(string, *uint64, *string) ([]domain.User, string, error) {
	return nil, "", nil
}
func (s stubUserProvider) WhoToFollow(*uint64, *string) ([]domain.User, string, error) {
	return nil, "", nil
}

type stubNotifier struct {
	added []domain.Notification
}

func (s *stubNotifier) Add(n domain.Notification) error {
	s.added = append(s.added, n)
	return nil
}

func TestNotifyingUserRepo_Create(t *testing.T) {
	owner := "owner-1"

	t.Run("new user notifies owner", func(t *testing.T) {
		n := &stubNotifier{}
		repo := newNotifyingUserRepo(stubUserProvider{}, n, owner)
		if _, err := repo.Create(domain.User{Id: "bob", Username: "bob"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if len(n.added) != 1 {
			t.Fatalf("expected 1 notification, got %d", len(n.added))
		}
		got := n.added[0]
		if got.Type != domain.NotificationNewUserType || got.UserId != owner {
			t.Fatalf("unexpected notification: %+v", got)
		}
		if got.Text != "bob joined Warpnet" {
			t.Fatalf("unexpected text: %q", got.Text)
		}
	})

	t.Run("owner's own record does not notify", func(t *testing.T) {
		n := &stubNotifier{}
		repo := newNotifyingUserRepo(stubUserProvider{}, n, owner)
		if _, err := repo.Create(domain.User{Id: owner, Username: "me"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if len(n.added) != 0 {
			t.Fatalf("expected no notification for owner, got %d", len(n.added))
		}
	})

	t.Run("already-exists does not notify", func(t *testing.T) {
		n := &stubNotifier{}
		repo := newNotifyingUserRepo(stubUserProvider{createErr: database.ErrUserAlreadyExists}, n, owner)
		if _, err := repo.Create(domain.User{Id: "bob"}); !errors.Is(err, database.ErrUserAlreadyExists) {
			t.Fatalf("expected ErrUserAlreadyExists, got %v", err)
		}
		if len(n.added) != 0 {
			t.Fatalf("expected no notification on duplicate, got %d", len(n.added))
		}
	})

	t.Run("falls back to id when username empty", func(t *testing.T) {
		n := &stubNotifier{}
		repo := newNotifyingUserRepo(stubUserProvider{}, n, owner)
		if _, err := repo.Create(domain.User{Id: "xyz"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if len(n.added) != 1 || n.added[0].Text != "xyz joined Warpnet" {
			t.Fatalf("unexpected: %+v", n.added)
		}
	})
}

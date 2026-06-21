package handler

import (
	"errors"
	"github.com/Warp-net/warpnet/core/mastodon"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"testing"
)

func TestStreamGetWhoToFollowHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{}, stubUserFollowsCounter{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{}, stubUserFollowsCounter{})
		_, err := h(marshal(t, event.GetAllUsersEvent{}), nil)
		if err == nil || err.Error() != "empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("filters out self and already followed", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(
			stubAuth{owner: domain.Owner{UserId: owner, NodeId: "node-1", Username: "test"}},
			stubUserFetcher{
				whoToFollowFn: func(limit *uint64, cursor *string) ([]domain.User, string, error) {
					return []domain.User{
						{Id: owner, Network: warpnet.WarpnetName},
						{Id: "already-followed", Network: warpnet.WarpnetName},
						{Id: "new-user", Network: warpnet.WarpnetName},
					}, "end", nil
				},
				getFn: func(userId string) (domain.User, error) {
					return domain.User{Id: owner, Network: warpnet.WarpnetName}, nil
				},
			},
			stubUserFollowsCounter{getFollowingsFn: func(userId string, limit *uint64, cursor *string) ([]string, string, error) {
				return []string{"already-followed"}, "", nil
			}},
		)
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 1 || r.Users[0].Id != "new-user" {
			t.Fatalf("expected only new-user, got: %v", r.Users)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{whoToFollowFn: func(limit *uint64, cursor *string) ([]domain.User, string, error) {
			return nil, "", repoErr
		}}, stubUserFollowsCounter{})
		_, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("empty results", func(t *testing.T) {
		// getFn misses for everyone (incl. the Mastodon entry) so nothing is pinned.
		h := StreamGetWhoToFollowHandler(
			stubAuth{owner: domain.Owner{UserId: owner}},
			stubUserFetcher{getFn: func(userId string) (domain.User, error) {
				return domain.User{}, errors.New("not found")
			}},
			stubUserFollowsCounter{},
		)
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 0 {
			t.Fatalf("expected empty users: %v", r.Users)
		}
	})

	t.Run("pins mastodon entry until followed", func(t *testing.T) {
		fetcher := stubUserFetcher{
			whoToFollowFn: func(limit *uint64, cursor *string) ([]domain.User, string, error) {
				return []domain.User{}, "", nil
			},
			getFn: func(userId string) (domain.User, error) {
				if userId == mastodon.EntryHandle {
					return domain.User{Id: mastodon.EntryHandle, NodeId: mastodon.GatewayNodeID, Network: mastodon.Network}, nil
				}
				return domain.User{}, errors.New("not found")
			},
		}

		h := StreamGetWhoToFollowHandler(
			stubAuth{owner: domain.Owner{UserId: owner, NodeId: "node-1"}},
			fetcher, stubUserFollowsCounter{},
		)
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 1 || r.Users[0].Id != mastodon.EntryHandle {
			t.Fatalf("expected pinned mastodon entry, got: %v", r.Users)
		}

		hFollowed := StreamGetWhoToFollowHandler(
			stubAuth{owner: domain.Owner{UserId: owner, NodeId: "node-1"}},
			fetcher,
			stubUserFollowsCounter{getFollowingsFn: func(userId string, limit *uint64, cursor *string) ([]string, string, error) {
				return []string{mastodon.EntryHandle}, "", nil
			}},
		)
		respFollowed, err := hFollowed(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		rFollowed := respFollowed.(event.UsersResponse)
		if len(rFollowed.Users) != 0 {
			t.Fatalf("expected mastodon entry hidden once followed, got: %v", rFollowed.Users)
		}
	})
}

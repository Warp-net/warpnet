package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/core/fediverse"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubWhoToFollowStreamer struct {
	genericStreamFn func(nodeID string, path stream.WarpRoute, data any) ([]byte, error)
}

func (s stubWhoToFollowStreamer) GenericStream(nodeID string, path stream.WarpRoute, data any) ([]byte, error) {
	if s.genericStreamFn != nil {
		return s.genericStreamFn(nodeID, path, data)
	}
	return nil, warpnet.ErrNodeIsOffline
}

func TestStreamGetWhoToFollowHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{}, stubUserFollowsCounter{}, stubWhoToFollowStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{}, stubUserFollowsCounter{}, stubWhoToFollowStreamer{})
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
			}}, stubWhoToFollowStreamer{},
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

	t.Run("filters out Warpnet users federated back by a gateway", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(
			stubAuth{owner: domain.Owner{UserId: owner, NodeId: "node-1", Username: "test"}},
			stubUserFetcher{
				whoToFollowFn: func(limit *uint64, cursor *string) ([]domain.User, string, error) {
					return []domain.User{
						{Id: "01KTRA1QJ8M2W7Y4ZB6C9D3E5F@warpnet-gw.example", NodeId: "gateway", Network: "mastodon"},
						{Id: "alice@mastodon.social", NodeId: "gateway", Network: "mastodon"},
					}, "end", nil
				},
				getFn: func(userId string) (domain.User, error) {
					return domain.User{Id: owner, Network: warpnet.WarpnetName}, nil
				},
			},
			stubUserFollowsCounter{}, stubWhoToFollowStreamer{},
		)
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 1 || r.Users[0].Id != "alice@mastodon.social" {
			t.Fatalf("expected only alice@mastodon.social, got: %v", r.Users)
		}
	})

	t.Run("keeps repo order", func(t *testing.T) {
		ulidID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		h := StreamGetWhoToFollowHandler(
			stubAuth{owner: domain.Owner{UserId: owner, NodeId: "node-owner", Username: "test"}},
			stubUserFetcher{
				whoToFollowFn: func(limit *uint64, cursor *string) ([]domain.User, string, error) {
					return []domain.User{
						{Id: "plain-id", NodeId: "node-1", Network: warpnet.WarpnetName},
						{Id: ulidID, NodeId: "node-2", Network: warpnet.WarpnetName},
					}, "end", nil
				},
				getFn: func(userId string) (domain.User, error) {
					return domain.User{Id: owner, Network: warpnet.WarpnetName}, nil
				},
			},
			stubUserFollowsCounter{}, stubWhoToFollowStreamer{},
		)
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 2 || r.Users[0].Id != "plain-id" || r.Users[1].Id != ulidID {
			t.Fatalf("expected repo order preserved, got: %v", r.Users)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{whoToFollowFn: func(limit *uint64, cursor *string) ([]domain.User, string, error) {
			return nil, "", repoErr
		}}, stubUserFollowsCounter{}, stubWhoToFollowStreamer{})
		_, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("empty results", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{}, stubUserFollowsCounter{}, stubWhoToFollowStreamer{})
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 0 {
			t.Fatalf("expected empty users: %v", r.Users)
		}
	})
}

func TestWhoToFollowGatewayDiscovery(t *testing.T) {
	owner := domain.Owner{UserId: "owner", NodeId: "own-node"}
	gatewayID := fediverse.GatewayNodeID()
	remote := []domain.User{
		{Id: "alice@remote.example", Username: "Alice", NodeId: gatewayID, Network: fediverse.MastodonNetwork},
		{Id: "followed@remote.example", NodeId: gatewayID, Network: fediverse.MastodonNetwork},
	}
	stored := []domain.User{{Id: "local", NodeId: "local-node", Network: warpnet.WarpnetName}}
	updates := []string{}
	repo := stubUserFetcher{
		createFn: func(user domain.User) (domain.User, error) {
			stored = append(stored, user)
			return user, database.ErrUserAlreadyExists
		},
		updateFn: func(id string, user domain.User) (domain.User, error) {
			updates = append(updates, id)
			return user, nil
		},
		getFn: func(id string) (domain.User, error) {
			return domain.User{Id: owner.UserId}, nil
		},
		whoToFollowFn: func(_ *uint64, _ *string) ([]domain.User, string, error) { return stored, "end", nil },
	}
	streamer := stubWhoToFollowStreamer{genericStreamFn: func(id string, route stream.WarpRoute, payload any) ([]byte, error) {
		ev := payload.(event.GetAllUsersEvent)
		if id != gatewayID || route != event.PUBLIC_GET_WHOTOFOLLOW || ev.UserId != owner.UserId || ev.Cursor != nil {
			t.Fatalf("request: %s %s %+v", id, route, ev)
		}
		return marshal(t, event.UsersResponse{Users: remote}), nil
	}}
	h := StreamGetWhoToFollowHandler(stubAuth{owner: owner}, repo, stubUserFollowsCounter{getFollowingsFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
		return []string{"followed@remote.example"}, "end", nil
	}}, streamer)
	raw, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner.UserId}), nil)
	if err != nil {
		t.Fatal(err)
	}
	response := raw.(event.UsersResponse)
	if len(response.Users) != 2 || response.Users[0].Id != "local" || response.Users[1].Id != "alice@remote.example" {
		t.Fatalf("recommendations = %+v", response)
	}
	if len(updates) != 2 || updates[0] != "alice@remote.example" || updates[1] != "followed@remote.example" {
		t.Fatalf("updated = %v", updates)
	}
}

func TestWhoToFollowGatewayFailurePreservesLocal(t *testing.T) {
	for name, data := range map[string][]byte{
		"offline":      nil,
		"malformed":    []byte("{"),
		"remote error": marshal(t, event.ResponseError{Message: "directory unavailable"}),
	} {
		t.Run(name, func(t *testing.T) {
			owner := domain.Owner{UserId: "owner", NodeId: "own-node"}
			repo := stubUserFetcher{
				getFn: func(id string) (domain.User, error) {
					return domain.User{Id: owner.UserId}, nil
				},
				whoToFollowFn: func(_ *uint64, _ *string) ([]domain.User, string, error) {
					return []domain.User{{Id: "local", NodeId: "other-node", Network: warpnet.WarpnetName}}, "end", nil
				},
				createFn: func(user domain.User) (domain.User, error) { t.Fatal("stored a failed response"); return user, nil },
			}
			streamer := stubWhoToFollowStreamer{genericStreamFn: func(_ string, _ stream.WarpRoute, _ any) ([]byte, error) {
				if data == nil {
					return nil, warpnet.ErrNodeIsOffline
				}
				return data, nil
			}}
			h := StreamGetWhoToFollowHandler(stubAuth{owner: owner}, repo, stubUserFollowsCounter{}, streamer)
			raw, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner.UserId}), nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := raw.(event.UsersResponse); len(got.Users) != 1 || got.Users[0].Id != "local" {
				t.Fatalf("fallback = %+v", got)
			}
		})
	}
}

func TestWhoToFollowGatewayPersistsProfiles(t *testing.T) {
	db, err := local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.NewAuthRepo(db, "test").Authenticate("test", "test"); err != nil {
		t.Fatal(err)
	}
	repo := database.NewUserRepo(db)
	owner := domain.Owner{UserId: "owner", NodeId: "own-node"}
	_, err = repo.Create(domain.User{Id: owner.UserId, Username: "Owner", NodeId: owner.NodeId, Network: warpnet.WarpnetName})
	if err != nil {
		t.Fatal(err)
	}
	fediverse.SeedEntryUser(repo)
	candidate := domain.User{Id: "alice@remote.example", Username: "Alice", NodeId: fediverse.GatewayNodeID(), Network: fediverse.MastodonNetwork, AvatarKey: "https://remote.example/avatar.png"}
	streamer := stubWhoToFollowStreamer{genericStreamFn: func(_ string, _ stream.WarpRoute, _ any) ([]byte, error) {
		return marshal(t, event.UsersResponse{Users: []domain.User{candidate}}), nil
	}}
	h := StreamGetWhoToFollowHandler(stubAuth{owner: owner}, repo, stubUserFollowsCounter{}, streamer)
	raw, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner.UserId}), nil)
	if err != nil {
		t.Fatal(err)
	}
	response := raw.(event.UsersResponse)
	found := false
	for _, u := range response.Users {
		if u.Id == candidate.Id {
			found = true
		}
	}
	if !found {
		t.Fatalf("recommendations = %+v", response)
	}
	stored, err := repo.Get(candidate.Id)
	if err != nil || stored.NodeId != candidate.NodeId || stored.AvatarKey != candidate.AvatarKey {
		t.Fatalf("persisted profile = %+v, err=%v", stored, err)
	}
}

func TestWhoToFollowSkipsRemoteFetch(t *testing.T) {
	for _, test := range []struct {
		name      string
		userID    string
		cursor    string
		ownerNode string
	}{
		{name: "other profile", userID: "other", ownerNode: "own-node"},
		{name: "later page", userID: "owner", cursor: "end", ownerNode: "own-node"},
		{name: "self gateway", userID: "owner", ownerNode: fediverse.GatewayNodeID()},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner := domain.Owner{UserId: "owner", NodeId: test.ownerNode}
			h := StreamGetWhoToFollowHandler(stubAuth{owner: owner}, stubUserFetcher{}, stubUserFollowsCounter{}, stubWhoToFollowStreamer{
				genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
					t.Fatal("unexpected remote fetch")
					return nil, nil
				},
			})
			_, err := h(marshal(t, event.GetAllUsersEvent{UserId: test.userID, Cursor: &test.cursor}), nil)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestWhoToFollowChecksRepositoryBeforeFetching(t *testing.T) {
	repoErr := errors.New("database unavailable")
	h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: "owner"}}, stubUserFetcher{
		whoToFollowFn: func(*uint64, *string) ([]domain.User, string, error) { return nil, "", repoErr },
	}, stubUserFollowsCounter{}, stubWhoToFollowStreamer{
		genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			t.Fatal("fetch after repository failure")
			return nil, nil
		},
	})
	_, err := h(marshal(t, event.GetAllUsersEvent{UserId: "owner"}), nil)
	if !errors.Is(err, repoErr) {
		t.Fatalf("error = %v", err)
	}
}

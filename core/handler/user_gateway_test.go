package handler

import (
	"testing"

	"github.com/Warp-net/warpnet/core/fediverse"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

func TestRefreshUsersGatewayDiscovery(t *testing.T) {
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
			if id == fediverse.EntryHandle {
				return domain.User{Id: id, NodeId: fediverse.GatewayNodeID()}, nil
			}
			return domain.User{Id: owner.UserId}, nil
		},
		whoToFollowFn: func(_ *uint64, _ *string) ([]domain.User, string, error) { return stored, "end", nil },
	}
	streamer := stubUserStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner.UserId}, genericStreamFn: func(id string, route stream.WarpRoute, payload any) ([]byte, error) {
		ev := payload.(event.GetAllUsersEvent)
		if id != gatewayID || route != event.PUBLIC_GET_USERS || ev.UserId != fediverse.EntryHandle || ev.Cursor != nil || ev.Limit != nil {
			t.Fatalf("request: %s %s %+v", id, route, ev)
		}
		return marshal(t, event.UsersResponse{Users: remote}), nil
	}}
	localCursor := "local-page"
	localLimit := uint64(5)
	_, err := StreamGetUsersHandler(repo, streamer)(marshal(t, event.GetAllUsersEvent{UserId: owner.UserId, Cursor: &localCursor, Limit: &localLimit}), nil)
	if err != nil {
		t.Fatal(err)
	}
	h := StreamGetWhoToFollowHandler(stubAuth{owner: owner}, repo, stubUserFollowsCounter{getFollowingsFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
		return []string{"followed@remote.example"}, "end", nil
	}})
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

func TestRefreshUsersGatewayFailurePreservesLocal(t *testing.T) {
	for name, data := range map[string][]byte{
		"offline":      nil,
		"malformed":    []byte("{"),
		"remote error": marshal(t, event.ResponseError{Message: "directory unavailable"}),
	} {
		t.Run(name, func(t *testing.T) {
			owner := domain.Owner{UserId: "owner", NodeId: "own-node"}
			repo := stubUserFetcher{
				getFn: func(id string) (domain.User, error) {
					if id == fediverse.EntryHandle {
						return domain.User{Id: id, NodeId: fediverse.GatewayNodeID()}, nil
					}
					return domain.User{Id: owner.UserId}, nil
				},
				whoToFollowFn: func(_ *uint64, _ *string) ([]domain.User, string, error) {
					return []domain.User{{Id: "local", NodeId: "other-node", Network: warpnet.WarpnetName}}, "end", nil
				},
				createFn: func(user domain.User) (domain.User, error) { t.Fatal("stored a failed response"); return user, nil },
			}
			streamer := stubUserStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner.UserId}, genericStreamFn: func(_ string, _ stream.WarpRoute, _ any) ([]byte, error) {
				if data == nil {
					return nil, warpnet.ErrNodeIsOffline
				}
				return data, nil
			}}
			refreshUsers(repo, event.GetAllUsersEvent{UserId: fediverse.EntryHandle}, streamer)
			h := StreamGetWhoToFollowHandler(stubAuth{owner: owner}, repo, stubUserFollowsCounter{})
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

func TestRefreshUsersGatewayPersistsProfiles(t *testing.T) {
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
	streamer := stubUserStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner.UserId}, genericStreamFn: func(_ string, _ stream.WarpRoute, _ any) ([]byte, error) {
		return marshal(t, event.UsersResponse{Users: []domain.User{candidate}}), nil
	}}
	refreshUsers(repo, event.GetAllUsersEvent{UserId: fediverse.EntryHandle}, streamer)
	h := StreamGetWhoToFollowHandler(stubAuth{owner: owner}, repo, stubUserFollowsCounter{})
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

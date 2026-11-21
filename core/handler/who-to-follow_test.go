package handler

import (
	"errors"
	"github.com/Warp-net/warpnet/core/bridge"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/require"
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
			stubAuth{owner: domain.Owner{UserId: owner, NodeId: "node-1", Login: "test"}},
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

	t.Run("filters out Warpnet users federated back by a gateway", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(
			stubAuth{owner: domain.Owner{UserId: owner, NodeId: "node-1", Login: "test"}},
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
			stubUserFollowsCounter{},
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
			stubAuth{owner: domain.Owner{UserId: owner, NodeId: "node-owner", Login: "test"}},
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
			stubUserFollowsCounter{},
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
		}}, stubUserFollowsCounter{})
		_, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("empty results", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{}, stubUserFollowsCounter{})
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

func TestWithBridgedSuggestions(t *testing.T) {
	const owner = "owner-1"
	inner := func([]byte, warpnet.WarpStream) (any, error) { return "answered", nil }
	gatewayUser := domain.User{Id: "npub1abc@nostr.com", Username: "ed", NodeId: bridge.GatewayNodeID(), Network: "nostr"}

	var created []string
	repo := stubUserFetcher{
		createFn: func(u domain.User) (domain.User, error) {
			created = append(created, u.Id)
			return u, nil
		},
	}
	var asked []string
	streamer := stubUserStreamer{
		nodeInfo: warpnet.NodeInfo{OwnerId: owner},
		genericStreamFn: func(nodeId string, path stream.WarpRoute, _ any) ([]byte, error) {
			asked = append(asked, nodeId+" "+string(path))
			return json.Marshal(event.UsersResponse{Users: []domain.User{gatewayUser}})
		},
	}
	h := WithBridgedSuggestions(inner, repo, streamer)

	out, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
	require.NoError(t, err)
	require.Equal(t, "answered", out)
	require.Equal(t, []string{bridge.GatewayNodeID() + " " + string(event.PUBLIC_GET_WHOTOFOLLOW)}, asked)
	require.Equal(t, []string{gatewayUser.Id}, created, "the gateway's suggestions are stored before answering")

	_, err = h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
	require.NoError(t, err)
	require.Len(t, asked, 1, "the gateway is asked at most once per interval")

	other := WithBridgedSuggestions(inner, repo, streamer)
	_, err = other(marshal(t, event.GetAllUsersEvent{UserId: "someone-else"}), nil)
	require.NoError(t, err)
	require.Len(t, asked, 1, "only the owner's own suggestions are refreshed")
}

func TestWithBridgedSuggestionsRetriesAnUnreachedGateway(t *testing.T) {
	const owner = "owner-1"
	inner := func([]byte, warpnet.WarpStream) (any, error) { return nil, nil }
	calls := 0
	streamer := stubUserStreamer{
		nodeInfo: warpnet.NodeInfo{OwnerId: owner},
		genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			calls++
			return nil, errors.New("node is offline")
		},
	}
	h := WithBridgedSuggestions(inner, stubUserFetcher{}, streamer)
	_, _ = h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
	_, _ = h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
	require.Equal(t, 1, calls, "a failed refresh waits for the retry interval")
}

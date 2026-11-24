package handler

import (
	"errors"
	"github.com/Warp-net/warpnet/core/bridge"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"sync"
	"testing"
)

func TestStreamGetWhoToFollowHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{}, stubUserFollowsCounter{}, stubUserStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{}, stubUserFollowsCounter{}, stubUserStreamer{})
		_, err := h(marshal(t, event.GetAllUsersEvent{}), nil)
		if err == nil || err.Error() != "empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("filters out self and already followed", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(
			stubAuth{owner: domain.Owner{UserId: owner, NodeId: "node-1", Login: "test"}},
			stubUserFetcher{
				whoToFollowFn: func(_ string, limit *uint64, cursor *string) ([]domain.User, string, error) {
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
			stubUserStreamer{},
		)
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.WhoToFollowResponse)
		if len(r.Users) != 1 || r.Users[0].Id != "new-user" {
			t.Fatalf("expected only new-user, got: %v", r.Users)
		}
	})

	t.Run("filters out Warpnet users federated back by a gateway", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(
			stubAuth{owner: domain.Owner{UserId: owner, NodeId: "node-1", Login: "test"}},
			stubUserFetcher{
				whoToFollowFn: func(_ string, limit *uint64, cursor *string) ([]domain.User, string, error) {
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
			stubUserStreamer{},
		)
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.WhoToFollowResponse).Networks[bridge.MastodonNetwork]
		if len(r.Users) != 1 || r.Users[0].Id != "alice@mastodon.social" {
			t.Fatalf("expected only alice@mastodon.social, got: %v", r.Users)
		}
	})

	t.Run("keeps repo order", func(t *testing.T) {
		ulidID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		h := StreamGetWhoToFollowHandler(
			stubAuth{owner: domain.Owner{UserId: owner, NodeId: "node-owner", Login: "test"}},
			stubUserFetcher{
				whoToFollowFn: func(_ string, limit *uint64, cursor *string) ([]domain.User, string, error) {
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
			stubUserStreamer{},
		)
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.WhoToFollowResponse)
		if len(r.Users) != 2 || r.Users[0].Id != "plain-id" || r.Users[1].Id != ulidID {
			t.Fatalf("expected repo order preserved, got: %v", r.Users)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{whoToFollowFn: func(_ string, limit *uint64, cursor *string) ([]domain.User, string, error) {
			return nil, "", repoErr
		}}, stubUserFollowsCounter{}, stubUserStreamer{})
		_, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("owner's first page asks the gateway for bridged suggestions", func(t *testing.T) {
		asked := make(chan string, 1)
		streamer := stubUserStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, _ stream.WarpRoute, _ any) ([]byte, error) {
				asked <- nodeId
				return json.Marshal(event.UsersResponse{})
			},
		}
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{}, stubUserFollowsCounter{}, streamer)
		if _, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got := <-asked; got != bridge.GatewayNodeID() {
			t.Fatalf("asked %s, want the gateway", got)
		}
	})

	t.Run("every network gets its own page and cursor", func(t *testing.T) {
		pages := map[string][]domain.User{
			warpnet.WarpnetName:    {{Id: "warp-1", NodeId: "n1", Network: warpnet.WarpnetName}},
			bridge.MastodonNetwork: {{Id: "alice@mastodon.social", NodeId: "gateway", Network: bridge.MastodonNetwork}},
			bridge.ThreadsNetwork:  {{Id: "zuck@threads.net", NodeId: "gateway", Network: bridge.ThreadsNetwork}},
			bridge.NostrNetwork:    {{Id: "npub1@nostr.example", NodeId: "gateway", Network: bridge.NostrNetwork}},
		}
		h := StreamGetWhoToFollowHandler(
			stubAuth{owner: domain.Owner{UserId: owner, NodeId: "node-owner"}},
			stubUserFetcher{
				whoToFollowFn: func(network string, _ *uint64, _ *string) ([]domain.User, string, error) {
					return pages[network], network + "-next", nil
				},
				getFn: func(string) (domain.User, error) {
					return domain.User{Id: owner, Network: warpnet.WarpnetName}, nil
				},
			},
			stubUserFollowsCounter{},
			stubUserStreamer{},
		)
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.WhoToFollowResponse)
		if len(r.Networks) != len(pages) {
			t.Fatalf("expected %d networks, got: %v", len(pages), r.Networks)
		}
		for network, users := range pages {
			got := r.Networks[network]
			if len(got.Users) != 1 || got.Users[0].Id != users[0].Id || got.Cursor != network+"-next" {
				t.Fatalf("%s: unexpected page %+v", network, got)
			}
		}
		if len(r.Users) != 1 || r.Users[0].Id != "warp-1" || r.Cursor != warpnet.WarpnetName+"-next" {
			t.Fatalf("expected the Warpnet page at the top level, got: %+v", r.UsersResponse)
		}
	})

	t.Run("cursors page only the networks asked for", func(t *testing.T) {
		var (
			mx    sync.Mutex
			asked = map[string]string{}
		)
		h := StreamGetWhoToFollowHandler(
			stubAuth{owner: domain.Owner{UserId: owner}},
			stubUserFetcher{
				whoToFollowFn: func(network string, _ *uint64, cursor *string) ([]domain.User, string, error) {
					mx.Lock()
					asked[network] = *cursor
					mx.Unlock()
					return nil, "end", nil
				},
			},
			stubUserFollowsCounter{},
			stubUserStreamer{},
		)
		ev := event.GetWhoToFollowEvent{
			GetAllUsersEvent: event.GetAllUsersEvent{UserId: owner},
			Cursors:          map[string]string{bridge.MastodonNetwork: "m-cursor"},
		}
		resp, err := h(marshal(t, ev), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(asked) != 1 || asked[bridge.MastodonNetwork] != "m-cursor" {
			t.Fatalf("unexpected repo calls: %v", asked)
		}
		r := resp.(event.WhoToFollowResponse)
		if _, ok := r.Networks[bridge.MastodonNetwork]; !ok || len(r.Networks) != 1 {
			t.Fatalf("unexpected networks: %v", r.Networks)
		}
		if r.Users == nil {
			t.Fatal("users must stay a list for older clients")
		}
	})

	t.Run("a legacy cursor pages Warpnet", func(t *testing.T) {
		var asked []string
		h := StreamGetWhoToFollowHandler(
			stubAuth{owner: domain.Owner{UserId: owner}},
			stubUserFetcher{
				whoToFollowFn: func(network string, _ *uint64, cursor *string) ([]domain.User, string, error) {
					asked = append(asked, network+":"+*cursor)
					return nil, "end", nil
				},
			},
			stubUserFollowsCounter{},
			stubUserStreamer{},
		)
		cursor := "w-cursor"
		if _, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner, Cursor: &cursor}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(asked) != 1 || asked[0] != warpnet.WarpnetName+":w-cursor" {
			t.Fatalf("unexpected repo calls: %v", asked)
		}
	})

	t.Run("empty results", func(t *testing.T) {
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubUserFetcher{}, stubUserFollowsCounter{}, stubUserStreamer{})
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.WhoToFollowResponse)
		if len(r.Users) != 0 {
			t.Fatalf("expected empty users: %v", r.Users)
		}
	})
}

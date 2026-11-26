//nolint:all
package handler

import (
	"slices"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/require"
)

func TestWhoToFollow_RecommendsAUserAnotherNodeSeesOnline(t *testing.T) {
	db, err := local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, database.NewAuthRepo(db, "test").Authenticate("test", "test"))
	users := database.NewUserRepo(db)

	for _, u := range []domain.User{
		{Id: "me", NodeId: "node-1", Username: "me"},
		{Id: "remote-owner", NodeId: "node-2", Username: "remote"},
		{Id: "carol", NodeId: "node-3", Username: "carol"},
	} {
		u.Network = warpnet.WarpnetName
		u.CreatedAt = time.Now()
		_, err := users.Create(u)
		require.NoError(t, err)
	}
	_, err = users.Update("carol", domain.User{IsOffline: true})
	require.NoError(t, err)

	recommended := func() []string {
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: "me", NodeId: "node-1"}}, users, stubUserFollowsCounter{}, stubUserStreamer{})
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: "me"}), nil)
		require.NoError(t, err)
		var ids []string
		for _, u := range resp.(event.WhoToFollowResponse).Users {
			ids = append(ids, u.Id)
		}
		return ids
	}
	require.False(t, slices.Contains(recommended(), "carol"), "an offline user is not recommended")

	streamer := stubUserStreamer{
		nodeInfo: warpnet.NodeInfo{OwnerId: "me"},
		genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(event.UsersResponse{Users: []domain.User{
				{Id: "remote-owner", NodeId: "node-2", Username: "remote"},
				{Id: "carol", NodeId: "node-3", Username: "stale carol"},
			}})
		},
	}
	refreshUsers(users, event.GetAllUsersEvent{UserId: "remote-owner"}, streamer)

	require.True(t, slices.Contains(recommended(), "carol"), "opening a profile whose node sees carol online brings her back")
	carol, err := users.Get("carol")
	require.NoError(t, err)
	require.Equal(t, "carol", carol.Username, "a second-hand copy does not rename a known user")
}

func TestWhoToFollow_EveryNetworkFromItsRootOnARealStore(t *testing.T) {
	db, err := local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, database.NewAuthRepo(db, "test").Authenticate("test", "test"))
	users := database.NewUserRepo(db)

	native := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	for _, u := range []domain.User{
		{Id: "me", NodeId: "node-me", Network: warpnet.WarpnetName},
		{Id: native, NodeId: "node-peer", Network: "testnet"},
		{Id: "a@mastodon.social", NodeId: "gateway", Network: "mastodon"},
		{Id: "b@threads.net", NodeId: "gateway", Network: "threads"},
		{Id: "npub1c@nostr.com", NodeId: "gateway", Network: "nostr"},
		{Id: "d@bsky.social", NodeId: "gateway", Network: "bluesky"},
		{Id: "moved@x.example", NodeId: "gateway", Network: "mastodon"},
	} {
		u.CreatedAt = time.Now()
		_, err := users.Create(u)
		require.NoError(t, err)
	}
	_, err = users.Update("moved@x.example", domain.User{Network: "nostr"})
	require.NoError(t, err)

	h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: "me", NodeId: "node-me"}}, users, stubUserFollowsCounter{}, stubUserStreamer{})
	resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: "me"}), nil)
	require.NoError(t, err)
	r := resp.(event.WhoToFollowResponse)

	ids := func(network string) []string {
		var out []string
		for _, u := range r.Networks[network].Users {
			out = append(out, u.Id)
		}
		return out
	}
	require.Equal(t, []string{native}, ids(warpnet.WarpnetName))
	require.Equal(t, []string{"a@mastodon.social"}, ids("mastodon"))
	require.Equal(t, []string{"b@threads.net"}, ids("threads"))
	require.ElementsMatch(t, []string{"npub1c@nostr.com", "moved@x.example"}, ids("nostr"))
	require.NotContains(t, r.Networks, "bluesky", "only the fixed networks are paged")

	legacy, err := h([]byte(`{"user_id":"me","cursor":"","limit":20}`), nil)
	require.NoError(t, err)
	var flat []string
	for _, u := range legacy.(event.WhoToFollowResponse).Users {
		flat = append(flat, u.Id)
	}
	require.Equal(t, []string{native}, flat, "older clients still get the Warpnet page")
}

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
		h := StreamGetWhoToFollowHandler(stubAuth{owner: domain.Owner{UserId: "me", NodeId: "node-1"}}, users, stubUserFollowsCounter{})
		resp, err := h(marshal(t, event.GetAllUsersEvent{UserId: "me"}), nil)
		require.NoError(t, err)
		var ids []string
		for _, u := range resp.(event.UsersResponse).Users {
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

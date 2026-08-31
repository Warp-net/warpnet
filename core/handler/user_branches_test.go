//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/require"
)

func TestStreamGetUserHandlerBranches(t *testing.T) {
	const owner = "owner-1"
	auth := stubAuth{owner: domain.Owner{UserId: owner, NodeId: "own-node"}}

	t.Run("own profile tolerates failing counters", func(t *testing.T) {
		counterErr := errors.New("counter down")
		follows := stubUserFollowsCounter{
			getFollowersCountFn:  func(string) (uint64, error) { return 0, counterErr },
			getFollowingsCountFn: func(string) (uint64, error) { return 0, counterErr },
		}
		tweets := stubUserTweetsCounter{tweetsCountFn: func(string) (uint64, error) {
			return 0, counterErr
		}}

		out, err := StreamGetUserHandler(tweets, follows, stubUserFetcher{}, auth, stubUserStreamer{})(
			marshal(t, event.GetUserEvent{UserId: owner}), nil)
		require.NoError(t, err)
		require.Equal(t, owner, out.(domain.User).Id)
	})

	t.Run("unknown user is fetched from its node and cached", func(t *testing.T) {
		var created domain.User
		repo := stubUserFetcher{
			getFn: func(string) (domain.User, error) {
				return domain.User{}, database.ErrUserNotFound
			},
			createFn: func(u domain.User) (domain.User, error) {
				created = u
				return u, nil
			},
		}
		streamer := stubUserStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(domain.User{Id: "other-1", Username: "remote-name"})
		}}

		out, err := StreamGetUserHandler(stubUserTweetsCounter{}, stubUserFollowsCounter{}, repo, auth, streamer)(
			marshal(t, event.GetUserEvent{UserId: "other-1", NodeId: "other-node"}), nil)
		require.NoError(t, err)
		require.Equal(t, "remote-name", out.(domain.User).Username)
		require.Equal(t, "remote-name", created.Username)
	})

	t.Run("unknown and unreachable user is an error", func(t *testing.T) {
		repo := stubUserFetcher{getFn: func(string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}
		streamer := stubUserStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return nil, errors.New("offline")
		}}

		_, err := StreamGetUserHandler(stubUserTweetsCounter{}, stubUserFollowsCounter{}, repo, auth, streamer)(
			marshal(t, event.GetUserEvent{UserId: "other-1", NodeId: "other-node"}), nil)
		require.Error(t, err)
	})

	t.Run("user hosted on this node is returned as-is", func(t *testing.T) {
		repo := stubUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "own-node"}, nil
		}}
		out, err := StreamGetUserHandler(stubUserTweetsCounter{}, stubUserFollowsCounter{}, repo, auth, stubUserStreamer{})(
			marshal(t, event.GetUserEvent{UserId: "other-1"}), nil)
		require.NoError(t, err)
		require.Equal(t, "own-node", out.(domain.User).NodeId)
	})
}

func TestUpdateOtherUser(t *testing.T) {
	base := domain.User{Id: "other-1", NodeId: "other-node", Username: "known"}
	ev := event.GetUserEvent{UserId: base.Id}

	t.Run("offline node marks the user offline", func(t *testing.T) {
		streamer := stubUserStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return nil, warpnet.ErrNodeIsOffline
		}}
		got := updateOtherUser(ev, base, streamer)
		require.True(t, got.IsOffline)
	})

	t.Run("stream failure returns the user untouched", func(t *testing.T) {
		streamer := stubUserStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return nil, errors.New("boom")
		}}
		got := updateOtherUser(ev, base, streamer)
		require.False(t, got.IsOffline)
		require.Equal(t, "known", got.Username)
	})

	t.Run("user-not-found response marks the user offline", func(t *testing.T) {
		streamer := stubUserStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(event.ResponseError{Message: "user not found"})
		}}
		got := updateOtherUser(ev, base, streamer)
		require.True(t, got.IsOffline)
	})

	t.Run("other error responses leave the user alone", func(t *testing.T) {
		streamer := stubUserStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(event.ResponseError{Message: "internal"})
		}}
		got := updateOtherUser(ev, base, streamer)
		require.False(t, got.IsOffline)
	})

	t.Run("garbage response leaves the user alone but stamps last-seen", func(t *testing.T) {
		streamer := stubUserStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return []byte("{"), nil
		}}
		got := updateOtherUser(ev, base, streamer)
		require.NotNil(t, got.LastSeen)
		require.False(t, got.IsOffline)
	})

	t.Run("valid response merges and stamps last-seen", func(t *testing.T) {
		streamer := stubUserStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(domain.User{Id: base.Id, Username: "fresh"})
		}}
		got := updateOtherUser(ev, base, streamer)
		require.Equal(t, "fresh", got.Username)
		require.NotNil(t, got.LastSeen)
		require.False(t, got.IsOffline)
	})
}

func TestRefreshUsers(t *testing.T) {
	nodeId, _ := nodeIdentity(t)
	selfInfo := warpnet.NodeInfo{ID: nodeId, OwnerId: "owner-1"}
	ev := event.GetAllUsersEvent{UserId: "other-user"}

	t.Run("own owner id is skipped", func(t *testing.T) {
		refreshUsers(stubUserFetcher{}, event.GetAllUsersEvent{UserId: selfInfo.OwnerId}, stubUserStreamer{nodeInfo: selfInfo})
	})

	t.Run("unknown user is skipped", func(t *testing.T) {
		repo := stubUserFetcher{getFn: func(string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}
		refreshUsers(repo, ev, stubUserStreamer{nodeInfo: selfInfo})
	})

	t.Run("lookup failure is skipped", func(t *testing.T) {
		repo := stubUserFetcher{getFn: func(string) (domain.User, error) {
			return domain.User{}, errors.New("down")
		}}
		refreshUsers(repo, ev, stubUserStreamer{nodeInfo: selfInfo})
	})

	t.Run("own node is skipped", func(t *testing.T) {
		repo := stubUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: nodeId.String()}, nil
		}}
		refreshUsers(repo, ev, stubUserStreamer{nodeInfo: selfInfo})
	})

	otherNodeRepo := func(create func(domain.User) (domain.User, error)) stubUserFetcher {
		return stubUserFetcher{
			getFn: func(id string) (domain.User, error) {
				return domain.User{Id: id, NodeId: "other-node"}, nil
			},
			createFn: create,
		}
	}

	t.Run("stream failure is skipped", func(t *testing.T) {
		streamer := stubUserStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return nil, errors.New("offline")
		}}
		refreshUsers(otherNodeRepo(nil), ev, streamer)
	})

	t.Run("error response is skipped", func(t *testing.T) {
		streamer := stubUserStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(event.ResponseError{Message: "boom"})
		}}
		refreshUsers(otherNodeRepo(nil), ev, streamer)
	})

	t.Run("garbage response is skipped", func(t *testing.T) {
		streamer := stubUserStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return []byte("{"), nil
		}}
		refreshUsers(otherNodeRepo(nil), ev, streamer)
	})

	t.Run("stores only foreign online users", func(t *testing.T) {
		var stored []string
		repo := otherNodeRepo(func(u domain.User) (domain.User, error) {
			stored = append(stored, u.Id)
			return u, nil
		})
		streamer := stubUserStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(event.UsersResponse{Users: []domain.User{
				{Id: "keep-me", NodeId: "far-node"},
				{Id: "offline-one", NodeId: "far-node", IsOffline: true},
				{Id: "loops-back", NodeId: nodeId.String()},
				{Id: selfInfo.OwnerId, NodeId: "far-node"},
			}})
		}}
		refreshUsers(repo, ev, streamer)
		require.Equal(t, []string{"keep-me"}, stored)
	})
}

func TestStreamGetUsersHandlerRefreshesWhenEmpty(t *testing.T) {
	nodeId, _ := nodeIdentity(t)
	selfInfo := warpnet.NodeInfo{ID: nodeId, OwnerId: "owner-1"}

	calls := 0
	repo := stubUserFetcher{
		listFn: func(*uint64, *string) ([]domain.User, string, error) {
			calls++
			if calls == 1 {
				return nil, "", nil
			}
			return []domain.User{{Id: "u1"}}, "cursor-1", nil
		},
		getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "other-node"}, nil
		},
	}
	streamer := stubUserStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
		return json.Marshal(event.UsersResponse{Users: []domain.User{{Id: "u1", NodeId: "far-node"}}})
	}}

	out, err := StreamGetUsersHandler(repo, streamer)(
		marshal(t, event.GetAllUsersEvent{UserId: "other-user"}), nil)
	require.NoError(t, err)
	require.Len(t, out.(event.UsersResponse).Users, 1)
	require.Equal(t, "cursor-1", out.(event.UsersResponse).Cursor)
}

package handler

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/bridge"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/require"
)

const wtfOwner = "owner-1"

var wtfAuth = stubAuth{owner: domain.Owner{UserId: wtfOwner, NodeId: "own-node", Login: "me"}}

func callWhoToFollow(t *testing.T, users stubUserFetcher, follows stubUserFollowsCounter, streamer stubUserStreamer, body any) (event.WhoToFollowResponse, error) {
	t.Helper()
	buf, ok := body.([]byte)
	if !ok {
		buf = marshal(t, body)
	}
	out, err := StreamGetWhoToFollowHandler(wtfAuth, users, follows, streamer)(buf, nil)
	if err != nil {
		return event.WhoToFollowResponse{}, err
	}
	return out.(event.WhoToFollowResponse), nil
}

func idsOf(users []domain.User) []string {
	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.Id)
	}
	return ids
}

// gatewayProbe records gateway calls; a missing call is checked after a grace
// period because the refresh runs in its own goroutine.
func gatewayProbe() (stubUserStreamer, func(time.Duration) bool) {
	asked := make(chan struct{}, 8)
	streamer := stubUserStreamer{
		nodeInfo: warpnet.NodeInfo{OwnerId: wtfOwner},
		genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			asked <- struct{}{}
			return json.Marshal(event.UsersResponse{})
		},
	}
	return streamer, func(wait time.Duration) bool {
		select {
		case <-asked:
			return true
		case <-time.After(wait):
			return false
		}
	}
}

func TestWhoToFollowHandlerRequestShapes(t *testing.T) {
	t.Run("a network listing failure fails the request before any page is read", func(t *testing.T) {
		listErr := errors.New("index unreadable")
		users := stubUserFetcher{
			listNetworksFn: func() ([]string, error) { return nil, listErr },
			whoToFollowFn: func(string, *uint64, *string) ([]domain.User, string, error) {
				t.Error("no page may be read without the network list")
				return nil, "", nil
			},
		}
		_, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, event.GetAllUsersEvent{UserId: wtfOwner})
		require.ErrorIs(t, err, listErr)
	})

	t.Run("a store with no networks answers empty lists, not nulls", func(t *testing.T) {
		users := stubUserFetcher{listNetworksFn: func() ([]string, error) { return nil, nil }}
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, event.GetAllUsersEvent{UserId: wtfOwner})
		require.NoError(t, err)
		wire, err := json.Marshal(resp)
		require.NoError(t, err)
		require.Contains(t, string(wire), `"networks":{}`)
		require.Contains(t, string(wire), `"users":[]`)
	})

	t.Run("a network listed twice is read once", func(t *testing.T) {
		var calls atomic.Int32
		users := stubUserFetcher{
			listNetworksFn: func() ([]string, error) {
				return []string{bridge.MastodonNetwork, bridge.MastodonNetwork, warpnet.WarpnetName, bridge.MastodonNetwork}, nil
			},
			whoToFollowFn: func(string, *uint64, *string) ([]domain.User, string, error) {
				calls.Add(1)
				return nil, "end", nil
			},
		}
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, event.GetAllUsersEvent{UserId: wtfOwner})
		require.NoError(t, err)
		require.Len(t, resp.Networks, 2)
		require.EqualValues(t, 2, calls.Load())
	})

	t.Run("one failing network fails the whole answer", func(t *testing.T) {
		pageErr := errors.New("threads page corrupt")
		networks := []string{warpnet.WarpnetName, bridge.MastodonNetwork, bridge.ThreadsNetwork, "nostr"}
		users := stubUserFetcher{
			listNetworksFn: func() ([]string, error) { return networks, nil },
			whoToFollowFn: func(network string, _ *uint64, _ *string) ([]domain.User, string, error) {
				if network == bridge.ThreadsNetwork {
					return nil, "", pageErr
				}
				return []domain.User{{Id: network + "-user", NodeId: network + "-node", Network: network}}, "end", nil
			},
		}
		out, err := StreamGetWhoToFollowHandler(wtfAuth, users, stubUserFollowsCounter{}, stubUserStreamer{})(
			marshal(t, event.GetAllUsersEvent{UserId: wtfOwner}), nil)
		require.ErrorIs(t, err, pageErr)
		require.Nil(t, out, "a half-built answer must not leak out")
	})

	t.Run("cursors win over a legacy cursor", func(t *testing.T) {
		var mx sync.Mutex
		asked := map[string]string{}
		users := stubUserFetcher{
			whoToFollowFn: func(network string, _ *uint64, cursor *string) ([]domain.User, string, error) {
				mx.Lock()
				asked[network] = *cursor
				mx.Unlock()
				return nil, "end", nil
			},
		}
		legacy := "legacy-cursor"
		ev := event.GetWhoToFollowEvent{
			GetAllUsersEvent: event.GetAllUsersEvent{UserId: wtfOwner, Cursor: &legacy},
			Cursors:          map[string]string{bridge.ThreadsNetwork: "t-cursor"},
		}
		_, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, ev)
		require.NoError(t, err)
		require.Equal(t, map[string]string{bridge.ThreadsNetwork: "t-cursor"}, asked)
	})

	t.Run("a finished legacy client does not start over", func(t *testing.T) {
		streamer, gatewayAsked := gatewayProbe()
		var asked []string
		users := stubUserFetcher{
			listNetworksFn: func() ([]string, error) {
				t.Error("a paging request must not list networks")
				return nil, nil
			},
			whoToFollowFn: func(network string, _ *uint64, cursor *string) ([]domain.User, string, error) {
				asked = append(asked, network+":"+*cursor)
				return nil, "end", nil
			},
		}
		end := "end"
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, streamer, event.GetAllUsersEvent{UserId: wtfOwner, Cursor: &end})
		require.NoError(t, err)
		require.Equal(t, []string{warpnet.WarpnetName + ":end"}, asked)
		require.Equal(t, "end", resp.Cursor)
		require.False(t, gatewayAsked(150*time.Millisecond), "a next page must not ask the gateway again")
	})

	t.Run("an empty legacy cursor is a first page", func(t *testing.T) {
		var listed atomic.Bool
		users := stubUserFetcher{
			listNetworksFn: func() ([]string, error) {
				listed.Store(true)
				return []string{"nostr"}, nil
			},
		}
		empty := ""
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, event.GetAllUsersEvent{UserId: wtfOwner, Cursor: &empty})
		require.NoError(t, err)
		require.True(t, listed.Load())
		require.Contains(t, resp.Networks, "nostr")
	})

	for name, body := range map[string]string{
		"null cursors":  `{"user_id":"owner-1","cursors":null}`,
		"empty cursors": `{"user_id":"owner-1","cursors":{}}`,
	} {
		t.Run(name+" are a first page", func(t *testing.T) {
			var listed atomic.Bool
			users := stubUserFetcher{listNetworksFn: func() ([]string, error) {
				listed.Store(true)
				return nil, nil
			}}
			_, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, []byte(body))
			require.NoError(t, err)
			require.True(t, listed.Load())
		})
	}

	for name, body := range map[string]string{
		"cursors as a string":      `{"user_id":"owner-1","cursors":"mastodon"}`,
		"cursors as a list":        `{"user_id":"owner-1","cursors":["mastodon"]}`,
		"a numeric cursor":         `{"user_id":"owner-1","cursors":{"mastodon":42}}`,
		"a nested cursor":          `{"user_id":"owner-1","cursors":{"mastodon":{"key":"x"}}}`,
		"a negative limit":         `{"user_id":"owner-1","limit":-1}`,
		"a user id that is a list": `{"user_id":["owner-1"]}`,
		"a truncated body":         `{"user_id":"owner-1","cursors":{"mastodon":`,
		"not json at all":          "\x00\xff\xfe",
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			users := stubUserFetcher{whoToFollowFn: func(string, *uint64, *string) ([]domain.User, string, error) {
				t.Error("a malformed request must not reach the store")
				return nil, "", nil
			}}
			_, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, []byte(body))
			require.Error(t, err)
		})
	}

	t.Run("cursors without a user id are still refused", func(t *testing.T) {
		_, err := callWhoToFollow(t, stubUserFetcher{}, stubUserFollowsCounter{}, stubUserStreamer{},
			event.GetWhoToFollowEvent{Cursors: map[string]string{warpnet.WarpnetName: "w"}})
		require.EqualError(t, err, "empty user id")
	})

	t.Run("a legacy request body still parses", func(t *testing.T) {
		var gotLimit uint64
		var gotCursor string
		users := stubUserFetcher{whoToFollowFn: func(_ string, limit *uint64, cursor *string) ([]domain.User, string, error) {
			gotLimit, gotCursor = *limit, *cursor
			return nil, "end", nil
		}}
		_, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{},
			[]byte(`{"user_id":"owner-1","cursor":"w-key","limit":7}`))
		require.NoError(t, err)
		require.EqualValues(t, 7, gotLimit)
		require.Equal(t, "w-key", gotCursor)
	})
}

func TestWhoToFollowHandlerGatewayRefresh(t *testing.T) {
	t.Run("the owner's paging request leaves the gateway alone", func(t *testing.T) {
		streamer, gatewayAsked := gatewayProbe()
		_, err := callWhoToFollow(t, stubUserFetcher{}, stubUserFollowsCounter{}, streamer,
			event.GetWhoToFollowEvent{
				GetAllUsersEvent: event.GetAllUsersEvent{UserId: wtfOwner},
				Cursors:          map[string]string{bridge.MastodonNetwork: "m"},
			})
		require.NoError(t, err)
		require.False(t, gatewayAsked(150*time.Millisecond))
	})

	t.Run("a visitor's first page leaves the gateway alone", func(t *testing.T) {
		streamer, gatewayAsked := gatewayProbe()
		_, err := callWhoToFollow(t, stubUserFetcher{}, stubUserFollowsCounter{}, streamer, event.GetAllUsersEvent{UserId: "visitor"})
		require.NoError(t, err)
		require.False(t, gatewayAsked(150*time.Millisecond))
	})

	t.Run("a dead gateway does not hold up the answer", func(t *testing.T) {
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		streamer := stubUserStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: wtfOwner},
			genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
				<-release
				return nil, errors.New("gateway unreachable")
			},
		}
		done := make(chan error, 1)
		go func() {
			_, err := callWhoToFollow(t, stubUserFetcher{}, stubUserFollowsCounter{}, streamer, event.GetAllUsersEvent{UserId: wtfOwner})
			done <- err
		}()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("the answer waited for the gateway")
		}
	})
}

func TestWhoToFollowHandlerFanOut(t *testing.T) {
	t.Run("every network is read with its own cursor and the shared limit", func(t *testing.T) {
		const n = 64
		cursors := make(map[string]string, n)
		for i := range n {
			cursors[fmt.Sprintf("net-%02d", i)] = fmt.Sprintf("cursor-%02d", i)
		}
		var mx sync.Mutex
		asked := map[string]string{}
		users := stubUserFetcher{whoToFollowFn: func(network string, limit *uint64, cursor *string) ([]domain.User, string, error) {
			if limit == nil || *limit != 3 {
				t.Errorf("%s: limit not passed through: %v", network, limit)
			}
			mx.Lock()
			asked[network] = *cursor
			mx.Unlock()
			*cursor = "scribbled by " + network
			return []domain.User{{Id: network + "-user", NodeId: network + "-node", Network: network}}, network + "-next", nil
		}}
		limit := uint64(3)
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{},
			event.GetWhoToFollowEvent{GetAllUsersEvent: event.GetAllUsersEvent{UserId: wtfOwner, Limit: &limit}, Cursors: cursors})
		require.NoError(t, err)
		require.Equal(t, cursors, asked)
		require.Len(t, resp.Networks, n)
		for network := range cursors {
			page := resp.Networks[network]
			require.Equal(t, network+"-next", page.Cursor)
			require.Equal(t, []string{network + "-user"}, idsOf(page.Users))
		}
	})

	t.Run("followings and the profile are read once, not per network", func(t *testing.T) {
		var followings, profiles atomic.Int32
		users := stubUserFetcher{
			listNetworksFn: func() ([]string, error) {
				return []string{warpnet.WarpnetName, bridge.MastodonNetwork, bridge.ThreadsNetwork, "nostr"}, nil
			},
			getFn: func(id string) (domain.User, error) {
				profiles.Add(1)
				return domain.User{Id: id, Network: warpnet.WarpnetName}, nil
			},
		}
		follows := stubUserFollowsCounter{getFollowingsFn: func(userId string, _ *uint64, _ *string) ([]string, string, error) {
			followings.Add(1)
			if userId != wtfOwner {
				t.Errorf("followings asked for %q, want the owner", userId)
			}
			return nil, "", nil
		}}
		_, err := callWhoToFollow(t, users, follows, stubUserStreamer{}, event.GetAllUsersEvent{UserId: "visitor"})
		require.NoError(t, err)
		require.EqualValues(t, 1, followings.Load())
		require.EqualValues(t, 1, profiles.Load())
	})

	t.Run("a visitor's followings never hide anyone; the owner's do", func(t *testing.T) {
		users := stubUserFetcher{
			getFn: func(id string) (domain.User, error) { return domain.User{Id: id, Network: warpnet.WarpnetName}, nil },
			whoToFollowFn: func(string, *uint64, *string) ([]domain.User, string, error) {
				return []domain.User{
					{Id: "owner-follows", NodeId: "n1", Network: warpnet.WarpnetName},
					{Id: "visitor-follows", NodeId: "n2", Network: warpnet.WarpnetName},
				}, "end", nil
			},
		}
		follows := stubUserFollowsCounter{getFollowingsFn: func(userId string, _ *uint64, _ *string) ([]string, string, error) {
			if userId == wtfOwner {
				return []string{"owner-follows"}, "", nil
			}
			return []string{"visitor-follows"}, "", nil
		}}
		resp, err := callWhoToFollow(t, users, follows, stubUserStreamer{}, event.GetAllUsersEvent{UserId: "visitor"})
		require.NoError(t, err)
		require.Equal(t, []string{"visitor-follows"}, idsOf(resp.Users))
	})

	t.Run("odd network names come back under the very same key", func(t *testing.T) {
		cursors := map[string]string{
			"":                       "c",
			"Mastodon":               "c",
			"nostr/../warpnet":       "c",
			"сеть":                   "c",
			strings.Repeat("x", 512): "c",
			"mastodon\x00":           "c",
		}
		users := stubUserFetcher{whoToFollowFn: func(network string, _ *uint64, _ *string) ([]domain.User, string, error) {
			return []domain.User{{Id: "u@" + network, NodeId: "gateway", Network: network}}, "", nil
		}}
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{},
			event.GetWhoToFollowEvent{GetAllUsersEvent: event.GetAllUsersEvent{UserId: wtfOwner}, Cursors: cursors})
		require.NoError(t, err)
		require.Len(t, resp.Networks, len(cursors))
		for network := range cursors {
			require.Contains(t, resp.Networks, network)
		}
	})
}

func TestWhoToFollowHandlerFilters(t *testing.T) {
	ownProfile := func(id string) (domain.User, error) { return domain.User{Id: id, Network: warpnet.WarpnetName}, nil }

	t.Run("one node per network: the same node may show up in two networks", func(t *testing.T) {
		users := stubUserFetcher{
			getFn:          ownProfile,
			listNetworksFn: func() ([]string, error) { return []string{warpnet.WarpnetName, bridge.MastodonNetwork}, nil },
			whoToFollowFn: func(network string, _ *uint64, _ *string) ([]domain.User, string, error) {
				return []domain.User{{Id: network + "-on-shared", NodeId: "shared-node", Network: network}}, "end", nil
			},
		}
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, event.GetAllUsersEvent{UserId: wtfOwner})
		require.NoError(t, err)
		require.Equal(t, []string{"warpnet-on-shared"}, idsOf(resp.Networks[warpnet.WarpnetName].Users))
		require.Equal(t, []string{"mastodon-on-shared"}, idsOf(resp.Networks[bridge.MastodonNetwork].Users))
	})

	t.Run("bridged accounts behind one gateway node are all kept", func(t *testing.T) {
		users := stubUserFetcher{
			getFn:          ownProfile,
			listNetworksFn: func() ([]string, error) { return []string{bridge.MastodonNetwork}, nil },
			whoToFollowFn: func(string, *uint64, *string) ([]domain.User, string, error) {
				return []domain.User{
					{Id: "a@mastodon.social", NodeId: "gateway", Network: bridge.MastodonNetwork},
					{Id: "b@mastodon.social", NodeId: "gateway", Network: bridge.MastodonNetwork},
					{Id: "c@mastodon.social", NodeId: "gateway", Network: bridge.MastodonNetwork},
				}, "end", nil
			},
		}
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, event.GetAllUsersEvent{UserId: wtfOwner})
		require.NoError(t, err)
		require.Len(t, resp.Networks[bridge.MastodonNetwork].Users, 3)
	})

	t.Run("the newest account of a node wins whatever order the store returns", func(t *testing.T) {
		older, newer := time.Now().Add(-time.Hour), time.Now()
		for name, page := range map[string][]domain.User{
			"newest first": {
				{Id: "new", NodeId: "n", Network: warpnet.WarpnetName, CreatedAt: newer},
				{Id: "old", NodeId: "n", Network: warpnet.WarpnetName, CreatedAt: older},
			},
			"newest last": {
				{Id: "old", NodeId: "n", Network: warpnet.WarpnetName, CreatedAt: older},
				{Id: "new", NodeId: "n", Network: warpnet.WarpnetName, CreatedAt: newer},
			},
			"newest in the middle": {
				{Id: "old", NodeId: "n", Network: warpnet.WarpnetName, CreatedAt: older},
				{Id: "new", NodeId: "n", Network: warpnet.WarpnetName, CreatedAt: newer},
				{Id: "older", NodeId: "n", Network: warpnet.WarpnetName, CreatedAt: older.Add(-time.Hour)},
			},
		} {
			t.Run(name, func(t *testing.T) {
				users := stubUserFetcher{getFn: ownProfile, whoToFollowFn: func(string, *uint64, *string) ([]domain.User, string, error) {
					return page, "end", nil
				}}
				resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, event.GetAllUsersEvent{UserId: wtfOwner})
				require.NoError(t, err)
				require.Equal(t, []string{"new"}, idsOf(resp.Users))
			})
		}
	})

	t.Run("a tie on creation time keeps the first account", func(t *testing.T) {
		at := time.Now()
		users := stubUserFetcher{getFn: ownProfile, whoToFollowFn: func(string, *uint64, *string) ([]domain.User, string, error) {
			return []domain.User{
				{Id: "first", NodeId: "n", Network: warpnet.WarpnetName, CreatedAt: at},
				{Id: "second", NodeId: "n", Network: warpnet.WarpnetName, CreatedAt: at},
			}, "end", nil
		}}
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, event.GetAllUsersEvent{UserId: wtfOwner})
		require.NoError(t, err)
		require.Equal(t, []string{"first"}, idsOf(resp.Users))
	})

	t.Run("native accounts with no node id are not merged together", func(t *testing.T) {
		users := stubUserFetcher{getFn: ownProfile, whoToFollowFn: func(string, *uint64, *string) ([]domain.User, string, error) {
			return []domain.User{
				{Id: "nodeless-1", Network: warpnet.WarpnetName},
				{Id: "nodeless-2", Network: warpnet.WarpnetName},
			}, "end", nil
		}}
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, event.GetAllUsersEvent{UserId: wtfOwner})
		require.NoError(t, err)
		require.Equal(t, []string{"nodeless-1", "nodeless-2"}, idsOf(resp.Users))
	})

	t.Run("the owner is hidden in every disguise and every network", func(t *testing.T) {
		users := stubUserFetcher{
			getFn:          ownProfile,
			listNetworksFn: func() ([]string, error) { return []string{warpnet.WarpnetName, bridge.MastodonNetwork}, nil },
			whoToFollowFn: func(network string, _ *uint64, _ *string) ([]domain.User, string, error) {
				return []domain.User{
					{Id: wtfOwner, NodeId: "own-node", Network: network},
					{Id: wtfOwner + "@warpnet-gw.example", NodeId: "gateway", Network: network},
					{Id: "alt-account", NodeId: "own-node", Network: network},
					{Id: "stranger-" + network, NodeId: "far-node", Network: network},
				}, "end", nil
			},
		}
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, event.GetAllUsersEvent{UserId: wtfOwner})
		require.NoError(t, err)
		for network, page := range resp.Networks {
			require.Equal(t, []string{"stranger-" + network}, idsOf(page.Users), network)
		}
	})

	t.Run("a node with no owner recommends nobody", func(t *testing.T) {
		users := stubUserFetcher{whoToFollowFn: func(string, *uint64, *string) ([]domain.User, string, error) {
			return []domain.User{{Id: "anyone", NodeId: "n1", Network: warpnet.WarpnetName}}, "end", nil
		}}
		out, err := StreamGetWhoToFollowHandler(stubAuth{}, users, stubUserFollowsCounter{}, stubUserStreamer{})(
			marshal(t, event.GetAllUsersEvent{UserId: "visitor"}), nil)
		require.NoError(t, err)
		require.Empty(t, out.(event.WhoToFollowResponse).Users)
	})

	t.Run("the owner's followings past the first 80 are not looked up", func(t *testing.T) {
		follows := stubUserFollowsCounter{getFollowingsFn: func(_ string, limit *uint64, cursor *string) ([]string, string, error) {
			require.NotNil(t, limit)
			require.EqualValues(t, 80, *limit)
			require.Nil(t, cursor)
			return nil, "more", nil
		}}
		_, err := callWhoToFollow(t, stubUserFetcher{}, follows, stubUserStreamer{}, event.GetAllUsersEvent{UserId: wtfOwner})
		require.NoError(t, err)
	})

	t.Run("a Mastodon profile shows Mastodon and nothing else", func(t *testing.T) {
		users := stubUserFetcher{
			getFn: func(id string) (domain.User, error) {
				return domain.User{Id: id, Network: bridge.MastodonNetwork}, nil
			},
			listNetworksFn: func() ([]string, error) {
				return []string{warpnet.WarpnetName, bridge.MastodonNetwork, bridge.ThreadsNetwork}, nil
			},
			whoToFollowFn: func(network string, _ *uint64, _ *string) ([]domain.User, string, error) {
				return []domain.User{{Id: network + "-user", NodeId: network + "-node", Network: network}}, "end", nil
			},
		}
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, event.GetAllUsersEvent{UserId: "alice@mastodon.social"})
		require.NoError(t, err)
		require.Empty(t, resp.Networks[warpnet.WarpnetName].Users)
		require.Empty(t, resp.Networks[bridge.ThreadsNetwork].Users)
		require.Equal(t, []string{"mastodon-user"}, idsOf(resp.Networks[bridge.MastodonNetwork].Users))
	})

	t.Run("another Warpnet profile shows no bridged networks", func(t *testing.T) {
		users := stubUserFetcher{
			listNetworksFn: func() ([]string, error) { return []string{warpnet.WarpnetName, bridge.MastodonNetwork}, nil },
			whoToFollowFn: func(network string, _ *uint64, _ *string) ([]domain.User, string, error) {
				return []domain.User{{Id: network + "-user", NodeId: network + "-node", Network: network}}, "end", nil
			},
		}
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, event.GetAllUsersEvent{UserId: "warp-peer"})
		require.NoError(t, err)
		require.Equal(t, []string{"warpnet-user"}, idsOf(resp.Users))
		require.Empty(t, resp.Networks[bridge.MastodonNetwork].Users)
	})

	t.Run("an all-offline page keeps its cursor so the client can move past it", func(t *testing.T) {
		users := stubUserFetcher{getFn: ownProfile, whoToFollowFn: func(string, *uint64, *string) ([]domain.User, string, error) {
			return []domain.User{
				{Id: "gone-1", NodeId: "n1", Network: warpnet.WarpnetName, IsOffline: true},
				{Id: "gone-2", NodeId: "n2", Network: warpnet.WarpnetName, IsOffline: true},
			}, "after-the-gone", nil
		}}
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, event.GetAllUsersEvent{UserId: wtfOwner})
		require.NoError(t, err)
		require.Empty(t, resp.Users)
		require.NotNil(t, resp.Users)
		require.Equal(t, "after-the-gone", resp.Cursor)
	})
}

func TestWhoToFollowHandlerWireCompat(t *testing.T) {
	users := stubUserFetcher{
		listNetworksFn: func() ([]string, error) { return []string{"", warpnet.WarpnetName, bridge.MastodonNetwork}, nil },
		whoToFollowFn: func(network string, _ *uint64, _ *string) ([]domain.User, string, error) {
			return []domain.User{{Id: "user-of-" + network, NodeId: "node-of-" + network, Network: network}}, "next-" + network, nil
		},
	}

	t.Run("an older client reads the Warpnet page from the flat fields", func(t *testing.T) {
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{}, event.GetAllUsersEvent{UserId: wtfOwner})
		require.NoError(t, err)
		wire, err := json.Marshal(resp)
		require.NoError(t, err)

		var legacy event.UsersResponse
		require.NoError(t, json.Unmarshal(wire, &legacy))
		require.Equal(t, []string{"user-of-warpnet"}, idsOf(legacy.Users))
		require.Equal(t, "next-warpnet", legacy.Cursor)

		var current event.WhoToFollowResponse
		require.NoError(t, json.Unmarshal(wire, &current))
		require.Len(t, current.Networks, 3)
		require.Equal(t, "next-", current.Networks[""].Cursor)
	})

	t.Run("untagged rows are never promoted to the Warpnet page", func(t *testing.T) {
		resp, err := callWhoToFollow(t, users, stubUserFollowsCounter{}, stubUserStreamer{},
			event.GetWhoToFollowEvent{GetAllUsersEvent: event.GetAllUsersEvent{UserId: wtfOwner}, Cursors: map[string]string{"": "x"}})
		require.NoError(t, err)
		require.Empty(t, resp.Users)
		require.Empty(t, resp.Cursor)
		require.Equal(t, []string{"user-of-"}, idsOf(resp.Networks[""].Users))
	})
}

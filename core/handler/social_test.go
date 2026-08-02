/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU Affero General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU Affero General Public License for more details.

 You should have received a copy of the GNU Affero General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package handler

import (
	"errors"
	"strings"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	bt, err := json.Marshal(v)
	require.NoError(t, err)
	return bt
}

// ---------------------------------------------------------------------------
// Mutes listing.
// ---------------------------------------------------------------------------

func TestStreamGetMutesHandler(t *testing.T) {
	t.Run("malformed payload is rejected", func(t *testing.T) {
		_, err := StreamGetMutesHandler(stubMutesRepo{})([]byte("{"), nil)
		assert.Error(t, err)
	})

	t.Run("empty user id is rejected", func(t *testing.T) {
		_, err := StreamGetMutesHandler(stubMutesRepo{})(mustJSON(t, event.GetMutesEvent{}), nil)
		assert.Error(t, err, "an anonymous request must not enumerate someone's mute list")
	})

	t.Run("storage failure surfaces", func(t *testing.T) {
		repo := stubMutesRepo{listFn: func(string, *uint64, *string) ([]string, string, error) {
			return nil, "", errors.New("db down")
		}}
		_, err := StreamGetMutesHandler(repo)(mustJSON(t, event.GetMutesEvent{UserId: "u1"}), nil)
		assert.Error(t, err)
	})

	t.Run("ids and cursor are passed through", func(t *testing.T) {
		var gotUser string
		var gotLimit *uint64
		var gotCursor *string
		repo := stubMutesRepo{listFn: func(m string, l *uint64, c *string) ([]string, string, error) {
			gotUser, gotLimit, gotCursor = m, l, c
			return []string{"muted-a", "muted-b"}, "next-page", nil
		}}

		limit := uint64(2)
		cursor := "page-1"
		out, err := StreamGetMutesHandler(repo)(mustJSON(t, event.GetMutesEvent{
			UserId: "u1", Limit: &limit, Cursor: &cursor,
		}), nil)
		require.NoError(t, err)

		resp, ok := out.(event.GetMutesResponse)
		require.True(t, ok)
		assert.Equal(t, []domain.ID{"muted-a", "muted-b"}, resp.Ids)
		assert.Equal(t, "next-page", resp.Cursor)
		assert.Equal(t, "u1", gotUser)
		require.NotNil(t, gotLimit)
		assert.Equal(t, uint64(2), *gotLimit)
		require.NotNil(t, gotCursor)
		assert.Equal(t, "page-1", *gotCursor)
	})

	t.Run("empty mute list is an empty slice not nil", func(t *testing.T) {
		repo := stubMutesRepo{listFn: func(string, *uint64, *string) ([]string, string, error) {
			return nil, "", nil
		}}
		out, err := StreamGetMutesHandler(repo)(mustJSON(t, event.GetMutesEvent{UserId: "u1"}), nil)
		require.NoError(t, err)

		resp := out.(event.GetMutesResponse)
		assert.NotNil(t, resp.Ids, "clients decode an absent array as a parse failure")
		assert.Empty(t, resp.Ids)
	})
}

// ---------------------------------------------------------------------------
// Follow requests — the locked-account gate.
// ---------------------------------------------------------------------------

func TestStreamGetFollowRequestsHandler(t *testing.T) {
	t.Run("malformed payload", func(t *testing.T) {
		_, err := StreamGetFollowRequestsHandler(stubFollowRepo{})([]byte("{"), nil)
		assert.Error(t, err)
	})

	t.Run("empty user id", func(t *testing.T) {
		_, err := StreamGetFollowRequestsHandler(stubFollowRepo{})(
			mustJSON(t, event.GetFollowRequestsEvent{}), nil)
		assert.Error(t, err, "pending requests are private to the account owner")
	})

	t.Run("storage failure surfaces", func(t *testing.T) {
		repo := stubFollowRepo{listFollowRequestsFn: func(string, *uint64, *string) ([]string, string, error) {
			return nil, "", errors.New("db down")
		}}
		_, err := StreamGetFollowRequestsHandler(repo)(
			mustJSON(t, event.GetFollowRequestsEvent{UserId: "u1"}), nil)
		assert.Error(t, err)
	})

	t.Run("requests and cursor pass through", func(t *testing.T) {
		repo := stubFollowRepo{listFollowRequestsFn: func(string, *uint64, *string) ([]string, string, error) {
			return []string{"pending-1", "pending-2"}, "cur", nil
		}}
		out, err := StreamGetFollowRequestsHandler(repo)(
			mustJSON(t, event.GetFollowRequestsEvent{UserId: "u1"}), nil)
		require.NoError(t, err)

		resp := out.(event.GetFollowRequestsResponse)
		assert.Equal(t, []domain.ID{"pending-1", "pending-2"}, resp.FollowerIds)
		assert.Equal(t, "cur", resp.Cursor)
	})

	t.Run("no pending requests yields an empty slice", func(t *testing.T) {
		out, err := StreamGetFollowRequestsHandler(stubFollowRepo{})(
			mustJSON(t, event.GetFollowRequestsEvent{UserId: "u1"}), nil)
		require.NoError(t, err)
		assert.NotNil(t, out.(event.GetFollowRequestsResponse).FollowerIds)
	})
}

func TestStreamAuthorizeFollowRequestHandler(t *testing.T) {
	t.Run("malformed payload", func(t *testing.T) {
		_, err := StreamAuthorizeFollowRequestHandler(stubFollowRepo{})([]byte("{"), nil)
		assert.Error(t, err)
	})

	t.Run("missing identifiers are rejected", func(t *testing.T) {
		_, err := StreamAuthorizeFollowRequestHandler(stubFollowRepo{})(
			mustJSON(t, event.FollowRequestActionEvent{FollowerId: "f"}), nil)
		assert.Error(t, err)

		_, err = StreamAuthorizeFollowRequestHandler(stubFollowRepo{})(
			mustJSON(t, event.FollowRequestActionEvent{UserId: "u"}), nil)
		assert.Error(t, err)
	})

	// Approving must create the edge in the follower→target direction. Getting
	// this backwards would make the account owner follow their own applicant.
	t.Run("approval creates the follow in the right direction", func(t *testing.T) {
		var followFrom, followTo string
		var removedTarget, removedFollower string

		repo := stubFollowRepo{
			followFn: func(from, to string) error {
				followFrom, followTo = from, to
				return nil
			},
			removeFollowRequestFn: func(target, follower string) error {
				removedTarget, removedFollower = target, follower
				return nil
			},
		}

		out, err := StreamAuthorizeFollowRequestHandler(repo)(
			mustJSON(t, event.FollowRequestActionEvent{UserId: "owner", FollowerId: "applicant"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.Accepted, out)

		assert.Equal(t, "applicant", followFrom, "the applicant follows the owner")
		assert.Equal(t, "owner", followTo)
		assert.Equal(t, "owner", removedTarget, "the pending row is keyed by the owner")
		assert.Equal(t, "applicant", removedFollower)
	})

	// If the follow cannot be stored, the request must stay pending so the
	// owner can retry — silently dropping it would strand the applicant.
	t.Run("failed follow leaves the request pending", func(t *testing.T) {
		removeCalled := false
		repo := stubFollowRepo{
			followFn: func(string, string) error { return errors.New("storage full") },
			removeFollowRequestFn: func(string, string) error {
				removeCalled = true
				return nil
			},
		}

		_, err := StreamAuthorizeFollowRequestHandler(repo)(
			mustJSON(t, event.FollowRequestActionEvent{UserId: "owner", FollowerId: "applicant"}), nil)
		assert.Error(t, err)
		assert.False(t, removeCalled, "the pending request must survive a failed approval")
	})

	t.Run("failed cleanup surfaces", func(t *testing.T) {
		repo := stubFollowRepo{
			removeFollowRequestFn: func(string, string) error { return errors.New("db down") },
		}
		_, err := StreamAuthorizeFollowRequestHandler(repo)(
			mustJSON(t, event.FollowRequestActionEvent{UserId: "owner", FollowerId: "applicant"}), nil)
		assert.Error(t, err)
	})
}

func TestStreamRejectFollowRequestHandler(t *testing.T) {
	t.Run("malformed payload", func(t *testing.T) {
		_, err := StreamRejectFollowRequestHandler(stubFollowRepo{})([]byte("{"), nil)
		assert.Error(t, err)
	})

	t.Run("missing identifiers are rejected", func(t *testing.T) {
		_, err := StreamRejectFollowRequestHandler(stubFollowRepo{})(
			mustJSON(t, event.FollowRequestActionEvent{FollowerId: "f"}), nil)
		assert.Error(t, err)

		_, err = StreamRejectFollowRequestHandler(stubFollowRepo{})(
			mustJSON(t, event.FollowRequestActionEvent{UserId: "u"}), nil)
		assert.Error(t, err)
	})

	// Rejecting must never create a follow edge — that is the whole point.
	t.Run("rejection removes the request and follows nobody", func(t *testing.T) {
		followCalled := false
		var removedTarget, removedFollower string

		repo := stubFollowRepo{
			followFn: func(string, string) error {
				followCalled = true
				return nil
			},
			removeFollowRequestFn: func(target, follower string) error {
				removedTarget, removedFollower = target, follower
				return nil
			},
		}

		out, err := StreamRejectFollowRequestHandler(repo)(
			mustJSON(t, event.FollowRequestActionEvent{UserId: "owner", FollowerId: "applicant"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.Accepted, out)
		assert.False(t, followCalled, "a rejected applicant must not end up following the owner")
		assert.Equal(t, "owner", removedTarget)
		assert.Equal(t, "applicant", removedFollower)
	})

	t.Run("storage failure surfaces", func(t *testing.T) {
		repo := stubFollowRepo{
			removeFollowRequestFn: func(string, string) error { return errors.New("db down") },
		}
		_, err := StreamRejectFollowRequestHandler(repo)(
			mustJSON(t, event.FollowRequestActionEvent{UserId: "owner", FollowerId: "applicant"}), nil)
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// Peer-blocklist escalation — a social block must reach the network layer.
// ---------------------------------------------------------------------------

type fakeEngagementStreamer struct {
	info     warpnet.NodeInfo
	response []byte
	err      error

	calls []string
}

func (f *fakeEngagementStreamer) NodeInfo() warpnet.NodeInfo { return f.info }

func (f *fakeEngagementStreamer) GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
	f.calls = append(f.calls, nodeId+" "+string(path))
	return f.response, f.err
}

func TestEscalateToPeerBlocklist(t *testing.T) {
	t.Run("nil dependencies are a no-op", func(t *testing.T) {
		assert.NotPanics(t, func() { escalateToPeerBlocklist(nil, &stubPeerBlocklister{}, "u") })
		assert.NotPanics(t, func() { escalateToPeerBlocklist(stubBlockUserResolver{}, nil, "u") })
	})

	t.Run("blocks the target's node", func(t *testing.T) {
		peers := &stubPeerBlocklister{}
		escalateToPeerBlocklist(stubBlockUserResolver{}, peers, "victim")
		assert.Equal(t, []string{"node-victim"}, peers.captured)
	})

	// A user we have never seen has no node to ban — the social block still
	// stands, so this must stay silent rather than blow up the handler.
	t.Run("unknown user is skipped quietly", func(t *testing.T) {
		peers := &stubPeerBlocklister{}
		users := stubBlockUserResolver{getFn: func(string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}
		escalateToPeerBlocklist(users, peers, "ghost")
		assert.Empty(t, peers.captured)
	})

	t.Run("lookup failure is swallowed", func(t *testing.T) {
		peers := &stubPeerBlocklister{}
		users := stubBlockUserResolver{getFn: func(string) (domain.User, error) {
			return domain.User{}, errors.New("db down")
		}}
		escalateToPeerBlocklist(users, peers, "victim")
		assert.Empty(t, peers.captured)
	})

	// A bridged or alias account has no node of its own; banning "" would ban
	// nothing at best and everything at worst.
	t.Run("user without a node id is skipped", func(t *testing.T) {
		peers := &stubPeerBlocklister{}
		users := stubBlockUserResolver{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: ""}, nil
		}}
		escalateToPeerBlocklist(users, peers, "bridged")
		assert.Empty(t, peers.captured)
	})

	t.Run("blocklist failure does not propagate", func(t *testing.T) {
		peers := &stubPeerBlocklister{blocklistFn: func(string) error { return errors.New("nope") }}
		assert.NotPanics(t, func() {
			escalateToPeerBlocklist(stubBlockUserResolver{}, peers, "victim")
		})
	})
}

func TestRemovePeerBlocklist(t *testing.T) {
	t.Run("nil dependencies are a no-op", func(t *testing.T) {
		assert.NotPanics(t, func() { removePeerBlocklist(nil, &stubPeerBlocklister{}, "u") })
		assert.NotPanics(t, func() { removePeerBlocklist(stubBlockUserResolver{}, nil, "u") })
	})

	t.Run("unblocks the target's node", func(t *testing.T) {
		peers := &stubPeerBlocklister{}
		removePeerBlocklist(stubBlockUserResolver{}, peers, "victim")
		assert.Equal(t, []string{"node-victim"}, peers.removed)
	})

	t.Run("unknown user is skipped", func(t *testing.T) {
		peers := &stubPeerBlocklister{}
		users := stubBlockUserResolver{getFn: func(string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}
		removePeerBlocklist(users, peers, "ghost")
		assert.Empty(t, peers.removed)
	})

	t.Run("lookup failure is swallowed", func(t *testing.T) {
		peers := &stubPeerBlocklister{}
		users := stubBlockUserResolver{getFn: func(string) (domain.User, error) {
			return domain.User{}, errors.New("db down")
		}}
		removePeerBlocklist(users, peers, "victim")
		assert.Empty(t, peers.removed)
	})

	t.Run("user without a node id is skipped", func(t *testing.T) {
		peers := &stubPeerBlocklister{}
		users := stubBlockUserResolver{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id}, nil
		}}
		removePeerBlocklist(users, peers, "bridged")
		assert.Empty(t, peers.removed)
	})
}

// ---------------------------------------------------------------------------
// Engagement forwarding — reading likers/retweeters off the author's node.
// ---------------------------------------------------------------------------

func TestForwardToOwner(t *testing.T) {
	ev := event.GetTweetLikersEvent{TweetId: "t1", OwnerUserId: "author"}

	t.Run("no owner or no streamer stays local", func(t *testing.T) {
		_, ok, err := forwardToOwner("", &fakeEngagementStreamer{}, stubLikedUserFetcher{}, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		assert.False(t, ok)

		_, ok, err = forwardToOwner("author", nil, stubLikedUserFetcher{}, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	// Asking ourselves over the network would be an infinite loop.
	t.Run("owner is this node's owner stays local", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{info: warpnet.NodeInfo{OwnerId: "author"}}
		_, ok, err := forwardToOwner("author", streamer, stubLikedUserFetcher{}, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Empty(t, streamer.calls, "a node must never stream to itself")
	})

	t.Run("unknown owner stays local", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{info: warpnet.NodeInfo{OwnerId: "me"}}
		users := stubLikedUserFetcher{getFn: func(string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}
		_, ok, err := forwardToOwner("author", streamer, users, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Empty(t, streamer.calls)
	})

	t.Run("owner lookup error propagates", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{info: warpnet.NodeInfo{OwnerId: "me"}}
		users := stubLikedUserFetcher{getFn: func(string) (domain.User, error) {
			return domain.User{}, errors.New("db down")
		}}
		_, ok, err := forwardToOwner("author", streamer, users, event.PUBLIC_GET_TWEET_LIKERS, ev)
		assert.Error(t, err)
		assert.False(t, ok)
	})

	// An offline author must degrade to whatever this node already knows,
	// not surface an error to the reader.
	t.Run("offline owner degrades to the local index", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{
			info: warpnet.NodeInfo{OwnerId: "me"},
			err:  warpnet.ErrNodeIsOffline,
		}
		users := stubLikedUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "remote-node"}, nil
		}}
		_, ok, err := forwardToOwner("author", streamer, users, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("other stream errors propagate", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{
			info: warpnet.NodeInfo{OwnerId: "me"},
			err:  errors.New("connection reset"),
		}
		users := stubLikedUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "remote-node"}, nil
		}}
		_, ok, err := forwardToOwner("author", streamer, users, event.PUBLIC_GET_TWEET_LIKERS, ev)
		assert.Error(t, err)
		assert.False(t, ok)
	})

	// A malicious or simply outdated peer can answer with garbage — the reader
	// must still get this node's own view instead of an error.
	t.Run("unparseable remote answer degrades to the local index", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{
			info:     warpnet.NodeInfo{OwnerId: "me"},
			response: []byte("<html>not json</html>"),
		}
		users := stubLikedUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "remote-node"}, nil
		}}
		_, ok, err := forwardToOwner("author", streamer, users, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("empty remote page degrades to the local index", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{
			info:     warpnet.NodeInfo{OwnerId: "me"},
			response: mustJSON(t, event.UsersResponse{}),
		}
		users := stubLikedUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "remote-node"}, nil
		}}
		_, ok, err := forwardToOwner("author", streamer, users, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("non-empty remote page wins", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{
			info: warpnet.NodeInfo{OwnerId: "me"},
			response: mustJSON(t, event.UsersResponse{
				Cursor: "remote-cursor",
				Users:  []domain.User{{Id: "liker-1"}},
			}),
		}
		users := stubLikedUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "remote-node"}, nil
		}}
		out, ok, err := forwardToOwner("author", streamer, users, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "remote-cursor", out.Cursor)
		require.Len(t, out.Users, 1)
		assert.Equal(t, "liker-1", out.Users[0].Id)
		require.Len(t, streamer.calls, 1)
		assert.True(t, strings.HasPrefix(streamer.calls[0], "remote-node "))
	})
}

func TestHydrateUsers(t *testing.T) {
	t.Run("no ids yields nothing", func(t *testing.T) {
		assert.Nil(t, hydrateUsers(stubLikedUserFetcher{}, nil))
		assert.Nil(t, hydrateUsers(stubLikedUserFetcher{}, []string{}))
	})

	t.Run("batch result is used as-is", func(t *testing.T) {
		users := stubLikedUserFetcher{batchFn: func(ids ...string) ([]domain.User, error) {
			return []domain.User{{Id: "a"}, {Id: "b"}}, nil
		}}
		got := hydrateUsers(users, []string{"a", "b"})
		assert.Len(t, got, 2)
	})

	// A failing batch read must not blank the whole engagement list.
	t.Run("batch failure falls back to per-id reads", func(t *testing.T) {
		users := stubLikedUserFetcher{
			batchFn: func(ids ...string) ([]domain.User, error) {
				return nil, errors.New("batch exploded")
			},
			getFn: func(id string) (domain.User, error) {
				return domain.User{Id: id, Username: "user-" + id}, nil
			},
		}
		got := hydrateUsers(users, []string{"a", "b"})
		require.Len(t, got, 2)
		assert.Equal(t, "user-a", got[0].Username)
	})

	// One deleted account must not erase everyone else from the list.
	t.Run("individually missing users are skipped not fatal", func(t *testing.T) {
		users := stubLikedUserFetcher{
			batchFn: func(ids ...string) ([]domain.User, error) { return nil, nil },
			getFn: func(id string) (domain.User, error) {
				if id == "gone" {
					return domain.User{}, database.ErrUserNotFound
				}
				return domain.User{Id: id}, nil
			},
		}
		got := hydrateUsers(users, []string{"a", "gone", "b"})
		require.Len(t, got, 2)
		assert.Equal(t, "a", got[0].Id)
		assert.Equal(t, "b", got[1].Id)
	})

	t.Run("all users missing yields an empty list", func(t *testing.T) {
		users := stubLikedUserFetcher{
			batchFn: func(ids ...string) ([]domain.User, error) { return nil, errors.New("x") },
			getFn: func(string) (domain.User, error) {
				return domain.User{}, database.ErrUserNotFound
			},
		}
		assert.Empty(t, hydrateUsers(users, []string{"a", "b"}))
	})
}

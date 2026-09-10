//nolint:all
package handler

import (
	"errors"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/mastodon"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/stretchr/testify/require"
)

func TestFilterHandlerBranches(t *testing.T) {
	const userId, filterId, kwId = "user-1", "filter-1", "kw-1"
	repoErr := errors.New("filter store down")

	t.Run("invalid payloads", func(t *testing.T) {
		for name, h := range map[string]warpnet.WarpHandlerFunc{
			"get":      StreamGetFilterHandler(stubFilterRepo{}),
			"list":     StreamGetFiltersHandler(stubFilterRepo{}),
			"create":   StreamNewFilterHandler(stubFilterRepo{}),
			"update":   StreamUpdateFilterHandler(stubFilterRepo{}),
			"delete":   StreamDeleteFilterHandler(stubFilterRepo{}),
			"addKw":    StreamAddFilterKeywordHandler(stubFilterRepo{}),
			"updateKw": StreamUpdateFilterKeywordHandler(stubFilterRepo{}),
			"deleteKw": StreamDeleteFilterKeywordHandler(stubFilterRepo{}),
		} {
			t.Run(name, func(t *testing.T) {
				_, err := h([]byte("{"), nil)
				require.Error(t, err)
			})
		}
	})

	t.Run("store failures surface", func(t *testing.T) {
		_, err := StreamGetFilterHandler(stubFilterRepo{getFn: func(string, string) (domain.Filter, error) {
			return domain.Filter{}, repoErr
		}})(marshal(t, event.GetFilterEvent{UserId: userId, FilterId: filterId}), nil)
		require.ErrorIs(t, err, repoErr)

		_, err = StreamGetFiltersHandler(stubFilterRepo{listFn: func(string, *uint64, *string) ([]domain.Filter, string, error) {
			return nil, "", repoErr
		}})(marshal(t, event.GetFiltersEvent{UserId: userId}), nil)
		require.ErrorIs(t, err, repoErr)

		_, err = StreamUpdateFilterHandler(stubFilterRepo{updateFn: func(string, domain.Filter) (domain.Filter, error) {
			return domain.Filter{}, repoErr
		}})(marshal(t, event.UpdateFilterEvent{UserId: userId, Id: filterId, Title: "t"}), nil)
		require.ErrorIs(t, err, repoErr)

		_, err = StreamDeleteFilterHandler(stubFilterRepo{deleteFn: func(string, string) error {
			return repoErr
		}})(marshal(t, event.DeleteFilterEvent{UserId: userId, FilterId: filterId}), nil)
		require.ErrorIs(t, err, repoErr)

		_, err = StreamAddFilterKeywordHandler(stubFilterRepo{addKwFn: func(string, string, domain.FilterKeyword) (domain.FilterKeyword, error) {
			return domain.FilterKeyword{}, repoErr
		}})(marshal(t, event.AddFilterKeywordEvent{UserId: userId, FilterId: filterId, Keyword: "kw"}), nil)
		require.ErrorIs(t, err, repoErr)

		_, err = StreamUpdateFilterKeywordHandler(stubFilterRepo{updateKwFn: func(string, domain.FilterKeyword) (domain.FilterKeyword, error) {
			return domain.FilterKeyword{}, repoErr
		}})(marshal(t, event.UpdateFilterKeywordEvent{UserId: userId, KeywordId: kwId, Keyword: "kw"}), nil)
		require.ErrorIs(t, err, repoErr)

		_, err = StreamDeleteFilterKeywordHandler(stubFilterRepo{deleteKwFn: func(string, string) error {
			return repoErr
		}})(marshal(t, event.DeleteFilterKeywordEvent{UserId: userId, KeywordId: kwId}), nil)
		require.ErrorIs(t, err, repoErr)
	})
}

func TestBlockMuteHandlerBranches(t *testing.T) {
	const blocker, blockee = "blocker-1", "blockee-1"
	repoErr := errors.New("block store down")

	t.Run("unblock guards", func(t *testing.T) {
		h := StreamUnblockHandler(stubBlocksRepo{}, stubBlockUserResolver{}, &stubPeerBlocklister{})
		_, err := h(marshal(t, event.UnblockEvent{BlockerId: blocker}), nil)
		require.Error(t, err)

		failing := StreamUnblockHandler(stubBlocksRepo{unblockFn: func(string, string) error {
			return repoErr
		}}, stubBlockUserResolver{}, &stubPeerBlocklister{})
		_, err = failing(marshal(t, event.UnblockEvent{BlockerId: blocker, BlockeeId: blockee}), nil)
		require.ErrorIs(t, err, repoErr)
	})

	t.Run("blocks listing failure surfaces", func(t *testing.T) {
		h := StreamGetBlocksHandler(stubBlocksRepo{listFn: func(string, *uint64, *string) ([]string, string, error) {
			return nil, "", repoErr
		}})
		_, err := h(marshal(t, event.GetBlocksEvent{UserId: blocker}), nil)
		require.ErrorIs(t, err, repoErr)
	})

	t.Run("mute guards", func(t *testing.T) {
		h := StreamMuteHandler(stubMutesRepo{})
		_, err := h([]byte("{"), nil)
		require.Error(t, err)

		_, err = h(marshal(t, event.MuteEvent{MuterId: blocker}), nil)
		require.Error(t, err)

		failing := StreamMuteHandler(stubMutesRepo{muteFn: func(string, string) error { return repoErr }})
		_, err = failing(marshal(t, event.MuteEvent{MuterId: blocker, MuteeId: blockee}), nil)
		require.ErrorIs(t, err, repoErr)
	})

	t.Run("unmute guards", func(t *testing.T) {
		h := StreamUnmuteHandler(stubMutesRepo{})
		_, err := h([]byte("{"), nil)
		require.Error(t, err)

		_, err = h(marshal(t, event.UnmuteEvent{MuterId: blocker}), nil)
		require.Error(t, err)

		failing := StreamUnmuteHandler(stubMutesRepo{unmuteFn: func(string, string) error { return repoErr }})
		_, err = failing(marshal(t, event.UnmuteEvent{MuterId: blocker, MuteeId: blockee}), nil)
		require.ErrorIs(t, err, repoErr)
	})

	t.Run("peer blocklist removal failure is tolerated", func(t *testing.T) {
		users := stubBlockUserResolver{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "peer-node"}, nil
		}}
		blocklister := &failingBlocklistRemover{}
		h := StreamUnblockHandler(stubBlocksRepo{}, users, blocklister)
		out, err := h(marshal(t, event.UnblockEvent{BlockerId: blocker, BlockeeId: blockee}), nil)
		require.NoError(t, err)
		require.Equal(t, event.Accepted, out)
		require.True(t, blocklister.called)
	})
}

// failingBlocklistRemover makes the peer-level blocklist removal fail so the
// handler's "log and carry on" branch runs.
type failingBlocklistRemover struct{ called bool }

func (f *failingBlocklistRemover) BlocklistPermanent(string) error { return nil }
func (f *failingBlocklistRemover) BlocklistRemove(string) error {
	f.called = true
	return errors.New("peer blocklist down")
}

func TestStreamGetWhoToFollowHandlerBranches(t *testing.T) {
	const ownerId = "owner-1"
	owner := domain.Owner{UserId: ownerId, NodeId: "own-node", Username: "me"}
	auth := stubAuth{owner: owner}

	t.Run("followings lookup failure is tolerated", func(t *testing.T) {
		follows := stubUserFollowsCounter{getFollowingsFn: func(string, *uint64, *string) ([]string, string, error) {
			return nil, "", errors.New("down")
		}}
		_, err := StreamGetWhoToFollowHandler(auth, stubUserFetcher{}, follows)(
			marshal(t, event.GetAllUsersEvent{UserId: ownerId}), nil)
		require.NoError(t, err)
	})

	t.Run("profile lookup failure falls back to the owner", func(t *testing.T) {
		users := stubUserFetcher{
			getFn: func(string) (domain.User, error) { return domain.User{}, errors.New("down") },
			whoToFollowFn: func(*uint64, *string) ([]domain.User, string, error) {
				return []domain.User{{Id: "cand-1", NodeId: "far-node", Network: warpnet.WarpnetName}}, "", nil
			},
		}
		out, err := StreamGetWhoToFollowHandler(auth, users, stubUserFollowsCounter{})(
			marshal(t, event.GetAllUsersEvent{UserId: ownerId}), nil)
		require.NoError(t, err)
		require.Len(t, out.(event.UsersResponse).Users, 1)
	})

	t.Run("filters offline, self, followed and cross-network candidates", func(t *testing.T) {
		older := time.Now().Add(-time.Hour)
		newer := time.Now()

		users := stubUserFetcher{
			getFn: func(id string) (domain.User, error) {
				return domain.User{Id: id, Network: warpnet.WarpnetName}, nil
			},
			whoToFollowFn: func(*uint64, *string) ([]domain.User, string, error) {
				return []domain.User{
					{Id: "offline", NodeId: "n1", Network: warpnet.WarpnetName, IsOffline: true},
					{Id: ownerId, NodeId: "own-node", Network: warpnet.WarpnetName},
					{Id: "already-followed", NodeId: "n2", Network: warpnet.WarpnetName},
					{Id: "shared-node-old", NodeId: "n3", Network: warpnet.WarpnetName, CreatedAt: older},
					{Id: "shared-node-new", NodeId: "n3", Network: warpnet.WarpnetName, CreatedAt: newer},
					{Id: "mastodon-1", NodeId: "n4", Network: mastodon.Network},
				}, "cursor-1", nil
			},
		}
		follows := stubUserFollowsCounter{getFollowingsFn: func(string, *uint64, *string) ([]string, string, error) {
			return []string{"already-followed"}, "", nil
		}}

		out, err := StreamGetWhoToFollowHandler(auth, users, follows)(
			marshal(t, event.GetAllUsersEvent{UserId: ownerId}), nil)
		require.NoError(t, err)

		resp := out.(event.UsersResponse)
		ids := make([]string, 0, len(resp.Users))
		for _, u := range resp.Users {
			ids = append(ids, u.Id)
		}
		// one entry per node: the newer of the two n3 records wins
		require.Contains(t, ids, "shared-node-new")
		require.NotContains(t, ids, "shared-node-old")
		require.NotContains(t, ids, "offline")
		require.NotContains(t, ids, ownerId)
		require.NotContains(t, ids, "already-followed")
		require.Equal(t, "cursor-1", resp.Cursor)
	})

	t.Run("a foreign-network profile hides other networks", func(t *testing.T) {
		users := stubUserFetcher{
			getFn: func(id string) (domain.User, error) {
				return domain.User{Id: id, Network: mastodon.Network}, nil
			},
			whoToFollowFn: func(*uint64, *string) ([]domain.User, string, error) {
				return []domain.User{{Id: "warp-1", NodeId: "n1", Network: warpnet.WarpnetName}}, "", nil
			},
		}
		out, err := StreamGetWhoToFollowHandler(auth, users, stubUserFollowsCounter{})(
			marshal(t, event.GetAllUsersEvent{UserId: "someone-else"}), nil)
		require.NoError(t, err)
		require.Empty(t, out.(event.UsersResponse).Users)
	})
}

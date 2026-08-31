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
package database

import (
	"fmt"
	"testing"

	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/require"
)

// faultOp names a transaction method and which call of it should fail.
type faultOp struct {
	method string
	nth    int
}

func op(method string) faultOp         { return faultOp{method, 1} }
func opN(method string, n int) faultOp { return faultOp{method, n} }

func (o faultOp) String() string { return fmt.Sprintf("%s#%d", o.method, o.nth) }

// faultCase drives one repo method through NewTxn failure and through each
// listed storage fault, requiring every one of them to surface to the caller.
type faultCase struct {
	name string
	// seed runs before the fault is armed, so fixtures don't consume call counts.
	seed func(t *testing.T, s *faultStore)
	run  func(s *faultStore) error
	ops  []faultOp
	// skipNewTxn is set for methods that never open a transaction themselves.
	skipNewTxn bool
}

func runFaultCases(t *testing.T, cases []faultCase) {
	t.Helper()
	for _, c := range cases {
		if !c.skipNewTxn {
			t.Run(c.name+"/NewTxn", func(t *testing.T) {
				s := newFaultStore(t)
				if c.seed != nil {
					c.seed(t, s)
				}
				s.failNewTxn(errFault)
				require.ErrorIs(t, c.run(s), errFault)
			})
		}
		for _, o := range c.ops {
			t.Run(c.name+"/"+o.String(), func(t *testing.T) {
				s := newFaultStore(t)
				if c.seed != nil {
					c.seed(t, s)
				}
				s.arm(o.method, o.nth)
				require.ErrorIs(t, c.run(s), errFault)
			})
		}
	}
}

func TestBlocksRepoErrorPaths(t *testing.T) {
	const blocker, blockee = "blocker-1", "blockee-1"
	seedBlock := func(t *testing.T, s *faultStore) {
		require.NoError(t, NewBlocksRepo(s).Block(blocker, blockee))
	}

	runFaultCases(t, []faultCase{
		{
			name: "Block",
			run:  func(s *faultStore) error { return NewBlocksRepo(s).Block(blocker, blockee) },
			ops:  []faultOp{op("Set"), opN("Set", 2), op("Commit")},
		},
		{
			name: "Unblock",
			seed: seedBlock,
			run:  func(s *faultStore) error { return NewBlocksRepo(s).Unblock(blocker, blockee) },
			ops:  []faultOp{op("Delete"), opN("Delete", 2), op("Commit")},
		},
		{
			name: "IsBlocked",
			seed: seedBlock,
			run: func(s *faultStore) error {
				_, err := NewBlocksRepo(s).IsBlocked(blocker, blockee)
				return err
			},
			ops: []faultOp{op("Get"), op("Commit")},
		},
		{
			name: "List",
			seed: seedBlock,
			run: func(s *faultStore) error {
				_, _, err := NewBlocksRepo(s).List(blocker, nil, nil)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
	})

	t.Run("IsBlockedCommitWhenAbsent", func(t *testing.T) {
		s := newFaultStore(t).arm("Commit", 1)
		_, err := NewBlocksRepo(s).IsBlocked(blocker, "never-blocked")
		require.ErrorIs(t, err, errFault)
	})

	t.Run("EmptyIDs", func(t *testing.T) {
		repo := NewBlocksRepo(newFaultStore(t))
		require.Error(t, repo.Block("", blockee))
		require.Error(t, repo.Block(blocker, ""))
		require.Error(t, repo.Unblock("", blockee))
		require.Error(t, repo.Unblock(blocker, ""))
		_, _, err := repo.List("", nil, nil)
		require.Error(t, err)

		blocked, err := repo.IsBlocked("", blockee)
		require.NoError(t, err)
		require.False(t, blocked)
		blocked, err = repo.IsBlocked(blocker, "")
		require.NoError(t, err)
		require.False(t, blocked)
	})
}

func TestMutesRepoErrorPaths(t *testing.T) {
	const muter, mutee = "muter-1", "mutee-1"
	seedMute := func(t *testing.T, s *faultStore) {
		require.NoError(t, NewMutesRepo(s).Mute(muter, mutee))
	}

	runFaultCases(t, []faultCase{
		{
			name: "Mute",
			run:  func(s *faultStore) error { return NewMutesRepo(s).Mute(muter, mutee) },
			ops:  []faultOp{op("Set"), opN("Set", 2), op("Commit")},
		},
		{
			name: "Unmute",
			seed: seedMute,
			run:  func(s *faultStore) error { return NewMutesRepo(s).Unmute(muter, mutee) },
			ops:  []faultOp{op("Delete"), opN("Delete", 2), op("Commit")},
		},
		{
			name: "IsMuted",
			seed: seedMute,
			run: func(s *faultStore) error {
				_, err := NewMutesRepo(s).IsMuted(muter, mutee)
				return err
			},
			ops: []faultOp{op("Get"), op("Commit")},
		},
		{
			name: "List",
			seed: seedMute,
			run: func(s *faultStore) error {
				_, _, err := NewMutesRepo(s).List(muter, nil, nil)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
	})

	t.Run("EmptyIDs", func(t *testing.T) {
		repo := NewMutesRepo(newFaultStore(t))
		require.Error(t, repo.Mute("", mutee))
		require.Error(t, repo.Mute(muter, ""))
		require.Error(t, repo.Unmute("", mutee))
		require.Error(t, repo.Unmute(muter, ""))
		_, _, err := repo.List("", nil, nil)
		require.Error(t, err)

		muted, err := repo.IsMuted("", mutee)
		require.NoError(t, err)
		require.False(t, muted)
	})
}

func TestSettingsRepoErrorPaths(t *testing.T) {
	const userId = "settings-user"
	seedSettings := func(t *testing.T, s *faultStore) {
		repo := NewSettingsRepo(s)
		require.NoError(t, repo.SetNotificationSettings(userId, domain.NotificationSettings{Recipient: "a@b.c"}))
		require.NoError(t, repo.SetGatewaySettings(userId, domain.GatewaySettings{NodeID: "node-1"}))
	}

	runFaultCases(t, []faultCase{
		{
			name: "SetNotificationSettings",
			run: func(s *faultStore) error {
				return NewSettingsRepo(s).SetNotificationSettings(userId, domain.NotificationSettings{})
			},
			ops: []faultOp{op("Set"), op("Commit")},
		},
		{
			name: "GetNotificationSettings",
			seed: seedSettings,
			run: func(s *faultStore) error {
				_, err := NewSettingsRepo(s).GetNotificationSettings(userId)
				return err
			},
			ops: []faultOp{op("Get"), op("Commit")},
		},
		{
			name: "SetGatewaySettings",
			run: func(s *faultStore) error {
				return NewSettingsRepo(s).SetGatewaySettings(userId, domain.GatewaySettings{})
			},
			ops: []faultOp{op("Set"), op("Commit")},
		},
		{
			name: "GetGatewaySettings",
			seed: seedSettings,
			run: func(s *faultStore) error {
				_, err := NewSettingsRepo(s).GetGatewaySettings(userId)
				return err
			},
			ops: []faultOp{op("Get"), op("Commit")},
		},
	})

	t.Run("EmptyUserId", func(t *testing.T) {
		repo := NewSettingsRepo(newFaultStore(t))
		_, err := repo.GetNotificationSettings("")
		require.Error(t, err)
		require.Error(t, repo.SetNotificationSettings("", domain.NotificationSettings{}))
		_, err = repo.GetGatewaySettings("")
		require.Error(t, err)
		require.Error(t, repo.SetGatewaySettings("", domain.GatewaySettings{}))
	})

	t.Run("CorruptPayload", func(t *testing.T) {
		s := newFaultStore(t)
		require.NoError(t, s.db.Set(settingsKey(userId), []byte("{not-json")))
		require.NoError(t, s.db.Set(gatewaySettingsKey(userId), []byte("{not-json")))

		repo := NewSettingsRepo(s)
		_, err := repo.GetNotificationSettings(userId)
		require.Error(t, err)
		_, err = repo.GetGatewaySettings(userId)
		require.Error(t, err)
	})

	t.Run("MissingReturnsZeroValue", func(t *testing.T) {
		repo := NewSettingsRepo(newFaultStore(t))
		ns, err := repo.GetNotificationSettings("absent-user")
		require.NoError(t, err)
		require.Equal(t, domain.NotificationSettings{}, ns)

		gs, err := repo.GetGatewaySettings("absent-user")
		require.NoError(t, err)
		require.Equal(t, domain.GatewaySettings{}, gs)
	})
}

func TestBookmarkRepoErrorPaths(t *testing.T) {
	const userId, tweetId, ownerId = "bm-user", "bm-tweet", "bm-owner"
	seedBookmark := func(t *testing.T, s *faultStore) {
		require.NoError(t, NewBookmarkRepo(s).Bookmark(userId, tweetId, ownerId))
	}

	runFaultCases(t, []faultCase{
		{
			name: "Bookmark",
			run:  func(s *faultStore) error { return NewBookmarkRepo(s).Bookmark(userId, tweetId, ownerId) },
			ops:  []faultOp{op("Set"), opN("Set", 2), op("Commit")},
		},
		{
			name: "BookmarkAlreadyPresent",
			seed: seedBookmark,
			run:  func(s *faultStore) error { return NewBookmarkRepo(s).Bookmark(userId, tweetId, ownerId) },
			ops:  []faultOp{op("Commit")},
		},
		{
			name: "Unbookmark",
			seed: seedBookmark,
			run:  func(s *faultStore) error { return NewBookmarkRepo(s).Unbookmark(userId, tweetId) },
			ops:  []faultOp{op("Get"), op("Delete"), opN("Delete", 2), op("Commit")},
		},
		{
			name: "List",
			seed: seedBookmark,
			run: func(s *faultStore) error {
				_, _, err := NewBookmarkRepo(s).List(userId, nil, nil)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
	})

	t.Run("UnbookmarkAbsentIsNoop", func(t *testing.T) {
		require.NoError(t, NewBookmarkRepo(newFaultStore(t)).Unbookmark(userId, "never-bookmarked"))
	})

	t.Run("EmptyIDs", func(t *testing.T) {
		repo := NewBookmarkRepo(newFaultStore(t))
		require.Error(t, repo.Bookmark("", tweetId, ownerId))
		require.Error(t, repo.Bookmark(userId, "", ownerId))
		require.Error(t, repo.Bookmark(userId, tweetId, ""))
		require.Error(t, repo.Unbookmark("", tweetId))
		require.Error(t, repo.Unbookmark(userId, ""))
		_, _, err := repo.List("", nil, nil)
		require.Error(t, err)
	})

	t.Run("CorruptPayload", func(t *testing.T) {
		s := newFaultStore(t)
		require.NoError(t, NewBookmarkRepo(s).Bookmark(userId, tweetId, ownerId))

		sortable, err := s.db.Get(local_store.NewPrefixBuilder(BookmarkRepoName).
			AddRootID(userId).
			AddRange(local_store.FixedRangeKey).
			AddParentId(tweetId).
			Build())
		require.NoError(t, err)
		require.NoError(t, s.db.Set(local_store.DatabaseKey(sortable), []byte("{not-json")))

		_, _, err = NewBookmarkRepo(s).List(userId, nil, nil)
		require.Error(t, err)
	})
}

func TestTimelineRepoErrorPaths(t *testing.T) {
	const userId, tweetId = "tl-user", "tl-tweet"
	tweet := domain.Tweet{Id: tweetId, UserId: userId, Text: "hello"}
	seedTimeline := func(t *testing.T, s *faultStore) {
		require.NoError(t, NewTimelineRepo(s).AddTweetToTimeline(userId, tweet))
	}

	runFaultCases(t, []faultCase{
		{
			name: "AddTweetToTimeline",
			run:  func(s *faultStore) error { return NewTimelineRepo(s).AddTweetToTimeline(userId, tweet) },
			ops:  []faultOp{op("Set"), opN("Set", 2), op("Commit")},
		},
		{
			name: "DeleteTweetFromTimeline",
			seed: seedTimeline,
			run:  func(s *faultStore) error { return NewTimelineRepo(s).DeleteTweetFromTimeline(userId, tweetId) },
			ops:  []faultOp{op("Get"), op("Delete"), opN("Delete", 2), op("Commit")},
		},
		{
			name: "GetTimeline",
			seed: seedTimeline,
			run: func(s *faultStore) error {
				_, _, err := NewTimelineRepo(s).GetTimeline(userId, nil, nil)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
	})

	t.Run("DeleteAbsentIsNoop", func(t *testing.T) {
		require.NoError(t, NewTimelineRepo(newFaultStore(t)).DeleteTweetFromTimeline(userId, "absent"))
	})

	t.Run("EmptyIDs", func(t *testing.T) {
		repo := NewTimelineRepo(newFaultStore(t))
		require.Error(t, repo.AddTweetToTimeline("", tweet))
		require.Error(t, repo.AddTweetToTimeline(userId, domain.Tweet{}))
		require.Error(t, repo.DeleteTweetFromTimeline("", tweetId))
		_, _, err := repo.GetTimeline("", nil, nil)
		require.Error(t, err)
	})

	t.Run("CorruptPayload", func(t *testing.T) {
		s := newFaultStore(t)
		require.NoError(t, NewTimelineRepo(s).AddTweetToTimeline(userId, tweet))

		sortable, err := s.db.Get(local_store.NewPrefixBuilder(TimelineRepoName).
			AddRootID(userId).
			AddRange(local_store.FixedRangeKey).
			AddParentId(tweetId).
			Build())
		require.NoError(t, err)
		require.NoError(t, s.db.Set(local_store.DatabaseKey(sortable), []byte("{not-json")))

		_, _, err = NewTimelineRepo(s).GetTimeline(userId, nil, nil)
		require.Error(t, err)
	})
}

func TestSubscriptionsRepoErrorPaths(t *testing.T) {
	const selfId, targetId = "sub-self", "sub-target"
	seedSub := func(t *testing.T, s *faultStore) {
		require.NoError(t, NewSubscriptionsRepo(s).Subscribe(selfId, targetId))
	}

	runFaultCases(t, []faultCase{
		{
			name: "Subscribe",
			run:  func(s *faultStore) error { return NewSubscriptionsRepo(s).Subscribe(selfId, targetId) },
			ops:  []faultOp{op("Set"), op("Commit")},
		},
		{
			name: "Unsubscribe",
			seed: seedSub,
			run:  func(s *faultStore) error { return NewSubscriptionsRepo(s).Unsubscribe(selfId, targetId) },
			ops:  []faultOp{op("Delete"), op("Commit")},
		},
		{
			name: "IsSubscribed",
			seed: seedSub,
			run: func(s *faultStore) error {
				_, err := NewSubscriptionsRepo(s).IsSubscribed(selfId, targetId)
				return err
			},
			ops: []faultOp{op("Get")},
		},
	})
}

func TestAliasesRepoErrorPaths(t *testing.T) {
	alias := domain.Alias{NodeId: "node-1"}

	t.Run("NilRepo", func(t *testing.T) {
		repo := NewAliasesRepo(nil)
		_, err := repo.GetAliases()
		require.ErrorIs(t, err, ErrNilAliasesRepo)
		require.ErrorIs(t, repo.SetAlias(alias), ErrNilAliasesRepo)
		_, err = repo.GetNodeIDs()
		require.ErrorIs(t, err, ErrNilAliasesRepo)
	})

	t.Run("SetAliasStoreFails", func(t *testing.T) {
		s := newFaultStore(t).arm("db.SetWithTTL", 1)
		require.ErrorIs(t, NewAliasesRepo(s).SetAlias(alias), errFault)
	})

	runFaultCases(t, []faultCase{
		{
			name: "GetAliases",
			run: func(s *faultStore) error {
				_, err := NewAliasesRepo(s).GetAliases()
				return err
			},
			ops: []faultOp{op("List")},
		},
		{
			name: "GetNodeIDs",
			run: func(s *faultStore) error {
				_, err := NewAliasesRepo(s).GetNodeIDs()
				return err
			},
			ops: []faultOp{op("List")},
		},
	})

	t.Run("CorruptPayload", func(t *testing.T) {
		s := newFaultStore(t)
		require.NoError(t, s.db.Set(local_store.NewPrefixBuilder(AliasesRepoName).
			AddRootID("None").
			AddRange(local_store.NoneRangeKey).
			AddParentId("broken").
			Build(), []byte("{not-json")))

		_, err := NewAliasesRepo(s).GetAliases()
		require.Error(t, err)
	})

	t.Run("EmptySetIsNotAnError", func(t *testing.T) {
		aliases, err := NewAliasesRepo(newFaultStore(t)).GetAliases()
		require.NoError(t, err)
		require.Empty(t, aliases)
	})
}

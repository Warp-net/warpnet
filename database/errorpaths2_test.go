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
	"testing"
	"time"

	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/require"
)

func TestFilterRepoErrorPaths(t *testing.T) {
	const userId = "filter-user"
	filter := domain.Filter{Id: "filter-1", Title: "spoilers", Keywords: []domain.FilterKeyword{{Id: "kw-1", Keyword: "ending"}}}

	seedFilter := func(t *testing.T, s *faultStore) {
		_, err := NewFilterRepo(s).Create(userId, filter)
		require.NoError(t, err)
	}

	runFaultCases(t, []faultCase{
		{
			name: "Create",
			run: func(s *faultStore) error {
				_, err := NewFilterRepo(s).Create(userId, filter)
				return err
			},
			ops: []faultOp{op("Set"), op("Commit")},
		},
		{
			name: "Get",
			seed: seedFilter,
			run: func(s *faultStore) error {
				_, err := NewFilterRepo(s).Get(userId, filter.Id)
				return err
			},
			ops: []faultOp{op("Get"), op("Commit")},
		},
		{
			name: "Update",
			seed: seedFilter,
			run: func(s *faultStore) error {
				_, err := NewFilterRepo(s).Update(userId, domain.Filter{Id: filter.Id, Title: "renamed"})
				return err
			},
			ops: []faultOp{op("Get"), op("Set"), opN("Commit", 2)},
		},
		{
			name: "Delete",
			seed: seedFilter,
			run:  func(s *faultStore) error { return NewFilterRepo(s).Delete(userId, filter.Id) },
			ops:  []faultOp{op("Delete"), op("Commit")},
		},
		{
			name: "List",
			seed: seedFilter,
			run: func(s *faultStore) error {
				_, _, err := NewFilterRepo(s).List(userId, nil, nil)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
		{
			name: "AddKeyword",
			seed: seedFilter,
			run: func(s *faultStore) error {
				_, err := NewFilterRepo(s).AddKeyword(userId, filter.Id, domain.FilterKeyword{Keyword: "twist"})
				return err
			},
			ops: []faultOp{op("Get"), op("Set"), opN("Commit", 2)},
		},
		{
			name: "UpdateKeyword",
			seed: seedFilter,
			run: func(s *faultStore) error {
				_, err := NewFilterRepo(s).UpdateKeyword(userId, domain.FilterKeyword{Id: "kw-1", Keyword: "finale"})
				return err
			},
			ops: []faultOp{op("List"), op("Set"), opN("Commit", 2)},
		},
		{
			name: "DeleteKeyword",
			seed: seedFilter,
			run:  func(s *faultStore) error { return NewFilterRepo(s).DeleteKeyword(userId, "kw-1") },
			ops:  []faultOp{op("List"), op("Set"), opN("Commit", 2)},
		},
	})

	t.Run("Validation", func(t *testing.T) {
		repo := NewFilterRepo(newFaultStore(t))

		_, err := repo.Create("", filter)
		require.Error(t, err)
		_, err = repo.Create(userId, domain.Filter{})
		require.Error(t, err)

		_, err = repo.Get("", filter.Id)
		require.ErrorIs(t, err, ErrFilterNotFound)
		_, err = repo.Get(userId, "")
		require.ErrorIs(t, err, ErrFilterNotFound)
		_, err = repo.Get(userId, "absent")
		require.ErrorIs(t, err, ErrFilterNotFound)

		_, err = repo.Update(userId, domain.Filter{Id: "absent"})
		require.ErrorIs(t, err, ErrFilterNotFound)

		_, err = repo.AddKeyword(userId, filter.Id, domain.FilterKeyword{})
		require.Error(t, err)
		_, err = repo.UpdateKeyword(userId, domain.FilterKeyword{})
		require.Error(t, err)
		require.Error(t, repo.DeleteKeyword(userId, ""))

		_, _, err = repo.List("", nil, nil)
		require.Error(t, err)
	})

	t.Run("KeywordOnAbsentFilter", func(t *testing.T) {
		repo := NewFilterRepo(newFaultStore(t))
		_, err := repo.UpdateKeyword(userId, domain.FilterKeyword{Id: "nope"})
		require.ErrorIs(t, err, ErrFilterNotFound)
		// deleting a keyword nobody owns is idempotent
		require.NoError(t, repo.DeleteKeyword(userId, "nope"))
	})

	t.Run("CorruptPayload", func(t *testing.T) {
		s := newFaultStore(t)
		seedFilter(t, s)
		require.NoError(t, s.db.Set(filterKey(userId, filter.Id), []byte("{not-json")))

		repo := NewFilterRepo(s)
		_, err := repo.Get(userId, filter.Id)
		require.Error(t, err)
		_, _, err = repo.List(userId, nil, nil)
		require.Error(t, err)
	})
}

func TestPollRepoErrorPaths(t *testing.T) {
	const tweetId, userId = "poll-tweet", "poll-voter"

	t.Run("VoteFaults", func(t *testing.T) {
		for _, o := range []faultOp{op("Set"), op("Increment"), op("Commit")} {
			t.Run(o.String(), func(t *testing.T) {
				s := newFaultStore(t).arm(o.method, o.nth)
				require.ErrorIs(t, NewPollRepo(s, nil).Vote(tweetId, userId, 0, false), errFault)
			})
		}
	})

	t.Run("VoteNewTxn", func(t *testing.T) {
		s := newFaultStore(t).failNewTxn(errFault)
		require.ErrorIs(t, NewPollRepo(s, nil).Vote(tweetId, userId, 0, false), errFault)
	})

	t.Run("VoteTwice", func(t *testing.T) {
		repo := NewPollRepo(newFaultStore(t), nil)
		require.NoError(t, repo.Vote(tweetId, userId, 1, false))
		require.ErrorIs(t, repo.Vote(tweetId, userId, 1, false), ErrPollAlreadyVoted)
	})

	t.Run("Validation", func(t *testing.T) {
		repo := NewPollRepo(newFaultStore(t), nil)
		require.Error(t, repo.Vote("", userId, 0, false))
		require.Error(t, repo.Vote(tweetId, "", 0, false))
		require.Error(t, repo.Vote(tweetId, userId, -1, false))

		_, _, err := repo.Voted("", userId)
		require.Error(t, err)
		_, _, err = repo.Voted(tweetId, "")
		require.Error(t, err)

		_, err = repo.Results("", 2)
		require.Error(t, err)
		_, err = repo.Results(tweetId, 0)
		require.Error(t, err)
	})

	t.Run("VotedReadsBack", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewPollRepo(s, nil)
		require.NoError(t, repo.Vote(tweetId, userId, 2, false))

		option, ok, err := repo.Voted(tweetId, userId)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, 2, option)

		_, ok, err = repo.Voted(tweetId, "other-voter")
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("VotedStoreFails", func(t *testing.T) {
		s := newFaultStore(t).arm("db.Get", 1)
		_, _, err := NewPollRepo(s, nil).Voted(tweetId, userId)
		require.ErrorIs(t, err, errFault)
	})

	t.Run("VotedCorruptOption", func(t *testing.T) {
		s := newFaultStore(t)
		require.NoError(t, s.db.Set(pollVoterKey(tweetId, userId), []byte("not-a-number")))
		_, _, err := NewPollRepo(s, nil).Voted(tweetId, userId)
		require.Error(t, err)
	})

	t.Run("ResultsCountsVotes", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewPollRepo(s, nil)
		require.NoError(t, repo.Vote(tweetId, "voter-a", 0, false))
		require.NoError(t, repo.Vote(tweetId, "voter-b", 0, false))
		require.NoError(t, repo.Vote(tweetId, "voter-c", 1, false))

		votes, err := repo.Results(tweetId, 3)
		require.NoError(t, err)
		require.Equal(t, []uint64{2, 1, 0}, votes)
	})

	t.Run("ResultsStoreFails", func(t *testing.T) {
		s := newFaultStore(t).arm("db.Get", 1)
		_, err := NewPollRepo(s, nil).Results(tweetId, 2)
		require.ErrorIs(t, err, errFault)
	})
}

func TestOutboxRepoErrorPaths(t *testing.T) {
	const nodeId = "dest-node"
	seedMessage := func(t *testing.T, s *faultStore) {
		_, err := NewOutboxRepo(s).Enqueue(nodeId, "/private/post/tweet", []byte(`{"a":1}`))
		require.NoError(t, err)
	}

	runFaultCases(t, []faultCase{
		{
			name: "Enqueue",
			run: func(s *faultStore) error {
				_, err := NewOutboxRepo(s).Enqueue(nodeId, "/route", []byte("{}"))
				return err
			},
			ops: []faultOp{op("SetWithTTL"), op("Commit")},
		},
		{
			name: "ListByNode",
			seed: seedMessage,
			run: func(s *faultStore) error {
				_, err := NewOutboxRepo(s).ListByNode(nodeId)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
		{
			name: "Delete",
			seed: seedMessage,
			run:  func(s *faultStore) error { return NewOutboxRepo(s).Delete(nodeId, "some-id") },
			ops:  []faultOp{op("Delete"), op("Commit")},
		},
		{
			name: "ListNodes",
			seed: seedMessage,
			run: func(s *faultStore) error {
				_, err := NewOutboxRepo(s).ListNodes()
				return err
			},
			ops: []faultOp{op("IterateKeys"), op("Commit")},
		},
	})

	t.Run("RoundTrip", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewOutboxRepo(s)

		msg, err := repo.Enqueue(nodeId, "/private/post/tweet", []byte(`{"a":1}`))
		require.NoError(t, err)
		require.NotEmpty(t, msg.MessageId)

		msgs, err := repo.ListByNode(nodeId)
		require.NoError(t, err)
		require.Len(t, msgs, 1)

		nodes, err := repo.ListNodes()
		require.NoError(t, err)
		require.Equal(t, []string{nodeId}, nodes)

		require.NoError(t, repo.Delete(nodeId, string(msg.MessageId)))
		msgs, err = repo.ListByNode(nodeId)
		require.NoError(t, err)
		require.Empty(t, msgs)
	})
}

func TestNotificationsRepoErrorPaths(t *testing.T) {
	const userId = "notif-user"
	notification := domain.Notification{
		Id:          "notif-1",
		RecepientId: userId,
		Type:        domain.NotificationFollowType,
		Text:        "followed you",
		CreatedAt:   time.Now(),
	}
	seedNotification := func(t *testing.T, s *faultStore) {
		require.NoError(t, NewNotificationsRepo(s).Add(notification))
	}

	runFaultCases(t, []faultCase{
		{
			name: "Add",
			run:  func(s *faultStore) error { return NewNotificationsRepo(s).Add(notification) },
			ops:  []faultOp{op("SetWithTTL"), op("Commit")},
		},
		{
			name: "MarkRead",
			seed: seedNotification,
			run:  func(s *faultStore) error { return NewNotificationsRepo(s).MarkRead(userId, notification.Id) },
			ops:  []faultOp{op("List"), op("Get"), op("SetWithTTL"), op("Commit")},
		},
		{
			name: "MarkAllRead",
			seed: seedNotification,
			run:  func(s *faultStore) error { return NewNotificationsRepo(s).MarkAllRead(userId) },
			ops:  []faultOp{op("List"), op("Get"), op("SetWithTTL"), op("Commit")},
		},
		{
			name: "Get",
			seed: seedNotification,
			run: func(s *faultStore) error {
				_, err := NewNotificationsRepo(s).Get(userId, notification.Id)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
		{
			name: "List",
			seed: seedNotification,
			run: func(s *faultStore) error {
				_, _, err := NewNotificationsRepo(s).List(userId, nil, nil)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
		{
			name: "ReverseList",
			seed: seedNotification,
			run: func(s *faultStore) error {
				_, _, err := NewNotificationsRepo(s).ReverseList(userId, nil, nil)
				return err
			},
			ops: []faultOp{op("List")},
		},
		{
			name: "UnreadCount",
			seed: seedNotification,
			run: func(s *faultStore) error {
				_, err := NewNotificationsRepo(s).UnreadCount(userId)
				return err
			},
			ops: []faultOp{op("List")},
		},
	})

	t.Run("Validation", func(t *testing.T) {
		repo := NewNotificationsRepo(newFaultStore(t))
		require.Error(t, repo.Add(domain.Notification{}))
		require.Error(t, repo.MarkRead("", notification.Id))
		require.Error(t, repo.MarkRead(userId, ""))
		require.Error(t, repo.MarkAllRead(""))

		_, err := repo.Get("", notification.Id)
		require.Error(t, err)
		_, err = repo.Get(userId, "")
		require.Error(t, err)
		_, _, err = repo.List("", nil, nil)
		require.Error(t, err)
		_, _, err = repo.ReverseList("", nil, nil)
		require.Error(t, err)
		_, err = repo.UnreadCount("")
		require.Error(t, err)
	})

	t.Run("AbsentNotification", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewNotificationsRepo(s)
		seedNotification(t, s)

		_, err := repo.Get(userId, "absent")
		require.Error(t, err)
		require.Error(t, repo.MarkRead(userId, "absent"))
	})

	t.Run("MarkReadFlow", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewNotificationsRepo(s)
		seedNotification(t, s)

		count, err := repo.UnreadCount(userId)
		require.NoError(t, err)
		require.Equal(t, uint64(1), count)

		require.NoError(t, repo.MarkRead(userId, notification.Id))
		got, err := repo.Get(userId, notification.Id)
		require.NoError(t, err)
		require.True(t, got.IsRead)

		count, err = repo.UnreadCount(userId)
		require.NoError(t, err)
		require.Zero(t, count)

		// marking an already-read notification again is a no-op, not an error
		require.NoError(t, repo.MarkAllRead(userId))
	})
}

func TestFollowRepoErrorPaths(t *testing.T) {
	const from, to = "follower-user", "followee-user"
	seedFollow := func(t *testing.T, s *faultStore) {
		require.NoError(t, NewFollowRepo(s).Follow(from, to))
	}

	runFaultCases(t, []faultCase{
		{
			name: "Follow",
			run:  func(s *faultStore) error { return NewFollowRepo(s).Follow(from, to) },
			ops: []faultOp{
				op("Set"), opN("Set", 2), opN("Set", 3), opN("Set", 4),
				op("Increment"), opN("Increment", 2), op("Commit"),
			},
		},
		{
			name: "GetFollowersCount",
			seed: seedFollow,
			run: func(s *faultStore) error {
				_, err := NewFollowRepo(s).GetFollowersCount(to)
				return err
			},
			ops: []faultOp{op("Get"), op("Commit")},
		},
		{
			name: "GetFollowingsCount",
			seed: seedFollow,
			run: func(s *faultStore) error {
				_, err := NewFollowRepo(s).GetFollowingsCount(from)
				return err
			},
			ops: []faultOp{op("Get"), op("Commit")},
		},
		{
			name: "GetFollowers",
			seed: seedFollow,
			run: func(s *faultStore) error {
				_, _, err := NewFollowRepo(s).GetFollowers(to, nil, nil)
				return err
			},
			ops: []faultOp{op("ListKeys"), op("Commit")},
		},
		{
			name: "GetFollowings",
			seed: seedFollow,
			run: func(s *faultStore) error {
				_, _, err := NewFollowRepo(s).GetFollowings(from, nil, nil)
				return err
			},
			ops: []faultOp{op("ListKeys"), op("Commit")},
		},
		{
			name: "AddFollowRequest",
			run:  func(s *faultStore) error { return NewFollowRepo(s).AddFollowRequest(to, from) },
			ops:  []faultOp{op("Set"), op("Commit")},
		},
		{
			name: "RemoveFollowRequest",
			run:  func(s *faultStore) error { return NewFollowRepo(s).RemoveFollowRequest(to, from) },
			ops:  []faultOp{op("Delete"), op("Commit")},
		},
		{
			name: "HasFollowRequest",
			run: func(s *faultStore) error {
				_, err := NewFollowRepo(s).HasFollowRequest(to, from)
				return err
			},
			ops: []faultOp{op("Commit")},
		},
		{
			name: "ListFollowRequests",
			run: func(s *faultStore) error {
				_, _, err := NewFollowRepo(s).ListFollowRequests(to, nil, nil)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
	})

	t.Run("UnfollowFaults", func(t *testing.T) {
		for _, o := range []faultOp{
			op("Delete"), opN("Delete", 2), opN("Delete", 3), opN("Delete", 4),
			op("Decrement"), opN("Decrement", 2), op("Commit"),
		} {
			t.Run(o.String(), func(t *testing.T) {
				s := newFaultStore(t)
				seedFollow(t, s)
				s.arm(o.method, o.nth)
				require.ErrorIs(t, NewFollowRepo(s).Unfollow(from, to), errFault)
			})
		}
	})

	t.Run("Validation", func(t *testing.T) {
		repo := NewFollowRepo(newFaultStore(t))
		require.Error(t, repo.Follow("", to))
		require.Error(t, repo.Follow(from, ""))
		require.Error(t, repo.Follow(from, from))
		require.Error(t, repo.Unfollow("", to))
		require.Error(t, repo.Unfollow(from, ""))

		_, err := repo.GetFollowersCount("")
		require.Error(t, err)
		_, err = repo.GetFollowingsCount("")
		require.Error(t, err)

		require.Error(t, repo.AddFollowRequest("", from))
		require.Error(t, repo.AddFollowRequest(to, ""))
		require.Error(t, repo.RemoveFollowRequest("", from))
		require.Error(t, repo.RemoveFollowRequest(to, ""))
		_, _, err = repo.ListFollowRequests("", nil, nil)
		require.Error(t, err)

		// an empty id pair is "no request", not an error
		has, err := repo.HasFollowRequest("", from)
		require.NoError(t, err)
		require.False(t, has)
		has, err = repo.HasFollowRequest(to, "")
		require.NoError(t, err)
		require.False(t, has)
	})

	t.Run("FollowRequestRoundTrip", func(t *testing.T) {
		repo := NewFollowRepo(newFaultStore(t))

		has, err := repo.HasFollowRequest(to, from)
		require.NoError(t, err)
		require.False(t, has)

		require.NoError(t, repo.AddFollowRequest(to, from))
		has, err = repo.HasFollowRequest(to, from)
		require.NoError(t, err)
		require.True(t, has)

		ids, _, err := repo.ListFollowRequests(to, nil, nil)
		require.NoError(t, err)
		require.Equal(t, []string{from}, ids)

		require.NoError(t, repo.RemoveFollowRequest(to, from))
		has, err = repo.HasFollowRequest(to, from)
		require.NoError(t, err)
		require.False(t, has)
		// removing twice is idempotent
		require.NoError(t, repo.RemoveFollowRequest(to, from))
	})

	t.Run("IsFollowingStoreFails", func(t *testing.T) {
		s := newFaultStore(t)
		seedFollow(t, s)

		repo := NewFollowRepo(s)
		require.True(t, repo.IsFollowing(from, to))
		require.True(t, repo.IsFollower(to, from))
		require.False(t, repo.IsFollowing(from, "stranger"))
		require.False(t, repo.IsFollower(to, "stranger"))

		s.arm("db.Get", 1)
		require.False(t, repo.IsFollowing(from, to))
	})

	t.Run("HasFollowRequestGetFails", func(t *testing.T) {
		s := newFaultStore(t)
		require.NoError(t, NewFollowRepo(s).AddFollowRequest(to, from))
		s.arm("Get", 1)
		_, err := NewFollowRepo(s).HasFollowRequest(to, from)
		require.ErrorIs(t, err, errFault)
	})

	t.Run("UnfollowLookupFails", func(t *testing.T) {
		s := newFaultStore(t)
		seedFollow(t, s)
		s.arm("db.Get", 1)
		require.ErrorIs(t, NewFollowRepo(s).Unfollow(from, to), errFault)
	})
}

func TestReactionRepoErrorPaths(t *testing.T) {
	const tweetId, userId, ownerId, emoji = "react-tweet", "react-user", "react-owner", "👍"
	seedReaction := func(t *testing.T, s *faultStore) {
		_, err := NewReactionRepo(s, nil).React(tweetId, userId, emoji, false)
		require.NoError(t, err)
	}

	t.Run("ReactFaults", func(t *testing.T) {
		for _, o := range []faultOp{op("Set"), op("Increment"), opN("Increment", 2), op("Commit")} {
			t.Run(o.String(), func(t *testing.T) {
				s := newFaultStore(t).arm(o.method, o.nth)
				_, err := NewReactionRepo(s, nil).React(tweetId, userId, emoji, false)
				require.ErrorIs(t, err, errFault)
			})
		}
	})

	t.Run("SwitchReactionFaults", func(t *testing.T) {
		for _, o := range []faultOp{op("Set"), op("Decrement"), op("Increment"), op("Commit")} {
			t.Run(o.String(), func(t *testing.T) {
				s := newFaultStore(t)
				seedReaction(t, s)
				s.arm(o.method, o.nth)
				_, err := NewReactionRepo(s, nil).React(tweetId, userId, "🎉", false)
				require.ErrorIs(t, err, errFault)
			})
		}
	})

	t.Run("SwitchToSameEmojiIsNoop", func(t *testing.T) {
		s := newFaultStore(t)
		seedReaction(t, s)
		count, err := NewReactionRepo(s, nil).React(tweetId, userId, emoji, false)
		require.NoError(t, err)
		require.Equal(t, uint64(1), count)
	})

	t.Run("UnreactFaults", func(t *testing.T) {
		for _, o := range []faultOp{op("Delete"), op("Decrement"), opN("Decrement", 2), op("Commit")} {
			t.Run(o.String(), func(t *testing.T) {
				s := newFaultStore(t)
				seedReaction(t, s)
				s.arm(o.method, o.nth)
				_, err := NewReactionRepo(s, nil).Unreact(tweetId, userId, false)
				require.ErrorIs(t, err, errFault)
			})
		}
	})

	t.Run("UnreactWithoutReaction", func(t *testing.T) {
		s := newFaultStore(t)
		// nothing reacted at all: the tweet has no counter to report
		_, err := NewReactionRepo(s, nil).Unreact(tweetId, "never-reacted", false)
		require.ErrorIs(t, err, ErrReactionsNotFound)

		// someone else reacted: unreacting a non-reactor leaves their count alone
		seedReaction(t, s)
		count, err := NewReactionRepo(s, nil).Unreact(tweetId, "never-reacted", false)
		require.NoError(t, err)
		require.Equal(t, uint64(1), count)
	})

	runFaultCases(t, []faultCase{
		{
			name: "Reactions",
			seed: seedReaction,
			run: func(s *faultStore) error {
				_, err := NewReactionRepo(s, nil).Reactions(tweetId)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
		{
			name: "Reactors",
			seed: seedReaction,
			run: func(s *faultStore) error {
				_, _, err := NewReactionRepo(s, nil).Reactors(tweetId, nil, nil)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
		{
			name: "SetReacted",
			run:  func(s *faultStore) error { return NewReactionRepo(s, nil).SetReacted(userId, tweetId, ownerId) },
			ops:  []faultOp{op("Set"), opN("Set", 2), op("Commit")},
		},
		{
			name: "Reacted",
			run: func(s *faultStore) error {
				_, _, err := NewReactionRepo(s, nil).Reacted(userId, nil, nil)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
	})

	t.Run("RemoveReactedFaults", func(t *testing.T) {
		for _, o := range []faultOp{op("Get"), op("Delete"), opN("Delete", 2), op("Commit")} {
			t.Run(o.String(), func(t *testing.T) {
				s := newFaultStore(t)
				require.NoError(t, NewReactionRepo(s, nil).SetReacted(userId, tweetId, ownerId))
				s.arm(o.method, o.nth)
				require.ErrorIs(t, NewReactionRepo(s, nil).RemoveReacted(userId, tweetId), errFault)
			})
		}
	})

	t.Run("ReactionAndCountStoreFails", func(t *testing.T) {
		s := newFaultStore(t)
		seedReaction(t, s)
		repo := NewReactionRepo(s, nil)

		got, err := repo.Reaction(tweetId, userId)
		require.NoError(t, err)
		require.Equal(t, emoji, got)

		s.arm("db.Get", 1)
		_, err = repo.Reaction(tweetId, userId)
		require.ErrorIs(t, err, errFault)

		s.arm("db.Get", 1)
		_, err = repo.ReactionsCount(tweetId)
		require.ErrorIs(t, err, errFault)
	})

	t.Run("Validation", func(t *testing.T) {
		repo := NewReactionRepo(newFaultStore(t), nil)

		_, err := repo.React("", userId, emoji, false)
		require.Error(t, err)
		_, err = repo.React(tweetId, "", emoji, false)
		require.Error(t, err)

		_, err = repo.Unreact("", userId, false)
		require.Error(t, err)
		_, err = repo.Unreact(tweetId, "", false)
		require.Error(t, err)

		_, err = repo.Reactions("")
		require.Error(t, err)
		_, err = repo.Reaction("", userId)
		require.Error(t, err)
		_, err = repo.Reaction(tweetId, "")
		require.Error(t, err)
		_, err = repo.ReactionsCount("")
		require.Error(t, err)
		_, _, err = repo.Reactors("", nil, nil)
		require.Error(t, err)

		require.Error(t, repo.SetReacted("", tweetId, ownerId))
		require.Error(t, repo.SetReacted(userId, "", ownerId))
		require.Error(t, repo.SetReacted(userId, tweetId, ""))
		require.Error(t, repo.RemoveReacted("", tweetId))
		require.Error(t, repo.RemoveReacted(userId, ""))
		_, _, err = repo.Reacted("", nil, nil)
		require.Error(t, err)
	})

	t.Run("CorruptReactedPayload", func(t *testing.T) {
		s := newFaultStore(t)
		require.NoError(t, NewReactionRepo(s, nil).SetReacted(userId, tweetId, ownerId))

		sortable, err := s.db.Get(local_store.NewPrefixBuilder(ReactionRepoName).
			AddSubPrefix(ReactedSubNamespace).
			AddRootID(userId).
			AddRange(local_store.FixedRangeKey).
			AddParentId(tweetId).
			Build())
		require.NoError(t, err)
		require.NoError(t, s.db.Set(local_store.DatabaseKey(sortable), []byte("{not-json")))

		_, _, err = NewReactionRepo(s, nil).Reacted(userId, nil, nil)
		require.Error(t, err)
	})
}

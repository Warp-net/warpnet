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
	"math"
	"testing"

	local "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	ds "github.com/ipfs/go-datastore"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

// brokenStats fails every call, exercising the "CRDT store unavailable, fall
// back to the local counter" branches.
type brokenStats struct{}

func (brokenStats) GetAggregatedStat(ds.Key) (uint64, error) { return 0, errFault }
func (brokenStats) Increment(ds.Key) error                   { return errFault }
func (brokenStats) Decrement(ds.Key) error                   { return errFault }

func TestTweetRepoErrorPaths(t *testing.T) {
	userId := ulid.Make().String()
	tweetId := ulid.Make().String()
	tweet := func() domain.Tweet {
		return domain.Tweet{Id: tweetId, UserId: userId, Text: "hello"}
	}
	seedTweet := func(t *testing.T, s *faultStore) {
		_, err := NewTweetRepo(s, nil).Create(userId, tweet())
		require.NoError(t, err)
	}

	parentId := ulid.Make().String()
	replyId := ulid.Make().String()
	reply := func() domain.Tweet {
		p := parentId
		return domain.Tweet{Id: replyId, UserId: userId, Text: "re", ParentId: &p}
	}
	seedReply := func(t *testing.T, s *faultStore) {
		_, err := NewTweetRepo(s, nil).AddReply(reply(), false)
		require.NoError(t, err)
	}

	retweeterId := ulid.Make().String()
	retweet := func() domain.Tweet {
		by := retweeterId
		return domain.Tweet{Id: tweetId, UserId: userId, Text: "rt", RetweetedBy: &by}
	}
	seedRetweet := func(t *testing.T, s *faultStore) {
		_, err := NewTweetRepo(s, nil).NewRetweet(retweet(), false)
		require.NoError(t, err)
	}

	runFaultCases(t, []faultCase{
		{
			name: "CreateWithTTL",
			run: func(s *faultStore) error {
				_, err := NewTweetRepo(s, nil).Create(userId, tweet())
				return err
			},
			ops: []faultOp{op("SetWithTTL"), opN("SetWithTTL", 2), op("Increment"), op("Commit")},
		},
		{
			name: "Update",
			seed: seedTweet,
			run: func(s *faultStore) error {
				return NewTweetRepo(s, nil).Update(domain.Tweet{Id: tweetId, UserId: userId, Text: "edited"})
			},
			ops: []faultOp{op("Get"), opN("Get", 2), op("SetWithTTL"), opN("SetWithTTL", 2), op("Commit")},
		},
		{
			name: "AppendEdit",
			run: func(s *faultStore) error {
				_, err := NewTweetRepo(s, nil).AppendEdit(domain.TweetEdit{
					OriginalTweetId: tweetId, UserId: userId, Text: "edited",
				})
				return err
			},
			ops: []faultOp{op("Set"), op("Commit")},
		},
		{
			name: "Pin",
			seed: seedTweet,
			run: func(s *faultStore) error {
				_, err := NewTweetRepo(s, nil).Pin(userId, tweetId)
				return err
			},
			ops: []faultOp{op("Get"), opN("Get", 2), op("SetWithTTL"), op("Commit")},
		},
		{
			name: "TweetsCount",
			seed: seedTweet,
			run: func(s *faultStore) error {
				_, err := NewTweetRepo(s, nil).TweetsCount(userId)
				return err
			},
			ops: []faultOp{op("Get"), op("Commit")},
		},
		{
			name: "Delete",
			seed: seedTweet,
			run:  func(s *faultStore) error { return NewTweetRepo(s, nil).Delete(userId, tweetId) },
			ops:  []faultOp{op("Get"), op("Delete"), opN("Delete", 2), op("Decrement"), op("Commit")},
		},
		{
			name: "List",
			seed: seedTweet,
			run: func(s *faultStore) error {
				_, _, err := NewTweetRepo(s, nil).List(userId, nil, nil)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
		{
			name: "AddReply",
			run: func(s *faultStore) error {
				_, err := NewTweetRepo(s, nil).AddReply(reply(), false)
				return err
			},
			ops: []faultOp{op("SetWithTTL"), op("Increment"), op("Commit")},
		},
		{
			name: "DeleteReply",
			seed: seedReply,
			run: func(s *faultStore) error {
				_, err := NewTweetRepo(s, nil).DeleteReply(parentId, replyId, false)
				return err
			},
			ops: []faultOp{op("Get"), op("Delete"), op("Decrement"), op("Commit")},
		},
		{
			name: "NewRetweet",
			run: func(s *faultStore) error {
				_, err := NewTweetRepo(s, nil).NewRetweet(retweet(), false)
				return err
			},
			ops: []faultOp{op("Set"), op("SetWithTTL"), op("Increment"), op("Commit")},
		},
		{
			name: "UnRetweet",
			seed: seedRetweet,
			run: func(s *faultStore) error {
				return NewTweetRepo(s, nil).UnRetweet(retweeterId, tweetId, false)
			},
			ops: []faultOp{op("Delete"), op("Get"), op("Decrement"), op("Commit")},
		},
		{
			name: "Retweeters",
			seed: seedRetweet,
			run: func(s *faultStore) error {
				_, _, err := NewTweetRepo(s, nil).Retweeters(tweetId, nil, nil)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
		{
			name: "RecordView",
			run: func(s *faultStore) error {
				_, err := NewTweetRepo(s, nil).RecordView(tweetId, "viewer-1")
				return err
			},
			ops: []faultOp{op("Get"), op("Set"), op("Increment"), op("Commit")},
		},
	})

	t.Run("GetStoreFails", func(t *testing.T) {
		s := newFaultStore(t)
		seedTweet(t, s)
		repo := NewTweetRepo(s, nil)

		s.arm("db.Get", 1)
		_, err := repo.Get(userId, tweetId)
		require.ErrorIs(t, err, errFault)

		s.arm("db.Get", 2)
		_, err = repo.Get(userId, tweetId)
		require.ErrorIs(t, err, errFault)
	})

	t.Run("GetNotFound", func(t *testing.T) {
		repo := NewTweetRepo(newFaultStore(t), nil)
		_, err := repo.Get(userId, "absent")
		require.ErrorIs(t, err, ErrTweetNotFound)
	})

	t.Run("GetCorruptPayload", func(t *testing.T) {
		s := newFaultStore(t)
		seedTweet(t, s)

		sortable, err := s.db.Get(local.NewPrefixBuilder(TweetsNamespace).
			AddRootID(userId).
			AddRange(local.FixedRangeKey).
			AddParentId(tweetId).
			Build())
		require.NoError(t, err)
		require.NoError(t, s.db.Set(local.DatabaseKey(sortable), []byte("{not-json")))

		repo := NewTweetRepo(s, nil)
		_, err = repo.Get(userId, tweetId)
		require.Error(t, err)
		_, _, err = repo.List(userId, nil, nil)
		require.Error(t, err)
	})

	t.Run("Validation", func(t *testing.T) {
		repo := NewTweetRepo(newFaultStore(t), nil)

		_, err := repo.Create(userId, domain.Tweet{})
		require.Error(t, err)
		require.Error(t, repo.Update(domain.Tweet{UserId: userId}))
		require.Error(t, repo.Update(domain.Tweet{Id: tweetId}))

		_, err = repo.AppendEdit(domain.TweetEdit{UserId: userId, Text: "t"})
		require.Error(t, err)
		_, err = repo.AppendEdit(domain.TweetEdit{OriginalTweetId: tweetId, Text: "t"})
		require.Error(t, err)
		_, err = repo.AppendEdit(domain.TweetEdit{OriginalTweetId: tweetId, UserId: userId})
		require.Error(t, err)

		_, err = repo.Pin("", tweetId)
		require.Error(t, err)
		_, err = repo.Pin(userId, "")
		require.Error(t, err)

		_, err = repo.Get("", tweetId)
		require.Error(t, err)
		_, err = repo.Get(userId, "")
		require.Error(t, err)

		_, err = repo.TweetsCount("")
		require.Error(t, err)
		_, _, err = repo.List("", nil, nil)
		require.Error(t, err)

		_, err = repo.AddReply(domain.Tweet{}, false)
		require.Error(t, err)
		_, err = repo.AddReply(domain.Tweet{Text: "no parent"}, false)
		require.Error(t, err)

		_, err = repo.GetReply("", replyId)
		require.Error(t, err)
		_, err = repo.GetReply(parentId, "")
		require.Error(t, err)
		_, err = repo.RepliesCount("")
		require.Error(t, err)
		_, err = repo.DeleteReply("", replyId, false)
		require.Error(t, err)
		_, err = repo.DeleteReply(parentId, "", false)
		require.Error(t, err)
		_, _, err = repo.GetReplies("", nil, nil)
		require.Error(t, err)

		_, err = repo.NewRetweet(domain.Tweet{Id: tweetId}, false)
		require.Error(t, err)
		require.Error(t, repo.UnRetweet("", tweetId, false))
		require.Error(t, repo.UnRetweet(retweeterId, "", false))
		_, err = repo.RetweetsCount("")
		require.Error(t, err)
		_, _, err = repo.Retweeters("", nil, nil)
		require.Error(t, err)

		_, err = repo.RecordView("", "viewer")
		require.Error(t, err)
		_, err = repo.RecordView(tweetId, "")
		require.Error(t, err)
		_, err = repo.GetViewsCount("")
		require.Error(t, err)
	})

	t.Run("Blocklist", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewTweetRepo(s, nil)

		require.NoError(t, repo.Blocklist(""))
		require.False(t, repo.IsBlocklisted(""))
		require.False(t, repo.IsBlocklisted(tweetId))

		require.NoError(t, repo.Blocklist(tweetId))
		require.True(t, repo.IsBlocklisted(tweetId))

		s.arm("db.Set", 1)
		require.ErrorIs(t, repo.Blocklist("other"), errFault)
	})

	t.Run("PinIsIdempotent", func(t *testing.T) {
		s := newFaultStore(t)
		seedTweet(t, s)
		repo := NewTweetRepo(s, nil)

		pinned, err := repo.Pin(userId, tweetId)
		require.NoError(t, err)
		require.True(t, pinned.Pinned)

		again, err := repo.Pin(userId, tweetId)
		require.NoError(t, err)
		require.True(t, again.Pinned)

		unpinned, err := repo.Unpin(userId, tweetId)
		require.NoError(t, err)
		require.False(t, unpinned.Pinned)
	})

	t.Run("PinAlreadyPinnedCommitFails", func(t *testing.T) {
		s := newFaultStore(t)
		seedTweet(t, s)
		_, err := NewTweetRepo(s, nil).Pin(userId, tweetId)
		require.NoError(t, err)

		s.arm("Commit", 1)
		_, err = NewTweetRepo(s, nil).Pin(userId, tweetId)
		require.ErrorIs(t, err, errFault)
	})

	t.Run("UpdateAbsentTweet", func(t *testing.T) {
		repo := NewTweetRepo(newFaultStore(t), nil)
		require.ErrorIs(t, repo.Update(domain.Tweet{Id: "absent", UserId: userId, Text: "x"}), ErrTweetNotFound)
	})

	t.Run("UpdateExpiredTweetClampsTTL", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewTweetRepo(s, nil)

		// A tweet stored with the maximum TTL has an expiration far in the
		// future; one with no TTL record clamps to zero on update.
		_, err := repo.CreateWithTTL(userId, tweet(), math.MaxInt64)
		require.NoError(t, err)
		require.NoError(t, repo.Update(domain.Tweet{Id: tweetId, UserId: userId, Text: "edited"}))

		got, err := repo.Get(userId, tweetId)
		require.NoError(t, err)
		require.Equal(t, "edited", got.Text)
		require.NotNil(t, got.UpdatedAt)
	})

	t.Run("UpdateModeration", func(t *testing.T) {
		s := newFaultStore(t)
		seedTweet(t, s)
		repo := NewTweetRepo(s, nil)

		verdict := domain.TweetModeration{IsOk: domain.OK}
		require.NoError(t, repo.Update(domain.Tweet{Id: tweetId, UserId: userId, Moderation: &verdict}))

		got, err := repo.Get(userId, tweetId)
		require.NoError(t, err)
		require.NotNil(t, got.Moderation)
	})

	t.Run("DeleteAbsentTweet", func(t *testing.T) {
		repo := NewTweetRepo(newFaultStore(t), nil)
		require.Error(t, repo.Delete(userId, "absent"))
	})

	t.Run("RepliesCountFallsBackToLocal", func(t *testing.T) {
		s := newFaultStore(t)
		seedReply(t, s)

		count, err := NewTweetRepo(s, brokenStats{}).RepliesCount(parentId)
		require.NoError(t, err)
		require.Equal(t, uint64(1), count)
	})

	t.Run("RetweetsCount", func(t *testing.T) {
		s := newFaultStore(t)
		seedRetweet(t, s)

		repo := NewTweetRepo(s, nil)
		count, err := repo.RetweetsCount(tweetId)
		require.NoError(t, err)
		require.Equal(t, uint64(1), count)

		_, err = repo.RetweetsCount("never-retweeted")
		require.ErrorIs(t, err, ErrTweetNotFound)

		// with a broken CRDT store the local counter still answers
		count, err = NewTweetRepo(s, brokenStats{}).RetweetsCount(tweetId)
		require.NoError(t, err)
		require.Equal(t, uint64(1), count)

		s.arm("db.Get", 1)
		_, err = repo.RetweetsCount(tweetId)
		require.ErrorIs(t, err, errFault)
	})

	t.Run("QuoteRetweetGetsFreshId", func(t *testing.T) {
		s := newFaultStore(t)
		quoted := tweetId
		by := retweeterId
		stored, err := NewTweetRepo(s, nil).NewRetweet(domain.Tweet{
			Id: tweetId, UserId: userId, Text: "look at this", RetweetedBy: &by, QuotedTweetId: &quoted,
		}, false)
		require.NoError(t, err)
		require.NotEqual(t, tweetId, stored.Id)
	})

	t.Run("SelfRetweetGetsPrefix", func(t *testing.T) {
		s := newFaultStore(t)
		by := userId
		stored, err := NewTweetRepo(s, nil).NewRetweet(domain.Tweet{
			Id: tweetId, UserId: userId, Text: "rt", RetweetedBy: &by,
		}, false)
		require.NoError(t, err)
		require.Equal(t, domain.RetweetPrefix+tweetId, stored.Id)
	})

	t.Run("UnRetweetWithoutCounter", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewTweetRepo(s, nil)
		by := retweeterId
		_, err := repo.NewRetweet(domain.Tweet{Id: tweetId, UserId: userId, Text: "rt", RetweetedBy: &by}, false)
		require.NoError(t, err)

		// drop the counter so UnRetweet takes the "nothing to decrement" path
		require.NoError(t, s.db.Delete(local.NewPrefixBuilder(TweetsNamespace).
			AddSubPrefix(reTweetsCountSubspace).
			AddRootID(tweetId).
			Build()))

		require.NoError(t, repo.UnRetweet(retweeterId, tweetId, false))
	})

	t.Run("RecordViewIsOncePerViewer", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewTweetRepo(s, nil)

		first, err := repo.RecordView(tweetId, "viewer-1")
		require.NoError(t, err)
		require.Equal(t, uint64(1), first)

		second, err := repo.RecordView(tweetId, "viewer-1")
		require.NoError(t, err)
		require.Equal(t, uint64(1), second)

		third, err := repo.RecordView(tweetId, "viewer-2")
		require.NoError(t, err)
		require.Equal(t, uint64(2), third)
	})

	t.Run("RecordViewRepeatCommitFails", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewTweetRepo(s, nil)
		_, err := repo.RecordView(tweetId, "viewer-1")
		require.NoError(t, err)

		s.arm("Commit", 1)
		_, err = repo.RecordView(tweetId, "viewer-1")
		require.ErrorIs(t, err, errFault)
	})

	t.Run("RecordViewWithBrokenStats", func(t *testing.T) {
		s := newFaultStore(t)
		count, err := NewTweetRepo(s, brokenStats{}).RecordView(tweetId, "viewer-1")
		require.NoError(t, err)
		require.Equal(t, uint64(1), count)
	})

	t.Run("GetViewsCount", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewTweetRepo(s, nil)

		_, err := repo.GetViewsCount(tweetId)
		require.ErrorIs(t, err, ErrViewsNotFound)

		_, err = repo.RecordView(tweetId, "viewer-1")
		require.NoError(t, err)

		count, err := repo.GetViewsCount(tweetId)
		require.NoError(t, err)
		require.Equal(t, uint64(1), count)

		s.arm("db.Get", 1)
		_, err = repo.GetViewsCount(tweetId)
		require.ErrorIs(t, err, errFault)
	})

	t.Run("StatsFailuresAreTolerated", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewTweetRepo(s, brokenStats{})

		// each of these logs the stats error and still succeeds locally
		_, err := repo.AddReply(reply(), true)
		require.NoError(t, err)
		_, err = repo.DeleteReply(parentId, replyId, true)
		require.NoError(t, err)

		by := retweeterId
		_, err = repo.NewRetweet(domain.Tweet{Id: tweetId, UserId: userId, Text: "rt", RetweetedBy: &by}, true)
		require.NoError(t, err)
		require.NoError(t, repo.UnRetweet(retweeterId, tweetId, true))
	})
}

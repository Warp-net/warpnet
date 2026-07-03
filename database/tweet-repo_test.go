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
	"strconv"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/suite"
)

type TweetRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *TweetRepo
}

func (s *TweetRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	auth := NewAuthRepo(s.db, "test")
	s.Require().NoError(auth.Authenticate("test", "test"))

	s.repo = NewTweetRepo(s.db, nil)
}

func (s *TweetRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *TweetRepoTestSuite) TestCreateAndGetTweet() {
	userId := ulid.Make().String()
	tweet := domain.Tweet{UserId: userId, Text: "hello world"}

	created, err := s.repo.Create(userId, tweet)
	s.Require().NoError(err)
	s.Equal(tweet.Text, created.Text)

	fetched, err := s.repo.Get(userId, created.Id)
	s.Require().NoError(err)
	s.Equal(created.Id, fetched.Id)
	s.Equal(created.Text, fetched.Text)
}

// TestCreateTweetAndReply covers the unified model end to end: a top-level
// tweet lands in the author's timeline keyspace, while a reply to it is a
// tweet with a parent that lives in the thread index — retrievable via the
// thread APIs, counted against its parent, and kept out of every timeline.
func (s *TweetRepoTestSuite) TestCreateTweetAndReply() {
	author := ulid.Make().String()

	tweet, err := s.repo.Create(author, domain.Tweet{UserId: author, Text: "root tweet"})
	s.Require().NoError(err)
	s.Equal(tweet.Id, tweet.RootId, "top-level tweet is its own root")
	s.False(tweet.IsReply())

	limit := uint64(10)
	list, _, err := s.repo.List(author, &limit, nil)
	s.Require().NoError(err)
	s.Len(list, 1)
	s.Equal(tweet.Id, list[0].Id)

	// A reply is a domain.Tweet with a parent, stored in the thread index.
	replier := ulid.Make().String()
	reply, err := s.repo.AddReply(domain.Tweet{
		UserId:   replier,
		Text:     "a reply",
		RootId:   tweet.Id,
		ParentId: &tweet.Id,
	})
	s.Require().NoError(err)
	s.True(reply.IsReply())
	s.NotEmpty(reply.Id)

	// The reply must not pollute either party's timeline keyspace.
	authorList, _, err := s.repo.List(author, &limit, nil)
	s.Require().NoError(err)
	s.Len(authorList, 1, "reply must stay out of the parent author's timeline")
	replierList, _, err := s.repo.List(replier, &limit, nil)
	s.Require().NoError(err)
	s.Len(replierList, 0, "reply must stay out of the replier's timeline")

	// The reply is retrievable from the thread by id and via the tree.
	gotReply, err := s.repo.GetReply(tweet.Id, reply.Id)
	s.Require().NoError(err)
	s.Equal("a reply", gotReply.Text)

	thread, _, err := s.repo.GetReplies(tweet.Id, &limit, nil)
	s.Require().NoError(err)
	s.Require().Len(thread, 1)
	s.Equal(reply.Id, thread[0].Id)

	// The parent's reply counter incremented.
	count, err := s.repo.RepliesCount(tweet.Id)
	s.Require().NoError(err)
	s.Equal(uint64(1), count)

	// Deleting the reply removes it from the thread and decrements the count.
	_, err = s.repo.DeleteReply(tweet.Id, reply.Id)
	s.Require().NoError(err)
	_, err = s.repo.GetReply(tweet.Id, reply.Id)
	s.Error(err)
	count, err = s.repo.RepliesCount(tweet.Id)
	s.Require().NoError(err)
	s.Equal(uint64(0), count)
}

// TestThreadNesting proves replies nest by partitioning on the parent: a
// reply-to-a-reply is stored under its immediate parent, each level is a
// separate scan, the thread RootId is preserved through the chain, and none
// of the replies leak into any author's timeline.
func (s *TweetRepoTestSuite) TestThreadNesting() {
	author := ulid.Make().String()
	replier := ulid.Make().String()

	root, err := s.repo.Create(author, domain.Tweet{UserId: author, Text: "root"})
	s.Require().NoError(err)

	r1, err := s.repo.AddReply(domain.Tweet{
		UserId: replier, Text: "lvl1", RootId: root.Id, ParentId: &root.Id,
	})
	s.Require().NoError(err)

	r2, err := s.repo.AddReply(domain.Tweet{
		UserId: replier, Text: "lvl2", RootId: root.Id, ParentId: &r1.Id,
	})
	s.Require().NoError(err)

	// RootId is preserved through the chain (not rewritten to the reply id).
	s.Equal(root.Id, r1.RootId)
	s.Equal(root.Id, r2.RootId)

	limit := uint64(10)

	// Each level is the direct-replies scan of its parent.
	lvl1, _, err := s.repo.GetReplies(root.Id, &limit, nil)
	s.Require().NoError(err)
	s.Require().Len(lvl1, 1)
	s.Equal(r1.Id, lvl1[0].Id)

	lvl2, _, err := s.repo.GetReplies(r1.Id, &limit, nil)
	s.Require().NoError(err)
	s.Require().Len(lvl2, 1)
	s.Equal(r2.Id, lvl2[0].Id)

	leaf, _, err := s.repo.GetReplies(r2.Id, &limit, nil)
	s.Require().NoError(err)
	s.Len(leaf, 0)

	// Point lookups resolve under the right parent.
	got1, err := s.repo.GetReply(root.Id, r1.Id)
	s.Require().NoError(err)
	s.Equal("lvl1", got1.Text)
	got2, err := s.repo.GetReply(r1.Id, r2.Id)
	s.Require().NoError(err)
	s.Equal("lvl2", got2.Text)

	// Per-parent reply counts.
	c0, _ := s.repo.RepliesCount(root.Id)
	s.Equal(uint64(1), c0)
	c1, _ := s.repo.RepliesCount(r1.Id)
	s.Equal(uint64(1), c1)

	// Nothing leaked into any timeline: only the root tweet is in the
	// author's list, and the replier authored no top-level tweets.
	authorList, _, err := s.repo.List(author, &limit, nil)
	s.Require().NoError(err)
	s.Len(authorList, 1)
	s.Equal(root.Id, authorList[0].Id)
	replierList, _, err := s.repo.List(replier, &limit, nil)
	s.Require().NoError(err)
	s.Len(replierList, 0)

	// Deleting the mid-level reply clears its parent's scan and count.
	_, err = s.repo.DeleteReply(root.Id, r1.Id)
	s.Require().NoError(err)
	lvl1, _, err = s.repo.GetReplies(root.Id, &limit, nil)
	s.Require().NoError(err)
	s.Len(lvl1, 0)
	c0, _ = s.repo.RepliesCount(root.Id)
	s.Equal(uint64(0), c0)
}

// TestThreadNestingDeep builds a 10-level deep reply chain (each reply
// answers the previous one) and verifies the storage holds it correctly:
// every level is its parent's single direct reply, every reply keeps the
// original thread RootId, every point lookup resolves under the right parent,
// per-parent counts are 1, and none of the 10 replies leak into a timeline.
func (s *TweetRepoTestSuite) TestThreadNestingDeep() {
	const depth = 10
	author := ulid.Make().String()
	replier := ulid.Make().String()

	root, err := s.repo.Create(author, domain.Tweet{UserId: author, Text: "root"})
	s.Require().NoError(err)

	// Build the chain: chain[0] is the root tweet, chain[i] replies to chain[i-1].
	chain := []domain.Tweet{root}
	for i := 1; i <= depth; i++ {
		parentID := chain[i-1].Id
		r, err := s.repo.AddReply(domain.Tweet{
			UserId:   replier,
			Text:     "lvl" + strconv.Itoa(i),
			RootId:   root.Id,
			ParentId: &parentID,
		})
		s.Require().NoError(err)
		chain = append(chain, r)
	}

	limit := uint64(10)
	for i := 1; i <= depth; i++ {
		parentID := chain[i-1].Id
		reply := chain[i]

		// RootId is preserved at every depth (never rewritten to the reply id).
		s.Equal(root.Id, reply.RootId, "level %d must keep the thread root", i)

		// Each parent has exactly its one direct reply.
		kids, _, err := s.repo.GetReplies(parentID, &limit, nil)
		s.Require().NoError(err)
		s.Require().Len(kids, 1, "level %d parent must have 1 direct reply", i)
		s.Equal(reply.Id, kids[0].Id)

		// Point lookup resolves under the correct parent.
		got, err := s.repo.GetReply(parentID, reply.Id)
		s.Require().NoError(err)
		s.Equal("lvl"+strconv.Itoa(i), got.Text)

		// Per-parent reply count.
		c, err := s.repo.RepliesCount(parentID)
		s.Require().NoError(err)
		s.Equal(uint64(1), c, "level %d parent count", i)
	}

	// The deepest reply has no children.
	leaf, _, err := s.repo.GetReplies(chain[depth].Id, &limit, nil)
	s.Require().NoError(err)
	s.Len(leaf, 0)

	// No reply leaked into a timeline: only the root tweet is in the author's
	// list, and the replier authored no top-level tweets.
	authorList, _, err := s.repo.List(author, &limit, nil)
	s.Require().NoError(err)
	s.Len(authorList, 1)
	s.Equal(root.Id, authorList[0].Id)
	replierList, _, err := s.repo.List(replier, &limit, nil)
	s.Require().NoError(err)
	s.Len(replierList, 0)

	// Deleting a mid-chain reply clears its parent's scan/count; deeper
	// replies remain stored under their own parents.
	mid := depth / 2
	_, err = s.repo.DeleteReply(chain[mid-1].Id, chain[mid].Id)
	s.Require().NoError(err)
	gone, _, err := s.repo.GetReplies(chain[mid-1].Id, &limit, nil)
	s.Require().NoError(err)
	s.Len(gone, 0)
	c, err := s.repo.RepliesCount(chain[mid-1].Id)
	s.Require().NoError(err)
	s.Equal(uint64(0), c)
	stillThere, err := s.repo.GetReply(chain[mid].Id, chain[mid+1].Id)
	s.Require().NoError(err)
	s.Equal("lvl"+strconv.Itoa(mid+1), stillThere.Text)
}

func (s *TweetRepoTestSuite) TestPinAndUnpin() {
	userId := ulid.Make().String()
	tweet := domain.Tweet{UserId: userId, Text: "pin me"}

	created, err := s.repo.Create(userId, tweet)
	s.Require().NoError(err)
	s.False(created.Pinned)

	pinned, err := s.repo.Pin(userId, created.Id)
	s.Require().NoError(err)
	s.True(pinned.Pinned)
	s.Equal(created.Id, pinned.Id)

	// Re-pin is a no-op: same state, no error.
	pinned2, err := s.repo.Pin(userId, created.Id)
	s.Require().NoError(err)
	s.True(pinned2.Pinned)

	fetched, err := s.repo.Get(userId, created.Id)
	s.Require().NoError(err)
	s.True(fetched.Pinned)

	unpinned, err := s.repo.Unpin(userId, created.Id)
	s.Require().NoError(err)
	s.False(unpinned.Pinned)

	fetched, err = s.repo.Get(userId, created.Id)
	s.Require().NoError(err)
	s.False(fetched.Pinned)
}

func (s *TweetRepoTestSuite) TestPinEmptyValidation() {
	_, err := s.repo.Pin("", "t")
	s.Error(err)
	_, err = s.repo.Pin("u", "")
	s.Error(err)
	_, err = s.repo.Unpin("", "t")
	s.Error(err)
	_, err = s.repo.Unpin("u", "")
	s.Error(err)
}

func (s *TweetRepoTestSuite) TestPinNonexistent() {
	_, err := s.repo.Pin(ulid.Make().String(), ulid.Make().String())
	s.Error(err)
}

func (s *TweetRepoTestSuite) TestDeleteTweet() {
	userId := ulid.Make().String()
	tweet := domain.Tweet{UserId: userId, Text: "to delete"}

	created, err := s.repo.Create(userId, tweet)
	s.Require().NoError(err)

	err = s.repo.Delete(userId, created.Id)
	s.Require().NoError(err)

	_, err = s.repo.Get(userId, created.Id)
	s.Error(err)
}

func (s *TweetRepoTestSuite) TestTweetsCount() {
	userId := ulid.Make().String()
	tweet := domain.Tweet{UserId: userId, Text: "counted"}

	_, err := s.repo.Create(userId, tweet)
	s.Require().NoError(err)

	count, err := s.repo.TweetsCount(userId)
	s.Require().NoError(err)
	s.Equal(uint64(1), count)
}

func (s *TweetRepoTestSuite) TestListTweets() {
	userId := ulid.Make().String()
	for i := 0; i < 3; i++ {
		_, err := s.repo.Create(userId, domain.Tweet{
			UserId:    userId,
			Text:      "tweet",
			CreatedAt: time.Now().Add(-time.Duration(i) * time.Second),
		})
		s.Require().NoError(err)
	}

	limit := uint64(10)
	tweets, cursor, err := s.repo.List(userId, &limit, nil)
	s.Require().NoError(err)
	s.Len(tweets, 3)
	s.Equal(cursor, "end")
}

func (s *TweetRepoTestSuite) TestRetweetAndRetweeters() {
	original := domain.Tweet{
		UserId:    ulid.Make().String(),
		Text:      "original",
		CreatedAt: time.Now(),
	}
	original, err := s.repo.Create(original.UserId, original)
	s.Require().NoError(err)

	retweeter := ulid.Make().String()
	retweeted := original
	retweeted.RetweetedBy = &retweeter
	retweeted.UserId = retweeter

	_, err = s.repo.NewRetweet(retweeted)
	s.Require().NoError(err)

	count, err := s.repo.RetweetsCount(original.Id)
	s.Require().NoError(err)
	s.Equal(uint64(1), count)

	retweeters, _, err := s.repo.Retweeters(original.Id, nil, nil)
	s.Require().NoError(err)
	s.Equal([]string{retweeter}, retweeters)
}

func (s *TweetRepoTestSuite) TestUnRetweet() {
	original := domain.Tweet{
		Id:        "TestUnRetweet",
		UserId:    ulid.Make().String(),
		Text:      "original",
		CreatedAt: time.Now(),
	}
	original, err := s.repo.Create(original.UserId, original)
	s.Require().NoError(err)

	retweeterId := ulid.Make().String()
	retweet := original
	retweet.RetweetedBy = &retweeterId

	created, err := s.repo.NewRetweet(retweet)
	s.Require().NoError(err)

	err = s.repo.UnRetweet(retweeterId, created.Id)
	s.Require().NoError(err)

	count, err := s.repo.RetweetsCount(original.Id)
	s.Require().NoError(err)
	s.Equal(uint64(0), count)
}

func (s *TweetRepoTestSuite) TestRecordView_IncrementsAndDedupes() {
	tweetId := ulid.Make().String()
	viewerA := ulid.Make().String()
	viewerB := ulid.Make().String()

	count, err := s.repo.RecordView(tweetId, viewerA)
	s.Require().NoError(err)
	s.Equal(uint64(1), count)

	// Same viewer within TTL is a no-op.
	count, err = s.repo.RecordView(tweetId, viewerA)
	s.Require().NoError(err)
	s.Equal(uint64(1), count)

	// Different viewer increments.
	count, err = s.repo.RecordView(tweetId, viewerB)
	s.Require().NoError(err)
	s.Equal(uint64(2), count)

	got, err := s.repo.GetViewsCount(tweetId)
	s.Require().NoError(err)
	s.Equal(uint64(2), got)
}

func (s *TweetRepoTestSuite) TestRecordView_InvalidParams() {
	_, err := s.repo.RecordView("", "viewer")
	s.Error(err)

	_, err = s.repo.RecordView("tweet", "")
	s.Error(err)
}

func (s *TweetRepoTestSuite) TestGetViewsCount_NotFound() {
	tweetId := ulid.Make().String()
	_, err := s.repo.GetViewsCount(tweetId)
	s.EqualError(err, ErrViewsNotFound.Error())
}

func (s *TweetRepoTestSuite) TestGetViewsCount_EmptyId() {
	_, err := s.repo.GetViewsCount("")
	s.Error(err)
}

func (s *TweetRepoTestSuite) TestRecordView_ConcurrentSafe() {
	tweetId := ulid.Make().String()
	const viewers = 16

	errCh := make(chan error, viewers)
	var wg sync.WaitGroup
	wg.Add(viewers)
	for i := 0; i < viewers; i++ {
		viewerId := ulid.Make().String()
		go func() {
			defer wg.Done()
			if _, err := s.repo.RecordView(tweetId, viewerId); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		s.Require().NoError(err)
	}

	count, err := s.repo.GetViewsCount(tweetId)
	s.Require().NoError(err)
	s.Equal(uint64(viewers), count)
}

func (s *TweetRepoTestSuite) TestRecordView_DifferentTweetsLockIndependently() {
	// Two different tweets should not block each other on the
	// sharded lock pool; records under each are independent.
	tweetA := ulid.Make().String()
	tweetB := ulid.Make().String()
	viewer := ulid.Make().String()

	a, err := s.repo.RecordView(tweetA, viewer)
	s.Require().NoError(err)
	s.Equal(uint64(1), a)

	b, err := s.repo.RecordView(tweetB, viewer)
	s.Require().NoError(err)
	s.Equal(uint64(1), b)
}

func (s *TweetRepoTestSuite) TestAddAndGetReply() {
	parentID := ulid.Make().String()
	rootID := ulid.Make().String()
	reply := domain.Tweet{
		Id:        ulid.Make().String(),
		RootId:    rootID,
		ParentId:  &parentID,
		UserId:    "user123",
		Text:      "This is a reply",
		CreatedAt: time.Now(),
	}

	saved, err := s.repo.AddReply(reply)
	s.Require().NoError(err)
	s.NotEmpty(saved.Id)

	got, err := s.repo.GetReply(parentID, reply.Id)
	s.Require().NoError(err)
	s.Equal(reply.Text, got.Text)
	s.Equal(reply.UserId, got.UserId)
}

func (s *TweetRepoTestSuite) TestRepliesCount() {
	parentID := ulid.Make().String()
	rootID := ulid.Make().String()

	for i := 0; i < 3; i++ {
		reply := domain.Tweet{
			Id:        ulid.Make().String(),
			RootId:    rootID,
			ParentId:  &parentID,
			UserId:    "user123",
			Text:      "reply",
			CreatedAt: time.Now(),
		}
		_, err := s.repo.AddReply(reply)
		s.Require().NoError(err)
	}

	count, err := s.repo.RepliesCount(parentID)
	s.Require().NoError(err)
	s.Equal(uint64(3), count)
}

// fakeTweetStats is an in-memory TweetStatsStorer used to assert that the
// per-tweet counters consult the aggregated CRDT store, not only the local db.
type fakeTweetStats struct {
	mu   sync.Mutex
	vals map[string]uint64
}

func newFakeTweetStats() *fakeTweetStats {
	return &fakeTweetStats{vals: make(map[string]uint64)}
}

func (f *fakeTweetStats) GetAggregatedStat(key ds.Key) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.vals[key.String()], nil
}

func (f *fakeTweetStats) Increment(key ds.Key) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.vals[key.String()]++
	return nil
}

func (f *fakeTweetStats) Decrement(key ds.Key) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.vals[key.String()] > 0 {
		f.vals[key.String()]--
	}
	return nil
}

// TestRepliesCountReadsAggregatedStat guards the cross-stack reply counter:
// RepliesCount must prefer the aggregated CRDT stat (like likes/retweets/views),
// not only the local per-partition count. Otherwise the frontend reply counter
// reads back stale/zero and stays hidden.
func (s *TweetRepoTestSuite) TestRepliesCountReadsAggregatedStat() {
	stats := newFakeTweetStats()
	repo := NewTweetRepo(s.db, stats)

	parentID := ulid.Make().String()
	rootID := ulid.Make().String()

	for i := 0; i < 3; i++ {
		_, err := repo.AddReply(domain.Tweet{
			Id:        ulid.Make().String(),
			RootId:    rootID,
			ParentId:  &parentID,
			UserId:    "user123",
			Text:      "reply",
			CreatedAt: time.Now(),
		})
		s.Require().NoError(err)
	}

	// Simulate replies aggregated from other nodes via the CRDT that never
	// touched this node's local counter.
	countKey := tweetsCountKey(parentID).DatastoreKey()
	s.Require().NoError(stats.Increment(countKey))
	s.Require().NoError(stats.Increment(countKey))

	// Local count is 3, aggregated CRDT count is 5: RepliesCount must report 5.
	localCount, err := repo.TweetsCount(parentID)
	s.Require().NoError(err)
	s.Equal(uint64(3), localCount)

	count, err := repo.RepliesCount(parentID)
	s.Require().NoError(err)
	s.Equal(uint64(5), count)
}

func (s *TweetRepoTestSuite) TestDeleteReply() {
	parentID := ulid.Make().String()
	rootID := ulid.Make().String()
	replyID := ulid.Make().String()

	reply := domain.Tweet{
		Id:        replyID,
		RootId:    rootID,
		ParentId:  &parentID,
		UserId:    "user123",
		Text:      "to delete",
		CreatedAt: time.Now(),
	}

	_, err := s.repo.AddReply(reply)
	s.Require().NoError(err)

	deleted, err := s.repo.DeleteReply(parentID, replyID)
	s.Require().NoError(err)
	s.Equal(replyID, deleted.Id)
	s.Require().NotNil(deleted.ParentId)
	s.Equal(parentID, *deleted.ParentId)

	_, err = s.repo.GetReply(parentID, replyID)
	s.Error(err)
}

func (s *TweetRepoTestSuite) TestGetReplies() {
	rootID := ulid.Make().String()
	parentID := ulid.Make().String()

	for i := 0; i < 3; i++ {
		reply := domain.Tweet{
			Id:        ulid.Make().String(),
			RootId:    rootID,
			ParentId:  &parentID,
			UserId:    "user",
			Text:      "child",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		_, err := s.repo.AddReply(reply)
		s.Require().NoError(err)
	}

	limit := uint64(10)
	replies, cursor, err := s.repo.GetReplies(parentID, &limit, nil)
	s.Require().NoError(err)
	s.Len(replies, 3)
	s.Equal(cursor, "end")

	for _, reply := range replies {
		s.Equal("child", reply.Text)
	}
}

func TestTweetRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(TweetRepoTestSuite))
}

func (s *TweetRepoTestSuite) TestList_NewestFirst() {
	userId := ulid.Make().String()

	older, err := s.repo.Create(userId, domain.Tweet{
		UserId:    userId,
		Text:      "older",
		CreatedAt: time.Now().Add(-2 * time.Second),
	})
	s.Require().NoError(err)
	newer, err := s.repo.Create(userId, domain.Tweet{UserId: userId, Text: "newer"})
	s.Require().NoError(err)

	limit := uint64(10)
	tweets, _, err := s.repo.List(userId, &limit, nil)
	s.Require().NoError(err)
	// The exact length also proves the fixed lookup keys written by
	// Create are skipped by List.
	s.Require().Len(tweets, 2)
	s.Equal(newer.Id, tweets[0].Id)
	s.Equal(older.Id, tweets[1].Id)
}

func (s *TweetRepoTestSuite) TestRetweeters_Multiple() {
	author := ulid.Make().String()
	src, err := s.repo.Create(author, domain.Tweet{UserId: author, Text: "source"})
	s.Require().NoError(err)

	retweeter1 := ulid.Make().String()
	retweeter2 := ulid.Make().String()

	rt1 := src
	rt1.RetweetedBy = &retweeter1
	_, err = s.repo.NewRetweet(rt1)
	s.Require().NoError(err)

	rt2 := src
	rt2.RetweetedBy = &retweeter2
	_, err = s.repo.NewRetweet(rt2)
	s.Require().NoError(err)

	limit := uint64(10)
	retweeters, _, err := s.repo.Retweeters(src.Id, &limit, nil)
	s.Require().NoError(err)
	s.Require().Len(retweeters, 2)
	s.ElementsMatch([]string{retweeter1, retweeter2}, retweeters)
}

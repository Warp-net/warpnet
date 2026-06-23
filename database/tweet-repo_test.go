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
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

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

	tree, _, err := s.repo.GetRepliesTree(tweet.Id, tweet.Id, &limit, nil)
	s.Require().NoError(err)
	s.Require().Len(tree, 1)
	s.Equal(reply.Id, tree[0].Reply.Id)

	// The parent's reply counter incremented.
	count, err := s.repo.RepliesCount(tweet.Id)
	s.Require().NoError(err)
	s.Equal(uint64(1), count)

	// Deleting the reply removes it from the thread and decrements the count.
	s.Require().NoError(s.repo.DeleteReply(tweet.Id, tweet.Id, reply.Id))
	_, err = s.repo.GetReply(tweet.Id, reply.Id)
	s.Error(err)
	count, err = s.repo.RepliesCount(tweet.Id)
	s.Require().NoError(err)
	s.Equal(uint64(0), count)
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

func TestTweetRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(TweetRepoTestSuite))
}

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

	"go.uber.org/goleak"

	"github.com/Warp-net/warpnet/database/local"
	"github.com/Warp-net/warpnet/domain"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/suite"
)

type TweetRepoTestSuite struct {
	suite.Suite

	db   *local.DB
	repo *TweetRepo
}

func (s *TweetRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local.New("", local.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	auth := NewAuthRepo(s.db)
	s.Require().NoError(auth.Authenticate("test", "test"))

	s.repo = NewTweetRepo(s.db)
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

func TestTweetRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(TweetRepoTestSuite))
}

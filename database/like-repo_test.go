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

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/suite"
)

type LikeRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *LikeRepo
}

func (s *LikeRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	authRepo := NewAuthRepo(s.db, "test")
	err = authRepo.Authenticate("test", "test")
	s.Require().NoError(err)

	s.repo = NewLikeRepo(s.db, nil)
}

func (s *LikeRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *LikeRepoTestSuite) TestLikeAndUnlike() {
	userId := ulid.Make().String()
	tweetId := ulid.Make().String()

	// Like
	likes, err := s.repo.Like(tweetId, userId, true)
	s.Require().NoError(err)
	s.Equal(uint64(1), likes)

	// Like again (should not increment)
	likes, err = s.repo.Like(tweetId, userId, true)
	s.Require().NoError(err)
	s.Equal(uint64(1), likes)

	// Check count directly
	count, err := s.repo.LikesCount(tweetId)
	s.Require().NoError(err)
	s.Equal(uint64(1), count)

	// Check likers
	limit := uint64(10)
	likers, cur, err := s.repo.Likers(tweetId, &limit, nil)
	s.Require().NoError(err)
	s.Len(likers, 1)
	s.Equal(cur, "end")
	s.Equal(userId, likers[0])

	// Unlike
	likes, err = s.repo.Unlike(tweetId, userId, true)
	s.Require().NoError(err)
	s.Equal(uint64(0), likes)

	// Unlike again (should not fail)
	likes, err = s.repo.Unlike(tweetId, userId, true)
	s.Require().NoError(err)
	s.Equal(uint64(0), likes)

	// Check likers now
	likers, _, err = s.repo.Likers(tweetId, &limit, nil)
	s.Require().NoError(err)
	s.Len(likers, 0)
}

func (s *LikeRepoTestSuite) TestLike_InvalidParams() {
	tweetId := ulid.Make().String()
	userId := ulid.Make().String()

	_, err := s.repo.Like("", userId, true)
	s.Error(err)

	_, err = s.repo.Like(tweetId, "", true)
	s.Error(err)

	_, err = s.repo.Unlike("", userId, true)
	s.Error(err)

	_, err = s.repo.Unlike(tweetId, "", true)
	s.Error(err)

	_, err = s.repo.LikesCount("")
	s.Error(err)

	_, _, err = s.repo.Likers("", nil, nil)
	s.Error(err)
}

func (s *LikeRepoTestSuite) TestLikesCount_NotFound() {
	id := ulid.Make().String()
	_, err := s.repo.LikesCount(id)
	s.EqualError(err, ErrLikesNotFound.Error())
}

func (s *LikeRepoTestSuite) TestLikers_Empty() {
	tweetId := ulid.Make().String()
	limit := uint64(10)
	likers, cur, err := s.repo.Likers(tweetId, &limit, nil)
	s.Require().NoError(err)
	s.Empty(likers)
	s.Equal(cur, "end")
}

func TestLikeRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(LikeRepoTestSuite))
}

func (s *LikeRepoTestSuite) TestLikedIndex() {
	userId := ulid.Make().String()
	ownerId := ulid.Make().String()
	tweetId := ulid.Make().String()

	// Empty before anything is liked.
	limit := uint64(10)
	liked, cur, err := s.repo.Liked(userId, &limit, nil)
	s.Require().NoError(err)
	s.Empty(liked)
	s.Equal("end", cur)

	// Index a liked tweet.
	err = s.repo.SetLiked(userId, tweetId, ownerId)
	s.Require().NoError(err)

	// Indexing again is a no-op, not a duplicate.
	err = s.repo.SetLiked(userId, tweetId, ownerId)
	s.Require().NoError(err)

	liked, cur, err = s.repo.Liked(userId, &limit, nil)
	s.Require().NoError(err)
	s.Require().Len(liked, 1)
	s.Equal("end", cur)
	s.Equal(userId, liked[0].UserId)
	s.Equal(tweetId, liked[0].TweetId)
	s.Equal(ownerId, liked[0].OwnerUserId)

	// A later like must come back first (newest-liked-first ordering).
	laterTweetId := ulid.Make().String()
	time.Sleep(2 * time.Millisecond)
	err = s.repo.SetLiked(userId, laterTweetId, ownerId)
	s.Require().NoError(err)

	liked, _, err = s.repo.Liked(userId, &limit, nil)
	s.Require().NoError(err)
	s.Require().Len(liked, 2)
	s.Equal(laterTweetId, liked[0].TweetId)
	s.Equal(tweetId, liked[1].TweetId)

	err = s.repo.RemoveLiked(userId, laterTweetId)
	s.Require().NoError(err)

	// Remove and verify the index is empty again.
	err = s.repo.RemoveLiked(userId, tweetId)
	s.Require().NoError(err)

	// Removing again should not fail.
	err = s.repo.RemoveLiked(userId, tweetId)
	s.Require().NoError(err)

	liked, _, err = s.repo.Liked(userId, &limit, nil)
	s.Require().NoError(err)
	s.Empty(liked)
}

func (s *LikeRepoTestSuite) TestLikedIndex_InvalidParams() {
	id := ulid.Make().String()

	s.Error(s.repo.SetLiked("", id, id))
	s.Error(s.repo.SetLiked(id, "", id))
	s.Error(s.repo.SetLiked(id, id, ""))
	s.Error(s.repo.RemoveLiked("", id))
	s.Error(s.repo.RemoveLiked(id, ""))
	_, _, err := s.repo.Liked("", nil, nil)
	s.Error(err)
}

func (s *LikeRepoTestSuite) TestLikers_Multiple() {
	tweetId := ulid.Make().String()
	user1 := ulid.Make().String()
	user2 := ulid.Make().String()

	_, err := s.repo.Like(tweetId, user1, true)
	s.Require().NoError(err)
	_, err = s.repo.Like(tweetId, user2, true)
	s.Require().NoError(err)

	limit := uint64(10)
	likers, _, err := s.repo.Likers(tweetId, &limit, nil)
	s.Require().NoError(err)
	s.Require().Len(likers, 2)
	s.ElementsMatch([]string{user1, user2}, likers)
}

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
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

type BookmarkRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *BookmarkRepo
}

func (s *BookmarkRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	authRepo := NewAuthRepo(s.db, "test")
	err = authRepo.Authenticate("test", "test")
	s.Require().NoError(err)

	s.repo = NewBookmarkRepo(s.db)
}

func (s *BookmarkRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *BookmarkRepoTestSuite) TestBookmarkAndList() {
	userId := uuid.New().String()
	tweetId := uuid.New().String()
	ownerId := uuid.New().String()

	err := s.repo.Bookmark(userId, tweetId, ownerId)
	s.Require().NoError(err)

	limit := uint64(10)
	bms, _, err := s.repo.List(userId, &limit, nil)
	s.Require().NoError(err)
	s.Len(bms, 1)
	s.Equal(tweetId, bms[0].TweetId)
	s.Equal(ownerId, bms[0].OwnerUserId)

	// A later bookmark must come back first (newest-first ordering).
	laterTweetId := uuid.New().String()
	time.Sleep(2 * time.Millisecond)
	s.Require().NoError(s.repo.Bookmark(userId, laterTweetId, ownerId))

	bms, _, err = s.repo.List(userId, &limit, nil)
	s.Require().NoError(err)
	s.Require().Len(bms, 2)
	s.Equal(laterTweetId, bms[0].TweetId)
	s.Equal(tweetId, bms[1].TweetId)
}

func (s *BookmarkRepoTestSuite) TestBookmarkIdempotent() {
	userId := uuid.New().String()
	tweetId := uuid.New().String()
	ownerId := uuid.New().String()

	s.Require().NoError(s.repo.Bookmark(userId, tweetId, ownerId))
	s.Require().NoError(s.repo.Bookmark(userId, tweetId, ownerId))

	limit := uint64(10)
	bms, _, err := s.repo.List(userId, &limit, nil)
	s.Require().NoError(err)
	s.Len(bms, 1)
}

func (s *BookmarkRepoTestSuite) TestUnbookmark() {
	userId := uuid.New().String()
	tweetId := uuid.New().String()
	ownerId := uuid.New().String()

	s.Require().NoError(s.repo.Bookmark(userId, tweetId, ownerId))
	s.Require().NoError(s.repo.Unbookmark(userId, tweetId))

	limit := uint64(10)
	bms, _, err := s.repo.List(userId, &limit, nil)
	s.Require().NoError(err)
	s.Empty(bms)
}

func (s *BookmarkRepoTestSuite) TestUnbookmark_NotExistsIsNoop() {
	userId := uuid.New().String()
	err := s.repo.Unbookmark(userId, "missing-tweet")
	s.Require().NoError(err)
}

func (s *BookmarkRepoTestSuite) TestEmptyValidation() {
	err := s.repo.Bookmark("", "t", "o")
	s.Error(err)
	err = s.repo.Bookmark("u", "", "o")
	s.Error(err)
	err = s.repo.Bookmark("u", "t", "")
	s.Error(err)

	err = s.repo.Unbookmark("", "t")
	s.Error(err)
	err = s.repo.Unbookmark("u", "")
	s.Error(err)

	_, _, err = s.repo.List("", nil, nil)
	s.Error(err)
}

func (s *BookmarkRepoTestSuite) TestIsolatedByUser() {
	u1 := uuid.New().String()
	u2 := uuid.New().String()
	s.Require().NoError(s.repo.Bookmark(u1, "t1", "o1"))
	s.Require().NoError(s.repo.Bookmark(u2, "t2", "o2"))

	limit := uint64(10)
	bms1, _, err := s.repo.List(u1, &limit, nil)
	s.Require().NoError(err)
	s.Len(bms1, 1)
	s.Equal("t1", bms1[0].TweetId)

	bms2, _, err := s.repo.List(u2, &limit, nil)
	s.Require().NoError(err)
	s.Len(bms2, 1)
	s.Equal("t2", bms2[0].TweetId)
}

func TestBookmarkRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(BookmarkRepoTestSuite))
}

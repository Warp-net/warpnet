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

package database

import (
	"testing"

	"github.com/Warp-net/warpnet/database/local"
	"github.com/Warp-net/warpnet/domain"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
)

type FollowRepoTestSuite struct {
	suite.Suite
	db   *local.DB
	repo *FollowRepo
}

func (s *FollowRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local.New(".", local.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	authRepo := NewAuthRepo(s.db)
	err = authRepo.Authenticate("test", "test")
	s.Require().NoError(err)

	s.repo = NewFollowRepo(s.db)
}

func (s *FollowRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *FollowRepoTestSuite) TestFollowAndUnfollow() {
	userA := ulid.Make().String()
	userB := ulid.Make().String()

	following := domain.Following{
		Follower: userA,
		Followee: userB,
	}

	// Follow
	err := s.repo.Follow(userA, userB, following)
	s.Require().NoError(err)

	// Check counts
	countFollowers, err := s.repo.GetFollowersCount(userA)
	s.Require().NoError(err)
	s.Equal(uint64(1), countFollowers)

	countFollowees, err := s.repo.GetFolloweesCount(userB)
	s.Require().NoError(err)
	s.Equal(uint64(1), countFollowees)

	// Check followers
	limit := uint64(10)
	followers, _, err := s.repo.GetFollowers(userB, &limit, nil)
	s.Require().NoError(err)
	s.Len(followers, 1)
	s.Equal(userA, followers[0].Follower)

	// Check followees
	followees, _, err := s.repo.GetFollowees(userA, &limit, nil)
	s.Require().NoError(err)
	s.Len(followees, 1)
	s.Equal(userB, followees[0].Followee)

	// Unfollow
	err = s.repo.Unfollow(userA, userB)
	s.Require().NoError(err)

	// Check counts after unfollowing
	countFollowers, err = s.repo.GetFollowersCount(userA)
	s.Require().NoError(err)
	s.Equal(uint64(0), countFollowers)

	countFollowees, err = s.repo.GetFolloweesCount(userB)
	s.Require().NoError(err)
	s.Equal(uint64(0), countFollowees)

	// Check empty lists
	followers, _, err = s.repo.GetFollowers(userB, &limit, nil)
	s.Require().NoError(err)
	s.Len(followers, 0)

	followees, _, err = s.repo.GetFollowees(userA, &limit, nil)
	s.Require().NoError(err)
	s.Len(followees, 0)
}

func (s *FollowRepoTestSuite) TestFollow_InvalidInput() {
	err := s.repo.Follow("", "userB", domain.Following{})
	s.Error(err)

	err = s.repo.Follow("userA", "", domain.Following{})
	s.Error(err)
}

func (s *FollowRepoTestSuite) TestGetFollowersCount_Empty() {
	count, err := s.repo.GetFollowersCount("")
	s.Error(err)
	s.Equal(uint64(0), count)
}

func (s *FollowRepoTestSuite) TestGetFolloweesCount_Empty() {
	count, err := s.repo.GetFolloweesCount("")
	s.Error(err)
	s.Equal(uint64(0), count)
}

func TestFollowRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(FollowRepoTestSuite))
}

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
	"github.com/Warp-net/warpnet/event"
	"go.uber.org/goleak"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/database/local"
	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/suite"
)

type UserRepoTestSuite struct {
	suite.Suite
	db   *local.DB
	repo *UserRepo
}

func (s *UserRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local.New(".", true)
	s.Require().NoError(err)

	authRepo := NewAuthRepo(s.db)
	err = authRepo.Authenticate("test", "test")
	s.Require().NoError(err)

	s.repo = NewUserRepo(s.db)
}

func (s *UserRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *UserRepoTestSuite) TestCreateAndGetUser() {
	user := domain.User{
		Id:        "user1",
		Username:  "testuser",
		CreatedAt: time.Now(),
	}
	created, err := s.repo.Create(user)
	s.Require().NoError(err)
	s.Equal(user.Id, created.Id)

	fetched, err := s.repo.Get(user.Id)
	s.Require().NoError(err)
	s.Equal(user.Id, fetched.Id)
	s.Equal(user.Username, fetched.Username)
}

func (s *UserRepoTestSuite) TestUpdateUser() {
	user := domain.User{
		Id:        "user2",
		Username:  "initial",
		CreatedAt: time.Now(),
	}
	_, err := s.repo.Create(user)
	s.Require().NoError(err)

	updated, err := s.repo.Update(user.Id, domain.User{
		Username: "updated",
	})
	s.Require().NoError(err)
	s.Equal("updated", updated.Username)
}

func (s *UserRepoTestSuite) TestDeleteUser() {
	user := domain.User{
		Id:        "user3",
		Username:  "todelete",
		CreatedAt: time.Now(),
	}
	_, err := s.repo.Create(user)
	s.Require().NoError(err)

	err = s.repo.Delete(user.Id)
	s.Require().NoError(err)

	_, err = s.repo.Get(user.Id)
	s.Equal(ErrUserNotFound, err)
}

func (s *UserRepoTestSuite) TestGetByNodeID() {
	user := domain.User{
		Id:        "user4",
		NodeId:    "node123",
		Username:  "nodeuser",
		CreatedAt: time.Now(),
	}
	_, err := s.repo.Create(user)
	s.Require().NoError(err)

	found, err := s.repo.GetByNodeID("node123")
	s.Require().NoError(err)
	s.Equal("user4", found.Id)
}

func (s *UserRepoTestSuite) TestListAndGetBatch() {
	users := []domain.User{
		{Id: "list1", Username: "a"},
		{Id: "list2", Username: "b"},
		{Id: "list3", Username: "c"},
	}
	for _, u := range users {
		u.CreatedAt = time.Now()
		_, err := s.repo.Create(u)
		s.Require().NoError(err)
	}

	limit := uint64(10)
	all, _, err := s.repo.List(&limit, nil)
	s.Require().NoError(err)
	s.GreaterOrEqual(len(all), 3)

	usersBatch, err := s.repo.GetBatch("list1", "list2")
	s.Require().NoError(err)
	s.Len(usersBatch, 2)
}

func (s *UserRepoTestSuite) TestValidateUser() {
	nodeId := "node123"
	user := domain.User{Id: "uniqueUser", CreatedAt: time.Now(), NodeId: nodeId}
	_, err := s.repo.Create(user)
	s.Require().NoError(err)

	err = s.repo.ValidateUserID(event.ValidationEvent{
		ValidatedNodeID: nodeId,
		SelfHashHex:     "",
		User:            &user,
	})
	s.NoError(err) // FIXME!

	user.Id = "nonexistent"

	err = s.repo.ValidateUserID(event.ValidationEvent{
		ValidatedNodeID: nodeId,
		SelfHashHex:     "",
		User:            &user,
	})
	s.NoError(err)
}

func TestUserRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(UserRepoTestSuite))
	closeWriter()
}

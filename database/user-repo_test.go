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
	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/suite"
)

type UserRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *UserRepo
}

func (s *UserRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	authRepo := NewAuthRepo(s.db, "test")
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

func (s *UserRepoTestSuite) TestSearch() {
	u1 := domain.User{Id: "u-alice", Username: "alice", Bio: "loves cats"}
	u2 := domain.User{Id: "u-bob", Username: "bob", Bio: "rust enthusiast"}
	u3 := domain.User{Id: "u-carol", Username: "carol", Bio: "alice's friend"}

	_, err := s.repo.Create(u1)
	s.Require().NoError(err)
	_, err = s.repo.Create(u2)
	s.Require().NoError(err)
	_, err = s.repo.Create(u3)
	s.Require().NoError(err)

	limit := uint64(50)
	hits, _, err := s.repo.Search("alice", &limit, nil)
	s.Require().NoError(err)
	// Matches by username (u1) and by bio (u3).
	ids := map[string]bool{}
	for _, u := range hits {
		ids[u.Id] = true
	}
	s.True(ids["u-alice"], "should match by username")
	s.True(ids["u-carol"], "should match by bio substring")
	s.False(ids["u-bob"], "should not match unrelated user")
}

func (s *UserRepoTestSuite) TestSearch_CaseInsensitive() {
	_, err := s.repo.Create(domain.User{Id: "u-mixed", Username: "MixedCase"})
	s.Require().NoError(err)

	hits, _, err := s.repo.Search("MIXED", nil, nil)
	s.Require().NoError(err)
	found := false
	for _, u := range hits {
		if u.Id == "u-mixed" {
			found = true
			break
		}
	}
	s.True(found, "search should be case-insensitive")
}

func (s *UserRepoTestSuite) TestSearch_EmptyQuery() {
	_, _, err := s.repo.Search("", nil, nil)
	s.Error(err)
}

func (s *UserRepoTestSuite) TestUpdatePersistsIsOffline() {
	u := domain.User{Id: "off-toggle", Username: "toggle", NodeId: "off-node-t", CreatedAt: time.Now()}
	_, err := s.repo.Create(u)
	s.Require().NoError(err)

	// Regression: Update used to drop IsOffline, so offline-marking never stuck.
	_, err = s.repo.Update(u.Id, domain.User{IsOffline: true})
	s.Require().NoError(err)
	got, err := s.repo.Get(u.Id)
	s.Require().NoError(err)
	s.True(got.IsOffline, "Update must persist IsOffline=true")

	_, err = s.repo.Update(u.Id, domain.User{IsOffline: false})
	s.Require().NoError(err)
	got, err = s.repo.Get(u.Id)
	s.Require().NoError(err)
	s.False(got.IsOffline, "Update must persist IsOffline=false")
}

func (s *UserRepoTestSuite) TestMarkForeignNodeUsersOffline() {
	node := "off-shared-node"
	owner := domain.User{Id: "off-owner", NodeId: node, Username: "owner", CreatedAt: time.Now()}
	ghost := domain.User{Id: "off-ghost", NodeId: node, Username: "ghost", CreatedAt: time.Now()}
	other := domain.User{Id: "off-other", NodeId: "off-other-node", Username: "other", CreatedAt: time.Now()}
	for _, u := range []domain.User{owner, ghost, other} {
		_, err := s.repo.Create(u)
		s.Require().NoError(err)
	}

	err := s.repo.MarkForeignNodeUsersOffline(node, owner.Id)
	s.Require().NoError(err)

	gotOwner, err := s.repo.Get(owner.Id)
	s.Require().NoError(err)
	s.False(gotOwner.IsOffline, "node owner must stay online")

	gotGhost, err := s.repo.Get(ghost.Id)
	s.Require().NoError(err)
	s.True(gotGhost.IsOffline, "stale same-node user must be marked offline")

	gotOther, err := s.repo.Get(other.Id)
	s.Require().NoError(err)
	s.False(gotOther.IsOffline, "user on another node must be untouched")
}

func TestUserRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(UserRepoTestSuite))
}

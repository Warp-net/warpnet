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

	"go.uber.org/goleak"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

type BlocksRepoTestSuite struct {
	suite.Suite
	db *local_store.DB
}

func (s *BlocksRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	authRepo := NewAuthRepo(s.db, "test")
	s.Require().NoError(authRepo.Authenticate("test", "test"))
}

func (s *BlocksRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *BlocksRepoTestSuite) TestBlockUnblockList() {
	repo := NewBlocksRepo(s.db)
	blocker := uuid.New().String()
	blockee := uuid.New().String()

	has, err := repo.IsBlocked(blocker, blockee)
	s.Require().NoError(err)
	s.False(has)

	s.Require().NoError(repo.Block(blocker, blockee))
	has, err = repo.IsBlocked(blocker, blockee)
	s.Require().NoError(err)
	s.True(has)

	limit := uint64(10)
	ids, _, err := repo.List(blocker, &limit, nil)
	s.Require().NoError(err)
	s.Len(ids, 1)

	s.Require().NoError(repo.Unblock(blocker, blockee))
	has, err = repo.IsBlocked(blocker, blockee)
	s.Require().NoError(err)
	s.False(has)
}

func (s *BlocksRepoTestSuite) TestValidation() {
	blocks := NewBlocksRepo(s.db)
	s.Error(blocks.Block("", "x"))
	s.Error(blocks.Block("x", ""))
	s.Error(blocks.Unblock("", "x"))
	s.Error(blocks.Unblock("x", ""))
	_, _, err := blocks.List("", nil, nil)
	s.Error(err)

	has, err := blocks.IsBlocked("", "x")
	s.NoError(err)
	s.False(has)
}

func TestBlocksRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(BlocksRepoTestSuite))
}

func (s *BlocksRepoTestSuite) TestList_Multiple() {
	repo := NewBlocksRepo(s.db)
	blocker := uuid.New().String()
	blockee1 := uuid.New().String()
	blockee2 := uuid.New().String()

	s.Require().NoError(repo.Block(blocker, blockee1))
	s.Require().NoError(repo.Block(blocker, blockee2))

	limit := uint64(10)
	ids, _, err := repo.List(blocker, &limit, nil)
	s.Require().NoError(err)
	s.Require().Len(ids, 2)
	s.ElementsMatch([]string{blockee1, blockee2}, ids)
}

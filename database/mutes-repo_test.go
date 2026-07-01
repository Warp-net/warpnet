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

type MutesRepoTestSuite struct {
	suite.Suite
	db *local_store.DB
}

func (s *MutesRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	authRepo := NewAuthRepo(s.db, "test")
	s.Require().NoError(authRepo.Authenticate("test", "test"))
}

func (s *MutesRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *MutesRepoTestSuite) TestMuteUnmute() {
	repo := NewMutesRepo(s.db)
	muter := uuid.New().String()
	mutee := uuid.New().String()

	s.Require().NoError(repo.Mute(muter, mutee))
	has, err := repo.IsMuted(muter, mutee)
	s.Require().NoError(err)
	s.True(has)

	s.Require().NoError(repo.Unmute(muter, mutee))
	has, err = repo.IsMuted(muter, mutee)
	s.Require().NoError(err)
	s.False(has)
}

func (s *MutesRepoTestSuite) TestUnmuteNonexistentIsNoop() {
	repo := NewMutesRepo(s.db)
	s.NoError(repo.Unmute(uuid.New().String(), uuid.New().String()))
}

func TestMutesRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(MutesRepoTestSuite))
}

func (s *MutesRepoTestSuite) TestList_Multiple() {
	repo := NewMutesRepo(s.db)
	muter := uuid.New().String()
	mutee1 := uuid.New().String()
	mutee2 := uuid.New().String()

	s.Require().NoError(repo.Mute(muter, mutee1))
	s.Require().NoError(repo.Mute(muter, mutee2))

	limit := uint64(10)
	ids, _, err := repo.List(muter, &limit, nil)
	s.Require().NoError(err)
	s.Require().Len(ids, 2)
	s.ElementsMatch([]string{mutee1, mutee2}, ids)
}

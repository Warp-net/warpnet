/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU General Public License for more details.

 You should have received a copy of the GNU General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

package database

import (
	"go.uber.org/goleak"
	"os"
	"testing"

	"github.com/Warp-net/warpnet/database/storage"
	"github.com/stretchr/testify/suite"
)

type ConsensusRepoTestSuite struct {
	suite.Suite
	db   *storage.DB
	repo *ConsensusRepo
}

func (s *ConsensusRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = storage.New(".", true, "")
	s.Require().NoError(err)
	authRepo := NewAuthRepo(s.db)

	err = authRepo.Authenticate("test", "test")
	s.Require().NoError(err)
	s.repo = NewConsensusRepo(s.db)
}

func (s *ConsensusRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *ConsensusRepoTestSuite) TestSetAndGet() {
	key := []byte("config-key")
	value := []byte("config-value")

	err := s.repo.Set(key, value)
	s.Require().NoError(err)

	got, err := s.repo.Get(key)
	s.Require().NoError(err)
	s.Equal(value, got)
}

func (s *ConsensusRepoTestSuite) TestGet_NotFound() {
	_, err := s.repo.Get([]byte("nonexistent"))
	s.EqualError(err, ErrConsensusKeyNotFound.Error())
}

func (s *ConsensusRepoTestSuite) TestSetAndGetUint64() {
	key := []byte("counter")
	var expected uint64 = 42

	err := s.repo.SetUint64(key, expected)
	s.Require().NoError(err)

	actual, err := s.repo.GetUint64(key)
	s.Require().NoError(err)
	s.Equal(expected, actual)
}

func (s *ConsensusRepoTestSuite) TestGetUint64_DefaultZero() {
	val, err := s.repo.GetUint64([]byte("missing-counter"))
	s.Require().NoError(err)
	s.Equal(uint64(0), val)
}

func (s *ConsensusRepoTestSuite) TestPath() {
	path := s.repo.SnapshotsPath()
	s.NotEmpty(path)
}

func (s *ConsensusRepoTestSuite) TestSync() {
	err := s.repo.Sync()
	s.NoError(err)
}

func (s *ConsensusRepoTestSuite) TestReset() {
	err := s.repo.Set([]byte("testkey"), []byte("testvalue"))
	s.Require().NoError(err)

	err = os.MkdirAll(s.repo.SnapshotsPath(), 0o755)
	s.Require().NoError(err)

	_, err = os.Stat(s.repo.SnapshotsPath())
	s.Require().NoError(err)

	err = s.repo.Reset()
	s.Require().NoError(err)

	_, err = s.repo.Get([]byte("testkey"))
	s.Require().ErrorIs(err, ErrConsensusKeyNotFound)

	_, err = os.Stat(s.repo.SnapshotsPath())
	s.Require().True(os.IsNotExist(err))
}

func TestConsensusRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(ConsensusRepoTestSuite))
	closeWriter()
}

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
	log "github.com/ipfs/go-log/writer"
	"go.uber.org/goleak"
	"testing"

	"github.com/Warp-net/warpnet/database/storage"
	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type AuthRepoTestSuite struct {
	suite.Suite
	db   *storage.DB
	repo *AuthRepo
}

func (s *AuthRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = storage.New(".", true, "")
	s.Require().NoError(err)
	s.repo = NewAuthRepo(s.db)

	err = s.repo.Authenticate("test", "test")
	s.Require().NoError(err)
}

func (s *AuthRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *AuthRepoTestSuite) TestAuthenticate_Success() {
	assert.NotEmpty(s.T(), s.repo.SessionToken())
	assert.NotNil(s.T(), s.repo.PrivateKey())
}

func (s *AuthRepoTestSuite) TestAuthenticate_InvalidCredentials() {
	err := s.repo.Authenticate("", "")
	assert.Error(s.T(), err)
}

func (s *AuthRepoTestSuite) TestAuthenticate_DBNotInitialized() {
	var repo = &AuthRepo{}
	err := repo.Authenticate("test", "test")
	assert.Error(s.T(), err)
}

func (s *AuthRepoTestSuite) TestSessionTokenAndPrivateKey() {
	assert.NotEmpty(s.T(), s.repo.SessionToken())
	assert.NotNil(s.T(), s.repo.PrivateKey())
}

func (s *AuthRepoTestSuite) TestPrivateKey_PanicIfNil() {
	defer func() {
		if r := recover(); r == nil {
			s.T().Errorf("expected panic on nil private key")
		}
	}()
	repo := &AuthRepo{}
	_ = repo.PrivateKey()
}

func (s *AuthRepoTestSuite) TestSetAndGetOwner() {
	owner := domain.Owner{UserId: "owner123"}

	saved, err := s.repo.SetOwner(owner)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "owner123", saved.UserId)

	fetched := s.repo.GetOwner()
	assert.Equal(s.T(), saved.UserId, fetched.UserId)
}

func (s *AuthRepoTestSuite) TestSetOwner_SetsCreatedAt() {
	owner := domain.Owner{UserId: "john"}

	saved, err := s.repo.SetOwner(owner)
	assert.NoError(s.T(), err)
	assert.False(s.T(), saved.CreatedAt.IsZero())
}

func TestAuthRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(AuthRepoTestSuite))
	closeWriter()
}

func closeWriter() {
	defer func() { recover() }()
	_ = log.WriterGroup.Close()
}

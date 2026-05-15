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

type UserSetRepoTestSuite struct {
	suite.Suite
	db *local_store.DB
}

func (s *UserSetRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	authRepo := NewAuthRepo(s.db, "test")
	s.Require().NoError(authRepo.Authenticate("test", "test"))
}

func (s *UserSetRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

// Block / Mute / Subscriptions share the same UserSetRepo shape, so a single
// table-driven suite exercises all three by constructor.
func (s *UserSetRepoTestSuite) repos() map[string]*UserSetRepo {
	return map[string]*UserSetRepo{
		"blocks":        NewBlocksRepo(s.db),
		"mutes":         NewMutesRepo(s.db),
		"subscriptions": NewSubscriptionsRepo(s.db),
	}
}

func (s *UserSetRepoTestSuite) TestAddHasListRemove() {
	for name, repo := range s.repos() {
		s.Run(name, func() {
			owner := uuid.New().String()
			t1 := uuid.New().String()
			t2 := uuid.New().String()

			has, err := repo.Has(owner, t1)
			s.Require().NoError(err)
			s.False(has)

			s.Require().NoError(repo.Add(owner, t1))
			s.Require().NoError(repo.Add(owner, t2))

			has, err = repo.Has(owner, t1)
			s.Require().NoError(err)
			s.True(has)

			limit := uint64(10)
			ids, _, err := repo.List(owner, &limit, nil)
			s.Require().NoError(err)
			s.Len(ids, 2)

			s.Require().NoError(repo.Remove(owner, t1))
			has, err = repo.Has(owner, t1)
			s.Require().NoError(err)
			s.False(has)

			ids, _, err = repo.List(owner, &limit, nil)
			s.Require().NoError(err)
			s.Len(ids, 1)
			s.Equal(t2, ids[0])
		})
	}
}

func (s *UserSetRepoTestSuite) TestEmptyValidation() {
	repo := NewBlocksRepo(s.db)
	s.Error(repo.Add("", "x"))
	s.Error(repo.Add("x", ""))
	s.Error(repo.Remove("", "x"))
	s.Error(repo.Remove("x", ""))

	_, _, err := repo.List("", nil, nil)
	s.Error(err)

	// Has tolerates empty inputs (returns false, nil).
	has, err := repo.Has("", "x")
	s.NoError(err)
	s.False(has)
}

func (s *UserSetRepoTestSuite) TestIsolatedByOwner() {
	repo := NewBlocksRepo(s.db)
	o1 := uuid.New().String()
	o2 := uuid.New().String()
	target := uuid.New().String()

	s.Require().NoError(repo.Add(o1, target))

	has, err := repo.Has(o1, target)
	s.Require().NoError(err)
	s.True(has)

	has, err = repo.Has(o2, target)
	s.Require().NoError(err)
	s.False(has, "owner o2 should not see o1's block")
}

func (s *UserSetRepoTestSuite) TestRemoveNonexistentIsNoop() {
	repo := NewMutesRepo(s.db)
	err := repo.Remove(uuid.New().String(), uuid.New().String())
	s.NoError(err)
}

func (s *UserSetRepoTestSuite) TestConvMutes() {
	repo := NewConvMutesRepo(s.db)

	s.Error(repo.Mute("", "t"))
	s.Error(repo.Mute("u", ""))
	s.Error(repo.Unmute("", "t"))
	s.Error(repo.Unmute("u", ""))

	user := uuid.New().String()
	tweet := uuid.New().String()
	s.Require().NoError(repo.Mute(user, tweet))
	// idempotent
	s.Require().NoError(repo.Mute(user, tweet))
	s.Require().NoError(repo.Unmute(user, tweet))
	// non-existent unmute is a no-op
	s.Require().NoError(repo.Unmute(user, tweet))
}

func (s *UserSetRepoTestSuite) TestUserNotes() {
	repo := NewUserNoteRepo(s.db)
	self := uuid.New().String()
	target := uuid.New().String()

	// empty fetch returns zero, no error
	note, err := repo.GetNote(self, target)
	s.Require().NoError(err)
	s.Empty(note)

	// set + read
	s.Require().NoError(repo.SetNote(self, target, "lovely person"))
	note, err = repo.GetNote(self, target)
	s.Require().NoError(err)
	s.Equal("lovely person", note)

	// overwrite
	s.Require().NoError(repo.SetNote(self, target, "actually annoying"))
	note, err = repo.GetNote(self, target)
	s.Require().NoError(err)
	s.Equal("actually annoying", note)

	// empty clears (no error)
	s.Require().NoError(repo.SetNote(self, target, ""))
	note, err = repo.GetNote(self, target)
	s.Require().NoError(err)
	s.Empty(note)

	// validation
	s.Error(repo.SetNote("", target, "x"))
	s.Error(repo.SetNote(self, "", "x"))

	// empty input on Get is soft (return zero, no error)
	note, err = repo.GetNote("", target)
	s.NoError(err)
	s.Empty(note)
}

func TestUserSetRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(UserSetRepoTestSuite))
}

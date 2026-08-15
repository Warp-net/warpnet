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
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type AliasesRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *AliasesRepo
}

func (s *AliasesRepoTestSuite) SetupTest() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	s.Require().NoError(s.db.Run("test", "test"))
	s.repo = NewAliasesRepo(s.db)
}

func (s *AliasesRepoTestSuite) TearDownTest() {
	s.db.Close()
}

func (s *AliasesRepoTestSuite) TestSetAndGetAlias_RoundTrip() {
	const aliasNodeId = "12D3KooWQ3umNTQweTREML1gqyag4T2Ps82wLnHV7fUNQA8CnMa9"

	a := domain.Alias{
		NodeId:   aliasNodeId,
		Token:    "session-token",
		Platform: "android",
	}
	s.Require().NoError(s.repo.SetAlias(a))

	aliases, err := s.repo.GetAliases()
	s.Require().NoError(err)
	s.Require().Len(aliases, 1)
	got := aliases[0]

	assert.Equal(s.T(), aliasNodeId, got.NodeId)
	assert.Equal(s.T(), a.Token, got.Token)
	assert.Equal(s.T(), a.Platform, got.Platform)
	assert.NotEmpty(s.T(), got.ID, "SetAlias should populate ID with a ULID")
	assert.False(s.T(), got.CreatedAt.IsZero(), "SetAlias should populate CreatedAt")
}

func (s *AliasesRepoTestSuite) TestGetAliases_Empty_ReturnsNoError() {
	aliases, err := s.repo.GetAliases()
	s.Require().NoError(err)
	assert.Empty(s.T(), aliases)
}

func (s *AliasesRepoTestSuite) TestGetNodeIDs() {
	peers := []string{
		"12D3KooWQ3umNTQweTREML1gqyag4T2Ps82wLnHV7fUNQA8CnMa9",
		"12D3KooWNXSGyfTuYc3JznW48jay73BtQgHszWfPpyF581EWcpGJ",
		"12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU",
	}
	for _, p := range peers {
		s.Require().NoError(s.repo.SetAlias(domain.Alias{
			NodeId: p,
			Token:  "tok-" + p,
		}))
	}

	ids, err := s.repo.GetNodeIDs()
	s.Require().NoError(err)
	assert.ElementsMatch(s.T(), peers, ids)
}

func (s *AliasesRepoTestSuite) TestGetAliases_NilRepo() {
	repo := &AliasesRepo{}
	_, err := repo.GetAliases()
	assert.ErrorIs(s.T(), err, ErrNilAliasesRepo)
}

func (s *AliasesRepoTestSuite) TestSetAlias_NilRepo() {
	repo := &AliasesRepo{}
	err := repo.SetAlias(domain.Alias{})
	assert.ErrorIs(s.T(), err, ErrNilAliasesRepo)
}

func TestAlias_JSONRoundTrip(t *testing.T) {
	a := domain.Alias{
		ID:         "01KPC4AP7FDEHD8CPTYZAKKDBK",
		CreatedAt:  time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC),
		NodeId:     "12D3KooWQ3umNTQweTREML1gqyag4T2Ps82wLnHV7fUNQA8CnMa9",
		Token:      "session-token",
		Platform:   "android",
		LastActive: time.Date(2026, 4, 27, 10, 5, 0, 0, time.UTC),
	}

	raw, err := json.Marshal(a)
	assert.NoError(t, err)

	var got domain.Alias
	err = json.Unmarshal(raw, &got)
	assert.NoError(t, err)

	assert.Equal(t, a.ID, got.ID)
	assert.Equal(t, a.NodeId, got.NodeId)
	assert.Equal(t, a.Token, got.Token)
	assert.Equal(t, a.Platform, got.Platform)
	assert.True(t, a.CreatedAt.Equal(got.CreatedAt))
	assert.True(t, a.LastActive.Equal(got.LastActive))
}

func TestAliasesRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(AliasesRepoTestSuite))
}

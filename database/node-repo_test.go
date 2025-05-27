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
	"context"
	"github.com/Warp-net/warpnet/security"
	"go.uber.org/goleak"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database/storage"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"github.com/stretchr/testify/suite"
)

type NodeRepoTestSuite struct {
	suite.Suite
	db   *storage.DB
	repo *NodeRepo
	ctx  context.Context
}

func (s *NodeRepoTestSuite) SetupSuite() {
	var err error
	s.ctx = context.Background()

	s.db, err = storage.New(".", true, "")
	s.Require().NoError(err)

	auth := NewAuthRepo(s.db)
	s.Require().NoError(auth.Authenticate("test", "test"))

	s.repo = NewNodeRepo(s.db)
}

func (s *NodeRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *NodeRepoTestSuite) TestPutGetHasDelete() {
	key := datastore.NewKey("test/key")
	value := []byte("hello")

	err := s.repo.Put(s.ctx, key, value)
	s.Require().NoError(err)

	got, err := s.repo.Get(s.ctx, key)
	s.Require().NoError(err)
	s.Equal(value, got)

	has, err := s.repo.Has(s.ctx, key)
	s.Require().NoError(err)
	s.True(has)

	err = s.repo.Delete(s.ctx, key)
	s.Require().NoError(err)

	_, err = s.repo.Get(s.ctx, key)
	s.ErrorIs(err, datastore.ErrNotFound)
}

func (s *NodeRepoTestSuite) TestPutWithTTLAndSetTTL() {
	key := datastore.NewKey("ttl/key")
	value := []byte("expiring")

	err := s.repo.PutWithTTL(s.ctx, key, value, time.Second*2)
	s.Require().NoError(err)

	// overwrite ttl
	time.Sleep(time.Second)
	err = s.repo.SetTTL(s.ctx, key, time.Second*5)
	s.Require().NoError(err)

	got, err := s.repo.Get(s.ctx, key)
	s.Require().NoError(err)
	s.Equal(value, got)
}

func (s *NodeRepoTestSuite) TestDiskUsage() {
	_, err := s.repo.DiskUsage(s.ctx)
	s.Require().NoError(err)
}

func (s *NodeRepoTestSuite) TestBlocklist() {
	pk, err := security.GenerateKeyFromSeed([]byte("peer123"))
	s.Require().NoError(err)

	id, err := warpnet.IDFromPublicKey(pk.Public())
	s.Require().NoError(err)

	err = s.repo.Blocklist24h(s.ctx, id)
	s.Require().NoError(err)

	isBlocked, err := s.repo.IsBlocklisted(s.ctx, id)
	s.Require().NoError(err)
	s.True(isBlocked)

	err = s.repo.BlocklistRemove(s.ctx, id)
	s.Require().NoError(err)

	isBlocked, err = s.repo.IsBlocklisted(s.ctx, id)
	s.Require().NoError(err)
	s.False(isBlocked)
}

func (s *NodeRepoTestSuite) TestQuerySimple() {
	key := datastore.NewKey("query/key")
	val := []byte("qval")
	err := s.repo.Put(s.ctx, key, val)
	s.Require().NoError(err)

	q := query.Query{Prefix: "query/key"}
	results, err := s.repo.Query(s.ctx, q)
	s.Require().NoError(err)
	s.Require().NotNil(results)

	defer results.Close()
	var found bool
	for r := range results.Next() {
		if r.Error != nil {
			continue
		}
		found = true
		break
	}
	s.True(found)
}

func TestNodeRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(NodeRepoTestSuite))
	closeWriter()
}

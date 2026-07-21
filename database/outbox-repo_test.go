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

	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/suite"
)

type OutboxRepoSuite struct {
	suite.Suite

	repo *OutboxRepo
	db   *local_store.DB
}

func (s *OutboxRepoSuite) SetupTest() {
	db, err := local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	s.Require().NoError(NewAuthRepo(db, "test").Authenticate("test", "test"))
	s.db = db
	s.repo = NewOutboxRepo(db)
}

func (s *OutboxRepoSuite) TearDownTest() {
	s.db.Close()
}

func (s *OutboxRepoSuite) TestEnqueueAndListFIFO() {
	node := ulid.Make().String()

	first, err := s.repo.Enqueue(node, "/public/post/message/0.0.0", []byte(`{"n":1}`))
	s.Require().NoError(err)
	second, err := s.repo.Enqueue(node, "/public/post/like/0.0.0", []byte(`{"n":2}`))
	s.Require().NoError(err)

	entries, err := s.repo.ListByNode(node)
	s.Require().NoError(err)
	s.Require().Len(entries, 2)
	// oldest first
	s.Equal(first.Id, entries[0].Id)
	s.Equal(second.Id, entries[1].Id)
	s.Equal([]byte(`{"n":1}`), entries[0].Payload)
	s.Equal("/public/post/like/0.0.0", entries[1].Route)
}

func (s *OutboxRepoSuite) TestDelete() {
	node := ulid.Make().String()
	entry, err := s.repo.Enqueue(node, "/public/post/message/0.0.0", []byte(`{}`))
	s.Require().NoError(err)

	s.Require().NoError(s.repo.Delete(node, entry.Id))

	entries, err := s.repo.ListByNode(node)
	s.Require().NoError(err)
	s.Empty(entries)
}

func (s *OutboxRepoSuite) TestSavePersistsAttempts() {
	node := ulid.Make().String()
	entry, err := s.repo.Enqueue(node, "/public/post/message/0.0.0", []byte(`{}`))
	s.Require().NoError(err)

	entry.Attempts = 3
	s.Require().NoError(s.repo.Save(entry))

	entries, err := s.repo.ListByNode(node)
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	s.Equal(3, entries[0].Attempts)
	s.Equal(entry.Id, entries[0].Id)
}

func (s *OutboxRepoSuite) TestListNodesDistinct() {
	nodeA := ulid.Make().String()
	nodeB := ulid.Make().String()

	_, err := s.repo.Enqueue(nodeA, "/public/post/message/0.0.0", []byte(`{}`))
	s.Require().NoError(err)
	_, err = s.repo.Enqueue(nodeA, "/public/post/like/0.0.0", []byte(`{}`))
	s.Require().NoError(err)
	_, err = s.repo.Enqueue(nodeB, "/public/post/message/0.0.0", []byte(`{}`))
	s.Require().NoError(err)

	nodes, err := s.repo.ListNodes()
	s.Require().NoError(err)
	s.ElementsMatch([]string{nodeA, nodeB}, nodes)
}

func (s *OutboxRepoSuite) TestListByNodeEmpty() {
	entries, err := s.repo.ListByNode(ulid.Make().String())
	s.Require().NoError(err)
	s.Empty(entries)
}

func TestOutboxRepoSuite(t *testing.T) {
	suite.Run(t, new(OutboxRepoSuite))
}

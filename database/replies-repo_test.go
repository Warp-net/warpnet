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
	"testing"
	"time"

	"github.com/Warp-net/warpnet/database/storage"
	"github.com/Warp-net/warpnet/domain"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/suite"
)

type ReplyRepoTestSuite struct {
	suite.Suite
	db   *storage.DB
	repo *ReplyRepo
}

func (s *ReplyRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = storage.New(".", true, "")
	s.Require().NoError(err)

	auth := NewAuthRepo(s.db)
	s.Require().NoError(auth.Authenticate("test", "test"))

	s.repo = NewRepliesRepo(s.db)
}

func (s *ReplyRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *ReplyRepoTestSuite) TestAddAndGetReply() {
	parentID := ulid.Make().String()
	rootID := ulid.Make().String()
	reply := domain.Tweet{
		Id:        ulid.Make().String(),
		RootId:    rootID,
		ParentId:  &parentID,
		UserId:    "user123",
		Text:      "This is a reply",
		CreatedAt: time.Now(),
	}

	saved, err := s.repo.AddReply(reply)
	s.Require().NoError(err)
	s.NotEmpty(saved.Id)

	got, err := s.repo.GetReply(rootID, reply.Id)
	s.Require().NoError(err)
	s.Equal(reply.Text, got.Text)
	s.Equal(reply.UserId, got.UserId)
}

func (s *ReplyRepoTestSuite) TestRepliesCount() {
	parentID := ulid.Make().String()
	rootID := ulid.Make().String()

	for i := 0; i < 3; i++ {
		reply := domain.Tweet{
			Id:        ulid.Make().String(),
			RootId:    rootID,
			ParentId:  &parentID,
			UserId:    "user123",
			Text:      "reply",
			CreatedAt: time.Now(),
		}
		_, err := s.repo.AddReply(reply)
		s.Require().NoError(err)
	}

	count, err := s.repo.RepliesCount(parentID)
	s.Require().NoError(err)
	s.Equal(uint64(3), count)
}

func (s *ReplyRepoTestSuite) TestDeleteReply() {
	parentID := ulid.Make().String()
	rootID := ulid.Make().String()
	replyID := ulid.Make().String()

	reply := domain.Tweet{
		Id:        replyID,
		RootId:    rootID,
		ParentId:  &parentID,
		UserId:    "user123",
		Text:      "to delete",
		CreatedAt: time.Now(),
	}

	_, err := s.repo.AddReply(reply)
	s.Require().NoError(err)

	err = s.repo.DeleteReply(rootID, parentID, replyID)
	s.Require().NoError(err)

	_, err = s.repo.GetReply(rootID, replyID)
	s.Error(err)
}

func (s *ReplyRepoTestSuite) TestGetRepliesTree() {
	rootID := ulid.Make().String()
	parentID := ulid.Make().String()

	// Создаем 3 реплая к parentID
	for i := 0; i < 3; i++ {
		reply := domain.Tweet{
			Id:        ulid.Make().String(),
			RootId:    rootID,
			ParentId:  &parentID,
			UserId:    "user",
			Text:      "child",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		_, err := s.repo.AddReply(reply)
		s.Require().NoError(err)
	}

	limit := uint64(10)
	tree, cursor, err := s.repo.GetRepliesTree(rootID, parentID, &limit, nil)
	s.Require().NoError(err)
	s.Len(tree, 3)
	s.Equal(cursor, "end")

	for _, node := range tree {
		s.Equal("child", node.Reply.Text)
	}
}

func TestReplyRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(ReplyRepoTestSuite))
	closeWriter()
}

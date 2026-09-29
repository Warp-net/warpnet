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
	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/suite"
)

type OrderRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *OrderRepo
}

func (s *OrderRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	authRepo := NewAuthRepo(s.db, "test")
	err = authRepo.Authenticate("test", "test")
	s.Require().NoError(err)

	s.repo = NewOrderRepo(s.db)
}

func (s *OrderRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *OrderRepoTestSuite) TestSaveAndGet() {
	p := domain.Order{TweetId: "t1", AuthorId: "a1", BuyerId: "b1", Nonce: "n1", TxId: "tx1"}
	s.Require().NoError(s.repo.Save(p))

	got, err := s.repo.Get("t1", "b1")
	s.Require().NoError(err)
	s.Equal("tx1", got.TxId)
	s.False(got.Confirmed)

	p.Confirmed = true
	p.Tweet = &domain.Tweet{Id: "t1", Text: "paid"}
	s.Require().NoError(s.repo.Save(p))

	got, err = s.repo.Get("t1", "b1")
	s.Require().NoError(err)
	s.True(got.Confirmed)
	s.Equal("paid", got.Tweet.Text)
}

func (s *OrderRepoTestSuite) TestGetIsPerBuyer() {
	s.Require().NoError(s.repo.Save(domain.Order{TweetId: "t2", BuyerId: "b1"}))

	_, err := s.repo.Get("t2", "b2")
	s.ErrorIs(err, ErrOrderNotFound)
}

func (s *OrderRepoTestSuite) TestEmptyValidation() {
	s.Error(s.repo.Save(domain.Order{BuyerId: "b1"}))
	s.Error(s.repo.Save(domain.Order{TweetId: "t1"}))
	_, err := s.repo.Get("", "b1")
	s.Error(err)
	_, err = s.repo.Get("t1", "")
	s.Error(err)
}

func TestOrderRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(OrderRepoTestSuite))
}

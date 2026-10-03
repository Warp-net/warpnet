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

func (s *OrderRepoTestSuite) TestCountConfirmed() {
	at := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	s.Require().NoError(s.repo.Save(domain.Order{TweetId: "t3", BuyerId: "c1", TxId: "tx3", Confirmed: true, CreatedAt: at}))
	s.Require().NoError(s.repo.Save(domain.Order{TweetId: "t4", BuyerId: "c1", TxId: "tx4", Confirmed: true, CreatedAt: at.Add(2 * time.Hour)}))
	s.Require().NoError(s.repo.Save(domain.Order{TweetId: "t5", BuyerId: "c1", TxId: "tx5", CreatedAt: at}))
	s.Require().NoError(s.repo.Save(domain.Order{TweetId: "t3", BuyerId: "xc1", TxId: "tx6", Confirmed: true, CreatedAt: at}))

	count, err := s.repo.CountConfirmed("c1", at.Add(-time.Hour), at.Add(time.Hour))
	s.Require().NoError(err)
	s.Equal(1, count, "only the buyer's own confirmed orders inside the window count")

	count, err = s.repo.CountConfirmed("nobody", at.Add(-time.Hour), at.Add(time.Hour))
	s.Require().NoError(err)
	s.Zero(count)

	_, err = s.repo.CountConfirmed("", at, at)
	s.Error(err)
}

func TestOrderRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(OrderRepoTestSuite))
}

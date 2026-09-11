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
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

type WalletRepoTestSuite struct {
	suite.Suite
	db *local_store.DB
}

func (s *WalletRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	authRepo := NewAuthRepo(s.db, "test")
	s.Require().NoError(authRepo.Authenticate("test", "test"))
}

func (s *WalletRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *WalletRepoTestSuite) TestSetGetList() {
	repo := NewWalletRepo(s.db)
	user := uuid.New().String()

	_, err := repo.GetAddress("tron-testnet", user)
	s.Require().ErrorIs(err, ErrWalletAddressNotFound)

	s.Require().NoError(repo.SetAddress("tron-testnet", user, "TAddrOne"))
	stored, err := repo.GetAddress("tron-testnet", user)
	s.Require().NoError(err)
	s.Equal("TAddrOne", stored.Address)
	s.Equal(user, stored.UserId)
	s.Equal("tron-testnet", stored.Chain)
	s.False(stored.UpdatedAt.IsZero())

	s.Require().NoError(repo.SetAddress("tron-testnet", user, "TAddrTwo"))
	stored, err = repo.GetAddress("tron-testnet", user)
	s.Require().NoError(err)
	s.Equal("TAddrTwo", stored.Address, "a later address must replace the earlier one")

	other := uuid.New().String()
	s.Require().NoError(repo.SetAddress("solana-mainnet", other, "SoLaNa"))

	limit := uint64(10)
	tron, _, err := repo.ListAddresses("tron-testnet", &limit, nil)
	s.Require().NoError(err)
	for _, item := range tron {
		s.Equal("tron-testnet", item.Chain, "listing one chain must not leak another chain's addresses")
	}

	solana, _, err := repo.ListAddresses("solana-mainnet", &limit, nil)
	s.Require().NoError(err)
	s.Len(solana, 1)
	s.Equal("SoLaNa", solana[0].Address)
}

func (s *WalletRepoTestSuite) TestValidation() {
	repo := NewWalletRepo(s.db)
	s.Error(repo.SetAddress("", "user", "TAddr"))
	s.Error(repo.SetAddress("tron", "", "TAddr"))
	s.Error(repo.SetAddress("tron", "user", ""))
	_, _, err := repo.ListAddresses("", nil, nil)
	s.Error(err)
}

func TestWalletRepoTestSuite(t *testing.T) {
	suite.Run(t, new(WalletRepoTestSuite))
}

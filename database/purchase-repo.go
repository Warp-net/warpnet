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

package database

import (
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
)

const PurchaseRepoName = "/PURCHASES"

var ErrPurchaseNotFound = local_store.DBError("purchase not found")

type PurchaseStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
}

type PurchaseRepo struct {
	db PurchaseStorer
}

func NewPurchaseRepo(db PurchaseStorer) *PurchaseRepo {
	return &PurchaseRepo{db: db}
}

func (repo *PurchaseRepo) Save(p domain.Purchase) error {
	if p.TweetId == "" {
		return local_store.DBError("empty tweet id")
	}
	if p.BuyerId == "" {
		return local_store.DBError("empty buyer id")
	}

	value, err := json.Marshal(p)
	if err != nil {
		return err
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err := txn.Set(purchaseKey(p.TweetId, p.BuyerId), value); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *PurchaseRepo) Get(tweetId, buyerId string) (domain.Purchase, error) {
	if tweetId == "" {
		return domain.Purchase{}, local_store.DBError("empty tweet id")
	}
	if buyerId == "" {
		return domain.Purchase{}, local_store.DBError("empty buyer id")
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return domain.Purchase{}, err
	}
	defer txn.Rollback()

	value, err := txn.Get(purchaseKey(tweetId, buyerId))
	if local_store.IsNotFoundError(err) {
		return domain.Purchase{}, ErrPurchaseNotFound
	}
	if err != nil {
		return domain.Purchase{}, err
	}

	var p domain.Purchase
	if err := json.Unmarshal(value, &p); err != nil {
		return domain.Purchase{}, err
	}
	if err := txn.Commit(); err != nil {
		return domain.Purchase{}, err
	}
	return p, nil
}

func purchaseKey(tweetId, buyerId string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(PurchaseRepoName).
		AddRootID(tweetId).
		AddParentId(buyerId).
		Build()
}

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
	"strings"
	"time"

	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
)

const OrderRepoName = "/ORDERS"

var ErrOrderNotFound = local_store.DBError("order not found")

type OrderStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
}

type OrderRepo struct {
	db OrderStorer
}

func NewOrderRepo(db OrderStorer) *OrderRepo {
	return &OrderRepo{db: db}
}

func (repo *OrderRepo) Save(o domain.Order) error {
	if o.TweetId == "" {
		return local_store.DBError("empty tweet id")
	}
	if o.BuyerId == "" {
		return local_store.DBError("empty buyer id")
	}

	value, err := json.Marshal(o)
	if err != nil {
		return err
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err := txn.Set(orderKey(o.TweetId, o.BuyerId), value); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *OrderRepo) Get(tweetId, buyerId string) (domain.Order, error) {
	if tweetId == "" {
		return domain.Order{}, local_store.DBError("empty tweet id")
	}
	if buyerId == "" {
		return domain.Order{}, local_store.DBError("empty buyer id")
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return domain.Order{}, err
	}
	defer txn.Rollback()

	value, err := txn.Get(orderKey(tweetId, buyerId))
	if local_store.IsNotFoundError(err) {
		return domain.Order{}, ErrOrderNotFound
	}
	if err != nil {
		return domain.Order{}, err
	}

	var o domain.Order
	if err := json.Unmarshal(value, &o); err != nil {
		return domain.Order{}, err
	}
	if err := txn.Commit(); err != nil {
		return domain.Order{}, err
	}
	return o, nil
}

func (repo *OrderRepo) CountConfirmed(buyerId string, from, to time.Time) (int, error) {
	if buyerId == "" {
		return 0, local_store.DBError("empty buyer id")
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()

	var keys []local_store.DatabaseKey
	prefix := local_store.DatabaseKey(OrderRepoName + local_store.Delimeter)
	err = txn.IterateKeys(prefix, func(key string) error {
		if strings.HasSuffix(key, local_store.Delimeter+buyerId) {
			keys = append(keys, local_store.DatabaseKey(key))
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if len(keys) == 0 {
		return 0, txn.Commit()
	}

	items, err := txn.BatchGet(keys...)
	if err != nil {
		return 0, err
	}
	var count int
	for _, item := range items {
		var o domain.Order
		if err := json.Unmarshal(item.Value, &o); err != nil {
			return 0, err
		}
		if o.Confirmed && !o.CreatedAt.Before(from) && !o.CreatedAt.After(to) {
			count++
		}
	}
	return count, txn.Commit()
}

func orderKey(tweetId, buyerId string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(OrderRepoName).
		AddRootID(tweetId).
		AddParentId(buyerId).
		Build()
}

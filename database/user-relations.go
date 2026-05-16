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
	"github.com/Warp-net/warpnet/database/local-store"
)

// userRelationStorer is the narrow slice of the kv store the
// per-relation repos (Blocks / Mutes / Subscriptions) need.
type userRelationStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
}

// addUserRelation writes a directional relation under namespace:
//
//	forward index   namespace / forwardSub / ownerId / otherId  ->  otherId
//	reverse index   namespace / reverseSub / otherId / ownerId  ->  ownerId
//
// Both writes happen inside one transaction so the two indexes stay
// in sync.
func addUserRelation(
	db userRelationStorer,
	namespace, forwardSub, reverseSub, ownerId, otherId string,
) error {
	if ownerId == "" {
		return local_store.DBError("empty owner id")
	}
	if otherId == "" {
		return local_store.DBError("empty other user id")
	}

	forward := local_store.NewPrefixBuilder(namespace).
		AddSubPrefix(forwardSub).
		AddRootID(ownerId).
		AddParentId(otherId).
		Build()
	reverse := local_store.NewPrefixBuilder(namespace).
		AddSubPrefix(reverseSub).
		AddRootID(otherId).
		AddParentId(ownerId).
		Build()

	txn, err := db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err = txn.Set(forward, []byte(otherId)); err != nil {
		return err
	}
	if err = txn.Set(reverse, []byte(ownerId)); err != nil {
		return err
	}
	return txn.Commit()
}

func removeUserRelation(
	db userRelationStorer,
	namespace, forwardSub, reverseSub, ownerId, otherId string,
) error {
	if ownerId == "" {
		return local_store.DBError("empty owner id")
	}
	if otherId == "" {
		return local_store.DBError("empty other user id")
	}

	forward := local_store.NewPrefixBuilder(namespace).
		AddSubPrefix(forwardSub).
		AddRootID(ownerId).
		AddParentId(otherId).
		Build()
	reverse := local_store.NewPrefixBuilder(namespace).
		AddSubPrefix(reverseSub).
		AddRootID(otherId).
		AddParentId(ownerId).
		Build()

	txn, err := db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err = txn.Delete(forward); err != nil && !local_store.IsNotFoundError(err) {
		return err
	}
	if err = txn.Delete(reverse); err != nil && !local_store.IsNotFoundError(err) {
		return err
	}
	return txn.Commit()
}

func hasUserRelation(
	db userRelationStorer,
	namespace, forwardSub, ownerId, otherId string,
) (bool, error) {
	if ownerId == "" || otherId == "" {
		return false, nil
	}
	forward := local_store.NewPrefixBuilder(namespace).
		AddSubPrefix(forwardSub).
		AddRootID(ownerId).
		AddParentId(otherId).
		Build()

	txn, err := db.NewTxn()
	if err != nil {
		return false, err
	}
	defer txn.Rollback()

	_, err = txn.Get(forward)
	if local_store.IsNotFoundError(err) {
		return false, txn.Commit()
	}
	if err != nil {
		return false, err
	}
	return true, txn.Commit()
}

func listUserRelations(
	db userRelationStorer,
	namespace, forwardSub, ownerId string,
	limit *uint64, cursor *string,
) ([]string, string, error) {
	if ownerId == "" {
		return nil, "", local_store.DBError("empty owner id")
	}
	prefix := local_store.NewPrefixBuilder(namespace).
		AddSubPrefix(forwardSub).
		AddRootID(ownerId).
		Build()

	txn, err := db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	items, cur, err := txn.List(prefix, limit, cursor)
	if err != nil {
		return nil, "", err
	}
	if err = txn.Commit(); err != nil {
		return nil, "", err
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, string(item.Value))
	}
	return ids, cur, nil
}

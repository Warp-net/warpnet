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

// FollowRequestsRepoName is the keyspace for pending follows on locked
// accounts. Layout mirrors UserSetRepo: a per-owner forward index from
// userId -> set<followerId>.
const FollowRequestsRepoName = "/FOLLOW_REQUESTS"

// FollowRequestsRepo holds pending follow requests for locked accounts.
// Once the target user authorizes a request, the entry is moved to the
// normal follow-repo and the request is removed; on reject, just removed.
type FollowRequestsRepo struct {
	db UserSetStorer
}

func NewFollowRequestsRepo(db UserSetStorer) *FollowRequestsRepo {
	return &FollowRequestsRepo{db: db}
}

func (repo *FollowRequestsRepo) Add(targetUserId, followerId string) error {
	if targetUserId == "" {
		return local_store.DBError("empty target user id")
	}
	if followerId == "" {
		return local_store.DBError("empty follower id")
	}
	key := local_store.NewPrefixBuilder(FollowRequestsRepoName).
		AddRootID(targetUserId).
		AddParentId(followerId).
		Build()
	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()
	if err := txn.Set(key, []byte(followerId)); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *FollowRequestsRepo) Remove(targetUserId, followerId string) error {
	if targetUserId == "" {
		return local_store.DBError("empty target user id")
	}
	if followerId == "" {
		return local_store.DBError("empty follower id")
	}
	key := local_store.NewPrefixBuilder(FollowRequestsRepoName).
		AddRootID(targetUserId).
		AddParentId(followerId).
		Build()
	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()
	if err := txn.Delete(key); err != nil && !local_store.IsNotFoundError(err) {
		return err
	}
	return txn.Commit()
}

func (repo *FollowRequestsRepo) Has(targetUserId, followerId string) (bool, error) {
	if targetUserId == "" || followerId == "" {
		return false, nil
	}
	key := local_store.NewPrefixBuilder(FollowRequestsRepoName).
		AddRootID(targetUserId).
		AddParentId(followerId).
		Build()
	txn, err := repo.db.NewTxn()
	if err != nil {
		return false, err
	}
	defer txn.Rollback()
	_, err = txn.Get(key)
	if local_store.IsNotFoundError(err) {
		return false, txn.Commit()
	}
	if err != nil {
		return false, err
	}
	return true, txn.Commit()
}

// List returns the follower ids pending approval for the given target user.
func (repo *FollowRequestsRepo) List(targetUserId string, limit *uint64, cursor *string) ([]string, string, error) {
	if targetUserId == "" {
		return nil, "", local_store.DBError("empty target user id")
	}
	prefix := local_store.NewPrefixBuilder(FollowRequestsRepoName).
		AddRootID(targetUserId).
		Build()
	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()
	items, cur, err := txn.List(prefix, limit, cursor)
	if err != nil {
		return nil, "", err
	}
	if err := txn.Commit(); err != nil {
		return nil, "", err
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, string(item.Value))
	}
	return ids, cur, nil
}

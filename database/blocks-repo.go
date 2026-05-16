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

// NOT to be confused with NodeRepo.Blocklist (database/node-repo.go),
// which is a *peer*-level anti-abuse mechanism keyed by libp2p PeerID.
// BlocksRepo below is the *social* block: permanent, user-scoped
// "hide this account from me" keyed by domain user id, kept until
// the user explicitly unblocks.

const (
	BlocksRepoName = "/BLOCKS"

	// blockeesSubName indexes per-blocker the user ids they have blocked.
	blockeesSubName = "BLOCKEES"
	// blockersSubName indexes per-blockee the user ids who blocked them
	// (used when we need to evaluate inbound traffic).
	blockersSubName = "BLOCKERS"
)

type BlocksStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
}

// BlocksRepo persists the set of user ids the local owner has blocked.
type BlocksRepo struct {
	db BlocksStorer
}

func NewBlocksRepo(db BlocksStorer) *BlocksRepo { return &BlocksRepo{db: db} }

// Block records that blockerId has blocked blockeeId.
func (repo *BlocksRepo) Block(blockerId, blockeeId string) error {
	if blockerId == "" {
		return local_store.DBError("empty blocker id")
	}
	if blockeeId == "" {
		return local_store.DBError("empty blockee id")
	}

	blockeesKey := local_store.NewPrefixBuilder(BlocksRepoName).
		AddSubPrefix(blockeesSubName).
		AddRootID(blockerId).
		AddParentId(blockeeId).
		Build()
	blockersKey := local_store.NewPrefixBuilder(BlocksRepoName).
		AddSubPrefix(blockersSubName).
		AddRootID(blockeeId).
		AddParentId(blockerId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err = txn.Set(blockeesKey, []byte(blockeeId)); err != nil {
		return err
	}
	if err = txn.Set(blockersKey, []byte(blockerId)); err != nil {
		return err
	}
	return txn.Commit()
}

// Unblock removes a previously-recorded block.
func (repo *BlocksRepo) Unblock(blockerId, blockeeId string) error {
	if blockerId == "" {
		return local_store.DBError("empty blocker id")
	}
	if blockeeId == "" {
		return local_store.DBError("empty blockee id")
	}

	blockeesKey := local_store.NewPrefixBuilder(BlocksRepoName).
		AddSubPrefix(blockeesSubName).
		AddRootID(blockerId).
		AddParentId(blockeeId).
		Build()
	blockersKey := local_store.NewPrefixBuilder(BlocksRepoName).
		AddSubPrefix(blockersSubName).
		AddRootID(blockeeId).
		AddParentId(blockerId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err = txn.Delete(blockeesKey); err != nil && !local_store.IsNotFoundError(err) {
		return err
	}
	if err = txn.Delete(blockersKey); err != nil && !local_store.IsNotFoundError(err) {
		return err
	}
	return txn.Commit()
}

func (repo *BlocksRepo) IsBlocked(blockerId, blockeeId string) (bool, error) {
	if blockerId == "" || blockeeId == "" {
		return false, nil
	}
	blockeesKey := local_store.NewPrefixBuilder(BlocksRepoName).
		AddSubPrefix(blockeesSubName).
		AddRootID(blockerId).
		AddParentId(blockeeId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return false, err
	}
	defer txn.Rollback()

	_, err = txn.Get(blockeesKey)
	if local_store.IsNotFoundError(err) {
		return false, txn.Commit()
	}
	if err != nil {
		return false, err
	}
	return true, txn.Commit()
}

func (repo *BlocksRepo) List(blockerId string, limit *uint64, cursor *string) ([]string, string, error) {
	if blockerId == "" {
		return nil, "", local_store.DBError("empty blocker id")
	}
	prefix := local_store.NewPrefixBuilder(BlocksRepoName).
		AddSubPrefix(blockeesSubName).
		AddRootID(blockerId).
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
	if err = txn.Commit(); err != nil {
		return nil, "", err
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, string(item.Value))
	}
	return ids, cur, nil
}

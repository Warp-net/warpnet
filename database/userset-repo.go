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

// BlocksRepo / MutesRepo / SubscriptionsRepo all share the same on-disk
// shape (per-owner forward index of related user ids) but are split into
// separate concrete types so each operation reads in isolation.
//
// NOT to be confused with NodeRepo.Blocklist (database/node-repo.go),
// which is a *peer*-level anti-abuse mechanism keyed by libp2p PeerID
// with escalating TTLs — that one bans misbehaving nodes from the
// network. The BlocksRepo below is the *social* block: permanent,
// user-scoped "hide this account from me" keyed by domain user id.

const (
	BlocksRepoName        = "/BLOCKS"
	MutesRepoName         = "/MUTES"
	SubscriptionsRepoName = "/SUBSCRIPTIONS" // local watchlist: whose new tweets I want notifications about
)

type userSetStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
}

// BlocksRepo persists the set of user ids the local owner has blocked.
type BlocksRepo struct {
	db userSetStorer
}

func NewBlocksRepo(db userSetStorer) *BlocksRepo { return &BlocksRepo{db: db} }

// Block records that blockerId has blocked blockeeId.
func (repo *BlocksRepo) Block(blockerId, blockeeId string) error {
	return userSetAdd(repo.db, BlocksRepoName, blockerId, blockeeId)
}

// Unblock removes a previously-recorded block.
func (repo *BlocksRepo) Unblock(blockerId, blockeeId string) error {
	return userSetRemove(repo.db, BlocksRepoName, blockerId, blockeeId)
}

func (repo *BlocksRepo) IsBlocked(blockerId, blockeeId string) (bool, error) {
	return userSetHas(repo.db, BlocksRepoName, blockerId, blockeeId)
}

func (repo *BlocksRepo) List(blockerId string, limit *uint64, cursor *string) ([]string, string, error) {
	return userSetList(repo.db, BlocksRepoName, blockerId, limit, cursor)
}

// MutesRepo persists the set of user ids the local owner has muted.
type MutesRepo struct {
	db userSetStorer
}

func NewMutesRepo(db userSetStorer) *MutesRepo { return &MutesRepo{db: db} }

func (repo *MutesRepo) Mute(muterId, muteeId string) error {
	return userSetAdd(repo.db, MutesRepoName, muterId, muteeId)
}

func (repo *MutesRepo) Unmute(muterId, muteeId string) error {
	return userSetRemove(repo.db, MutesRepoName, muterId, muteeId)
}

func (repo *MutesRepo) IsMuted(muterId, muteeId string) (bool, error) {
	return userSetHas(repo.db, MutesRepoName, muterId, muteeId)
}

func (repo *MutesRepo) List(muterId string, limit *uint64, cursor *string) ([]string, string, error) {
	return userSetList(repo.db, MutesRepoName, muterId, limit, cursor)
}

// SubscriptionsRepo persists the local "notify me about this user's new
// posts" watchlist.
type SubscriptionsRepo struct {
	db userSetStorer
}

func NewSubscriptionsRepo(db userSetStorer) *SubscriptionsRepo { return &SubscriptionsRepo{db: db} }

func (repo *SubscriptionsRepo) Subscribe(selfId, targetUserId string) error {
	return userSetAdd(repo.db, SubscriptionsRepoName, selfId, targetUserId)
}

func (repo *SubscriptionsRepo) Unsubscribe(selfId, targetUserId string) error {
	return userSetRemove(repo.db, SubscriptionsRepoName, selfId, targetUserId)
}

func (repo *SubscriptionsRepo) IsSubscribed(selfId, targetUserId string) (bool, error) {
	return userSetHas(repo.db, SubscriptionsRepoName, selfId, targetUserId)
}

// userSetAdd / userSetRemove / userSetHas / userSetList are the shared
// storage helpers — package-private so external callers go through the
// typed BlocksRepo / MutesRepo / SubscriptionsRepo APIs.
func userSetAdd(db userSetStorer, namespace, ownerId, targetUserId string) error {
	if ownerId == "" {
		return local_store.DBError("empty owner id")
	}
	if targetUserId == "" {
		return local_store.DBError("empty target user id")
	}

	forward := local_store.NewPrefixBuilder(namespace).
		AddSubPrefix("BY").
		AddRootID(ownerId).
		AddParentId(targetUserId).
		Build()
	reverse := local_store.NewPrefixBuilder(namespace).
		AddSubPrefix("OF").
		AddRootID(targetUserId).
		AddParentId(ownerId).
		Build()

	txn, err := db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err = txn.Set(forward, []byte(targetUserId)); err != nil {
		return err
	}
	if err = txn.Set(reverse, []byte(ownerId)); err != nil {
		return err
	}
	return txn.Commit()
}

func userSetRemove(db userSetStorer, namespace, ownerId, targetUserId string) error {
	if ownerId == "" {
		return local_store.DBError("empty owner id")
	}
	if targetUserId == "" {
		return local_store.DBError("empty target user id")
	}

	forward := local_store.NewPrefixBuilder(namespace).
		AddSubPrefix("BY").
		AddRootID(ownerId).
		AddParentId(targetUserId).
		Build()
	reverse := local_store.NewPrefixBuilder(namespace).
		AddSubPrefix("OF").
		AddRootID(targetUserId).
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

func userSetHas(db userSetStorer, namespace, ownerId, targetUserId string) (bool, error) {
	if ownerId == "" || targetUserId == "" {
		return false, nil
	}
	forward := local_store.NewPrefixBuilder(namespace).
		AddSubPrefix("BY").
		AddRootID(ownerId).
		AddParentId(targetUserId).
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

func userSetList(db userSetStorer, namespace, ownerId string, limit *uint64, cursor *string) ([]string, string, error) {
	if ownerId == "" {
		return nil, "", local_store.DBError("empty owner id")
	}
	prefix := local_store.NewPrefixBuilder(namespace).
		AddSubPrefix("BY").
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

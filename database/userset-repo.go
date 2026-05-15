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

// Per-user sets of user ids — same shape for block, mute. Each namespace
// stores two directed indices so both "who did I block" and "who blocked me"
// lookups are O(prefix-scan).
//
// NOT to be confused with NodeRepo.Blocklist (database/node-repo.go), which
// is a *peer*-level anti-abuse mechanism keyed by libp2p PeerID with
// escalating TTLs — that one bans misbehaving nodes from the network. The
// BlocksRepo below is the *social* block: a permanent, user-scoped "hide
// this account from me" flag keyed by domain user id, drives Mastodon's
// blockAccount UX in Tusky. Both coexist; one operates on transport, the
// other on social-graph visibility.
const (
	BlocksRepoName        = "/BLOCKS"
	MutesRepoName         = "/MUTES"
	SubscriptionsRepoName = "/SUBSCRIPTIONS" // local watchlist: which users I want notifications about
	ConvMutesRepo         = "/CONV_MUTES"    // muted conversations: user -> tweetId
	UserNotesRepoName     = "/USER_NOTES"    // private notes: (self, target) -> string note
)

type UserSetStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
}

type UserSetRepo struct {
	db        UserSetStorer
	namespace string
}

func NewBlocksRepo(db UserSetStorer) *UserSetRepo { return &UserSetRepo{db: db, namespace: BlocksRepoName} }
func NewMutesRepo(db UserSetStorer) *UserSetRepo  { return &UserSetRepo{db: db, namespace: MutesRepoName} }
func NewSubscriptionsRepo(db UserSetStorer) *UserSetRepo {
	return &UserSetRepo{db: db, namespace: SubscriptionsRepoName}
}

// Add records that ownerId added targetId to the set.
func (repo *UserSetRepo) Add(ownerId, targetId string) error {
	if ownerId == "" {
		return local_store.DBError("empty owner id")
	}
	if targetId == "" {
		return local_store.DBError("empty target id")
	}

	forward := local_store.NewPrefixBuilder(repo.namespace).
		AddSubPrefix("BY").
		AddRootID(ownerId).
		AddParentId(targetId).
		Build()
	reverse := local_store.NewPrefixBuilder(repo.namespace).
		AddSubPrefix("OF").
		AddRootID(targetId).
		AddParentId(ownerId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err = txn.Set(forward, []byte(targetId)); err != nil {
		return err
	}
	if err = txn.Set(reverse, []byte(ownerId)); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *UserSetRepo) Remove(ownerId, targetId string) error {
	if ownerId == "" {
		return local_store.DBError("empty owner id")
	}
	if targetId == "" {
		return local_store.DBError("empty target id")
	}

	forward := local_store.NewPrefixBuilder(repo.namespace).
		AddSubPrefix("BY").
		AddRootID(ownerId).
		AddParentId(targetId).
		Build()
	reverse := local_store.NewPrefixBuilder(repo.namespace).
		AddSubPrefix("OF").
		AddRootID(targetId).
		AddParentId(ownerId).
		Build()

	txn, err := repo.db.NewTxn()
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

func (repo *UserSetRepo) Has(ownerId, targetId string) (bool, error) {
	if ownerId == "" || targetId == "" {
		return false, nil
	}
	forward := local_store.NewPrefixBuilder(repo.namespace).
		AddSubPrefix("BY").
		AddRootID(ownerId).
		AddParentId(targetId).
		Build()

	txn, err := repo.db.NewTxn()
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

// List returns the set of user ids the owner added to this namespace.
func (repo *UserSetRepo) List(ownerId string, limit *uint64, cursor *string) ([]string, string, error) {
	if ownerId == "" {
		return nil, "", local_store.DBError("empty owner id")
	}
	prefix := local_store.NewPrefixBuilder(repo.namespace).
		AddSubPrefix("BY").
		AddRootID(ownerId).
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

// UserNoteRepo persists per-target private notes (Mastodon's
// "Edit profile note about <user>"). Stored locally only, never
// surfaced to the target or any peer.
type UserNoteRepo struct {
	db UserSetStorer
}

func NewUserNoteRepo(db UserSetStorer) *UserNoteRepo {
	return &UserNoteRepo{db: db}
}

func (repo *UserNoteRepo) SetNote(selfId, targetId, note string) error {
	if selfId == "" {
		return local_store.DBError("empty self id")
	}
	if targetId == "" {
		return local_store.DBError("empty target id")
	}
	key := local_store.NewPrefixBuilder(UserNotesRepoName).
		AddRootID(selfId).
		AddParentId(targetId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	// Empty note clears the entry — kept as an explicit delete so List
	// scans don't trip on stale blank values.
	if note == "" {
		if err := txn.Delete(key); err != nil && !local_store.IsNotFoundError(err) {
			return err
		}
		return txn.Commit()
	}
	if err := txn.Set(key, []byte(note)); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *UserNoteRepo) GetNote(selfId, targetId string) (string, error) {
	if selfId == "" || targetId == "" {
		return "", nil
	}
	key := local_store.NewPrefixBuilder(UserNotesRepoName).
		AddRootID(selfId).
		AddParentId(targetId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return "", err
	}
	defer txn.Rollback()

	bt, err := txn.Get(key)
	if local_store.IsNotFoundError(err) {
		return "", txn.Commit()
	}
	if err != nil {
		return "", err
	}
	if err := txn.Commit(); err != nil {
		return "", err
	}
	return string(bt), nil
}

// ConvMuteRepo persists muted conversations (user -> tweet id set).
type ConvMuteRepo struct {
	db UserSetStorer
}

func NewConvMutesRepo(db UserSetStorer) *ConvMuteRepo { return &ConvMuteRepo{db: db} }

func (repo *ConvMuteRepo) Mute(userId, tweetId string) error {
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	if tweetId == "" {
		return local_store.DBError("empty tweet id")
	}
	key := local_store.NewPrefixBuilder(ConvMutesRepo).
		AddRootID(userId).
		AddParentId(tweetId).
		Build()
	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()
	if err = txn.Set(key, []byte(tweetId)); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *ConvMuteRepo) Unmute(userId, tweetId string) error {
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	if tweetId == "" {
		return local_store.DBError("empty tweet id")
	}
	key := local_store.NewPrefixBuilder(ConvMutesRepo).
		AddRootID(userId).
		AddParentId(tweetId).
		Build()
	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()
	if err = txn.Delete(key); err != nil && !local_store.IsNotFoundError(err) {
		return err
	}
	return txn.Commit()
}

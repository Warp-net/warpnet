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
	"time"

	"github.com/Warp-net/warpnet/database/local-store"
)

const (
	ConversationsRepoName = "/CONVERSATIONS"
	convoListNS           = "LIST"
	convoItemNS           = "ITEM"
)

// ConversationsRepo records which root tweets a user has participated in
// (either as author of the root or as a replier on a thread they touched).
// Stored locally per user — never replicated. Tusky's "Conversations" tab
// reads it.
type ConversationsRepo struct {
	db UserSetStorer
}

func NewConversationsRepo(db UserSetStorer) *ConversationsRepo {
	return &ConversationsRepo{db: db}
}

// Touch records that userId saw a (potentially new) conversation rooted at
// rootTweetId at time `at`. Idempotent: the list key carries the most
// recent timestamp, the item key tracks the latest list-key so older entries
// can be cleaned up when the same conversation is updated.
func (repo *ConversationsRepo) Touch(userId, rootTweetId string, at time.Time) error {
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	if rootTweetId == "" {
		return local_store.DBError("empty tweet id")
	}
	if at.IsZero() {
		at = time.Now()
	}

	listKey := local_store.NewPrefixBuilder(ConversationsRepoName).
		AddSubPrefix(convoListNS).
		AddRootID(userId).
		AddReversedTimestamp(at).
		AddParentId(rootTweetId).
		Build()

	itemKey := local_store.NewPrefixBuilder(ConversationsRepoName).
		AddSubPrefix(convoItemNS).
		AddRootID(userId).
		AddParentId(rootTweetId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	// Delete the previous list-entry (if any) so the user doesn't see the
	// same conversation at two timestamps.
	if prev, err := txn.Get(itemKey); err == nil && len(prev) != 0 {
		if err := txn.Delete(local_store.DatabaseKey(prev)); err != nil && !local_store.IsNotFoundError(err) {
			return err
		}
	}
	if err := txn.Set(itemKey, []byte(listKey)); err != nil {
		return err
	}
	if err := txn.Set(listKey, []byte(rootTweetId)); err != nil {
		return err
	}
	return txn.Commit()
}

// Hide drops the conversation entry from the user's list. Does NOT touch
// the underlying tweets — only the local visibility flag.
func (repo *ConversationsRepo) Hide(userId, rootTweetId string) error {
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	if rootTweetId == "" {
		return local_store.DBError("empty tweet id")
	}
	itemKey := local_store.NewPrefixBuilder(ConversationsRepoName).
		AddSubPrefix(convoItemNS).
		AddRootID(userId).
		AddParentId(rootTweetId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	listKeyBytes, err := txn.Get(itemKey)
	if local_store.IsNotFoundError(err) {
		return txn.Commit()
	}
	if err != nil {
		return err
	}
	if err := txn.Delete(itemKey); err != nil {
		return err
	}
	if len(listKeyBytes) != 0 {
		if err := txn.Delete(local_store.DatabaseKey(listKeyBytes)); err != nil && !local_store.IsNotFoundError(err) {
			return err
		}
	}
	return txn.Commit()
}

// List returns the user's conversation root tweet IDs, newest first.
func (repo *ConversationsRepo) List(userId string, limit *uint64, cursor *string) ([]string, string, error) {
	if userId == "" {
		return nil, "", local_store.DBError("empty user id")
	}
	prefix := local_store.NewPrefixBuilder(ConversationsRepoName).
		AddSubPrefix(convoListNS).
		AddRootID(userId).
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
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, string(item.Value))
	}
	return out, cur, nil
}

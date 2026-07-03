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
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
)

const BookmarkRepoName = "/BOOKMARKS"

type BookmarkStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
}

type BookmarkRepo struct {
	db BookmarkStorer
}

func NewBookmarkRepo(db BookmarkStorer) *BookmarkRepo {
	return &BookmarkRepo{db: db}
}

func (repo *BookmarkRepo) Bookmark(userId, tweetId, ownerUserId string) error {
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	if tweetId == "" {
		return local_store.DBError("empty tweet id")
	}
	if ownerUserId == "" {
		return local_store.DBError("empty owner user id")
	}

	bm := domain.Bookmark{
		UserId:      userId,
		TweetId:     tweetId,
		OwnerUserId: ownerUserId,
		CreatedAt:   time.Now(),
	}

	// Same fixed/sortable key pair as the chat message repo: the fixed key
	// gives deterministic lookup for unbookmark and is skipped by
	// iteration, the sortable key orders the list newest-first.
	fixedKey := local_store.NewPrefixBuilder(BookmarkRepoName).
		AddRootID(userId).
		AddRange(local_store.FixedRangeKey).
		AddParentId(tweetId).
		Build()

	sortableKey := local_store.NewPrefixBuilder(BookmarkRepoName).
		AddRootID(userId).
		AddReversedTimestamp(bm.CreatedAt).
		AddParentId(tweetId).
		Build()

	bt, err := json.Marshal(bm)
	if err != nil {
		return err
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if existing, err := txn.Get(fixedKey); err == nil && len(existing) != 0 {
		return txn.Commit() // already bookmarked, no-op
	}
	if err = txn.Set(fixedKey, sortableKey.Bytes()); err != nil {
		return err
	}
	if err = txn.Set(sortableKey, bt); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *BookmarkRepo) Unbookmark(userId, tweetId string) error {
	if userId == "" {
		return local_store.DBError("empty user id")
	}
	if tweetId == "" {
		return local_store.DBError("empty tweet id")
	}

	fixedKey := local_store.NewPrefixBuilder(BookmarkRepoName).
		AddRootID(userId).
		AddRange(local_store.FixedRangeKey).
		AddParentId(tweetId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	sortableKey, err := txn.Get(fixedKey)
	if err != nil && !local_store.IsNotFoundError(err) {
		return err
	}
	if len(sortableKey) == 0 {
		return txn.Commit() // not bookmarked, no-op
	}
	if err = txn.Delete(fixedKey); err != nil {
		return err
	}
	if err = txn.Delete(local_store.DatabaseKey(sortableKey)); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *BookmarkRepo) List(userId string, limit *uint64, cursor *string) ([]domain.Bookmark, string, error) {
	if userId == "" {
		return nil, "", local_store.DBError("empty user id")
	}

	prefix := local_store.NewPrefixBuilder(BookmarkRepoName).
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
	if err = txn.Commit(); err != nil {
		return nil, "", err
	}

	bms := make([]domain.Bookmark, 0, len(items))
	for _, item := range items {
		var bm domain.Bookmark
		if err := json.Unmarshal(item.Value, &bm); err != nil {
			return nil, "", err
		}
		bms = append(bms, bm)
	}
	return bms, cur, nil
}

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
	"github.com/google/uuid"
)

const TweetEditsRepoName = "/TWEET_EDITS"

type TweetEditStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
}

type TweetEditsRepo struct {
	db TweetEditStorer
}

func NewTweetEditsRepo(db TweetEditStorer) *TweetEditsRepo {
	return &TweetEditsRepo{db: db}
}

// Append records a new edit revision for the given tweet. Edits are
// append-only — never updated, never deleted (except via the tweet's
// own delete handler).
func (repo *TweetEditsRepo) Append(edit domain.TweetEdit) (domain.TweetEdit, error) {
	if edit.OriginalTweetId == "" {
		return domain.TweetEdit{}, local_store.DBError("empty tweet id")
	}
	if edit.UserId == "" {
		return domain.TweetEdit{}, local_store.DBError("empty user id")
	}
	if edit.Text == "" {
		return domain.TweetEdit{}, local_store.DBError("empty text")
	}
	if edit.Id == "" {
		edit.Id = uuid.New().String()
	}
	if edit.EditedAt.IsZero() {
		edit.EditedAt = time.Now()
	}

	key := local_store.NewPrefixBuilder(TweetEditsRepoName).
		AddRootID(edit.OriginalTweetId).
		AddReversedTimestamp(edit.EditedAt).
		AddParentId(edit.Id).
		Build()

	bt, err := json.Marshal(edit)
	if err != nil {
		return domain.TweetEdit{}, err
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return domain.TweetEdit{}, err
	}
	defer txn.Rollback()

	if err := txn.Set(key, bt); err != nil {
		return domain.TweetEdit{}, err
	}
	if err := txn.Commit(); err != nil {
		return domain.TweetEdit{}, err
	}
	return edit, nil
}

// List returns the edit history for a tweet, newest first.
func (repo *TweetEditsRepo) List(tweetId string, limit *uint64, cursor *string) ([]domain.TweetEdit, string, error) {
	if tweetId == "" {
		return nil, "", local_store.DBError("empty tweet id")
	}
	prefix := local_store.NewPrefixBuilder(TweetEditsRepoName).
		AddRootID(tweetId).
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

	edits := make([]domain.TweetEdit, 0, len(items))
	for _, item := range items {
		var e domain.TweetEdit
		if err := json.Unmarshal(item.Value, &e); err != nil {
			return nil, "", err
		}
		edits = append(edits, e)
	}
	return edits, cur, nil
}

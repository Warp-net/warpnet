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
	"fmt"
	"sort"
	"time"

	"github.com/Warp-net/warpnet/domain"

	"github.com/Warp-net/warpnet/database/local"
	"github.com/Warp-net/warpnet/json"
)

const TimelineRepoName = "/TIMELINE"

type TimelineStorer interface {
	Set(key local.DatabaseKey, value []byte) error
	NewTxn() (local.WarpTransactioner, error)
	Delete(key local.DatabaseKey) error
}

type TimelineRepo struct {
	db TimelineStorer
}

func NewTimelineRepo(db TimelineStorer) *TimelineRepo {
	return &TimelineRepo{db: db}
}

func (repo *TimelineRepo) AddTweetToTimeline(userId string, tweet domain.Tweet) error {
	if userId == "" {
		return local.DBError("userID cannot be blank")
	}
	if tweet.Id == "" {
		return local.DBError("tweet id should not be empty")
	}
	if tweet.CreatedAt.IsZero() {
		tweet.CreatedAt = time.Now()
	}

	fixedKey := local.NewPrefixBuilder(TimelineRepoName).
		AddRootID(userId).
		AddRange(local.FixedRangeKey).
		AddParentId(tweet.Id).
		Build()

	sortableKey := local.NewPrefixBuilder(TimelineRepoName).
		AddRootID(userId).
		AddReversedTimestamp(tweet.CreatedAt).
		AddParentId(tweet.Id).
		Build()

	data, err := json.Marshal(tweet)
	if err != nil {
		return fmt.Errorf("timeline marshal: %w", err)
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return fmt.Errorf("creating transaction: %w", err)
	}
	defer txn.Rollback()

	if err := txn.Set(fixedKey, sortableKey.Bytes()); err != nil {
		return fmt.Errorf("adding timeline sortable key: %w", err)
	}
	if err := txn.Set(sortableKey, data); err != nil {
		return fmt.Errorf("adding timeline data: %w", err)
	}
	return txn.Commit()
}

func (repo *TimelineRepo) DeleteTweetFromTimeline(userID, tweetID string) error {
	if userID == "" {
		return local.DBError("user ID cannot be blank")
	}

	fixedKey := local.NewPrefixBuilder(TimelineRepoName).
		AddRootID(userID).
		AddRange(local.FixedRangeKey).
		AddParentId(tweetID).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return fmt.Errorf("creating transaction: %w", err)
	}
	defer txn.Rollback()

	sortableKeyBytes, err := txn.Get(fixedKey)
	if local.IsNotFoundError(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := txn.Delete(fixedKey); err != nil {
		return err
	}
	if err := txn.Delete(local.DatabaseKey(sortableKeyBytes)); err != nil {
		return err
	}
	return txn.Commit()
}

// GetTimeline retrieves a user's timeline sorted from newest to oldest
func (repo *TimelineRepo) GetTimeline(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
	if userId == "" {
		return nil, "", local.DBError("user ID cannot be blank")
	}

	prefix := local.NewPrefixBuilder(TimelineRepoName).AddRootID(userId).Build()

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

	tweets := make([]domain.Tweet, 0, len(items))
	for _, item := range items {
		var t domain.Tweet
		err = json.Unmarshal(item.Value, &t)
		if err != nil {
			return nil, "", err
		}
		tweets = append(tweets, t)
	}
	sort.SliceStable(tweets, func(i, j int) bool {
		return tweets[i].CreatedAt.After(tweets[j].CreatedAt)
	})

	return tweets, cur, nil
}

/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU General Public License for more details.

 You should have received a copy of the GNU General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

package database

import (
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/domain"
	"sort"
	"time"

	"github.com/Warp-net/warpnet/database/storage"
	"github.com/Warp-net/warpnet/json"
)

const TimelineRepoName = "/TIMELINE"

type TimelineStorer interface {
	Set(key storage.DatabaseKey, value []byte) error
	NewTxn() (storage.WarpTransactioner, error)
	Delete(key storage.DatabaseKey) error
}

type TimelineRepo struct {
	db TimelineStorer
}

func NewTimelineRepo(db TimelineStorer) *TimelineRepo {
	return &TimelineRepo{db: db}
}

func (repo *TimelineRepo) AddTweetToTimeline(userId string, tweet domain.Tweet) error {
	if userId == "" {
		return errors.New("userID cannot be blank")
	}
	if tweet.Id == "" {
		return fmt.Errorf("tweet id should not be nil")
	}
	if tweet.CreatedAt.IsZero() {
		return fmt.Errorf("tweet created at should not be zero")
	}

	key := storage.NewPrefixBuilder(TimelineRepoName).
		AddRootID(userId).
		AddReversedTimestamp(tweet.CreatedAt).
		AddParentId(tweet.Id).
		Build()

	data, err := json.JSON.Marshal(tweet)
	if err != nil {
		return fmt.Errorf("timeline marshal: %w", err)
	}
	return repo.db.Set(key, data)
}

func (repo *TimelineRepo) DeleteTweetFromTimeline(userID, tweetID string, createdAt time.Time) error {
	if userID == "" {
		return errors.New("user ID cannot be blank")
	}
	if createdAt.IsZero() {
		return fmt.Errorf("created time should not be zero")
	}
	key := storage.NewPrefixBuilder(TimelineRepoName).
		AddRootID(userID).
		AddReversedTimestamp(createdAt).
		AddParentId(tweetID).
		Build()
	return repo.db.Delete(key)
}

// GetTimeline retrieves a user's timeline sorted from newest to oldest
func (repo *TimelineRepo) GetTimeline(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
	if userId == "" {
		return nil, "", errors.New("user ID cannot be blank")
	}

	prefix := storage.NewPrefixBuilder(TimelineRepoName).AddRootID(userId).Build()

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
		err = json.JSON.Unmarshal(item.Value, &t)
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

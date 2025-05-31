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
	"encoding/binary"
	"errors"
	"github.com/Warp-net/warpnet/database/storage"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"time"
)

var ErrAlreadyFollowed = errors.New("already followed")

const (
	FollowRepoName       = "/FOLLOWINGS"
	followeeSubName      = "FOLLOWEE"
	followerSubName      = "FOLLOWER"
	followeeCountSubName = "FOLLOWEECOUNT"
	followerCountSubName = "FOLLOWERCOUNT"
)

type FollowerStorer interface {
	NewTxn() (storage.WarpTransactioner, error)
	Set(key storage.DatabaseKey, value []byte) error
	Get(key storage.DatabaseKey) ([]byte, error)
	Delete(key storage.DatabaseKey) error
}

// FollowRepo handles reader/writer relationships
type FollowRepo struct {
	db FollowerStorer
}

func NewFollowRepo(db FollowerStorer) *FollowRepo {
	return &FollowRepo{db: db}
}

func (repo *FollowRepo) Follow(fromUserId, toUserId string, event domain.Following) error {
	if fromUserId == "" || toUserId == "" {
		return errors.New("invalid follow params")
	}

	data, _ := json.JSON.Marshal(event)

	fixedFolloweeKey := storage.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followeeSubName).
		AddRootID(toUserId).
		AddRange(storage.FixedRangeKey).
		AddParentId(fromUserId).
		Build()

	fixedFollowerKey := storage.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followerSubName).
		AddRootID(fromUserId).
		AddRange(storage.FixedRangeKey).
		AddParentId(toUserId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	_, err = txn.Get(fixedFollowerKey)
	if err == nil {
		return ErrAlreadyFollowed
	}

	sortableFolloweeKey := storage.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followeeSubName).
		AddRootID(toUserId).
		AddReversedTimestamp(time.Now()).
		AddParentId(fromUserId).
		Build()

	sortableFollowerKey := storage.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followerSubName).
		AddRootID(fromUserId).
		AddReversedTimestamp(time.Now()).
		AddParentId(toUserId).
		Build()

	followeesCountKey := storage.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followeeCountSubName).
		AddRootID(toUserId).
		Build()

	followersCountKey := storage.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followerCountSubName).
		AddRootID(fromUserId).
		Build()

	if err := txn.Set(sortableFollowerKey, data); err != nil {
		return err
	}
	if err := txn.Set(sortableFolloweeKey, data); err != nil {
		return err
	}
	if err := txn.Set(fixedFollowerKey, []byte(sortableFollowerKey)); err != nil {
		return err
	}
	if err := txn.Set(fixedFolloweeKey, []byte(sortableFolloweeKey)); err != nil {
		return err
	}
	if _, err := txn.Increment(followersCountKey); err != nil {
		return err
	}
	if _, err := txn.Increment(followeesCountKey); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *FollowRepo) Unfollow(fromUserId, toUserId string) error {
	fixedFolloweeKey := storage.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followeeSubName).
		AddRootID(toUserId).
		AddRange(storage.FixedRangeKey).
		AddParentId(fromUserId).
		Build()

	fixedFollowerKey := storage.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followerSubName).
		AddRootID(fromUserId).
		AddRange(storage.FixedRangeKey).
		AddParentId(toUserId).
		Build()

	followeesCountKey := storage.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followeeCountSubName).
		AddRootID(toUserId).
		Build()

	followersCountKey := storage.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followerCountSubName).
		AddRootID(fromUserId).
		Build()

	sortableFolloweeKey, err := repo.db.Get(fixedFolloweeKey)
	if err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return err
	}
	sortableFollowerKey, err := repo.db.Get(fixedFollowerKey)
	if err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return err
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err := txn.Delete(fixedFolloweeKey); err != nil {
		return err
	}
	if err := txn.Delete(storage.DatabaseKey(sortableFolloweeKey)); err != nil {
		return err
	}
	if err := txn.Delete(fixedFollowerKey); err != nil {
		return err
	}
	if err := txn.Delete(storage.DatabaseKey(sortableFollowerKey)); err != nil {
		return err
	}
	if _, err := txn.Decrement(followeesCountKey); err != nil {
		return err
	}
	if _, err := txn.Decrement(followersCountKey); err != nil {
		return err
	}

	return txn.Commit()
}

func (repo *FollowRepo) GetFollowersCount(userId string) (uint64, error) {
	if userId == "" {
		return 0, errors.New("followers count: empty userID")
	}
	followersCountKey := storage.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followerCountSubName).
		AddRootID(userId).
		Build()
	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()
	bt, err := txn.Get(followersCountKey)
	if errors.Is(err, storage.ErrKeyNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count := binary.BigEndian.Uint64(bt)
	return count, txn.Commit()
}

func (repo *FollowRepo) GetFolloweesCount(userId string) (uint64, error) {
	if userId == "" {
		return 0, errors.New("followers count: empty userID")
	}
	followeesCountKey := storage.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followeeCountSubName).
		AddRootID(userId).
		Build()
	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()
	bt, err := txn.Get(followeesCountKey)
	if errors.Is(err, storage.ErrKeyNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count := binary.BigEndian.Uint64(bt)
	return count, txn.Commit()
}

func (repo *FollowRepo) GetFollowers(userId string, limit *uint64, cursor *string) ([]domain.Following, string, error) {
	followeePrefix := storage.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followeeSubName).
		AddRootID(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	items, cur, err := txn.List(followeePrefix, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	if err := txn.Commit(); err != nil {
		return nil, "", err
	}

	followings := make([]domain.Following, 0, len(items))
	for _, item := range items {
		var f domain.Following
		err = json.JSON.Unmarshal(item.Value, &f)
		if err != nil {
			return nil, "", err
		}
		followings = append(followings, f)
	}

	return followings, cur, nil
}

// GetFollowees followee - one who is followed (has his/her posts monitored by another user)
func (repo *FollowRepo) GetFollowees(userId string, limit *uint64, cursor *string) ([]domain.Following, string, error) {
	followerPrefix := storage.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followerSubName).
		AddRootID(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	items, cur, err := txn.List(followerPrefix, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	if err := txn.Commit(); err != nil {
		return nil, "", err
	}

	followings := make([]domain.Following, 0, len(items))
	for _, item := range items {
		var f domain.Following
		err = json.JSON.Unmarshal(item.Value, &f)
		if err != nil {
			return nil, "", err
		}
		followings = append(followings, f)
	}

	return followings, cur, nil
}

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
	"encoding/binary"
	"strings"
	"time"

	"github.com/Warp-net/warpnet/database/local"
)

var ErrAlreadyFollowed = local.DBError("already followed")

const (
	FollowRepoName        = "/FOLLOW"
	followingSubName      = "FOLLOWING"
	followerSubName       = "FOLLOWER"
	followingCountSubName = "FOLLOWINGCOUNT"
	followerCountSubName  = "FOLLOWERCOUNT"
)

type FollowerStorer interface {
	NewTxn() (local.WarpTransactioner, error)
	Set(key local.DatabaseKey, value []byte) error
	Get(key local.DatabaseKey) ([]byte, error)
	Delete(key local.DatabaseKey) error
}

type FollowRepo struct {
	db FollowerStorer
}

func NewFollowRepo(db FollowerStorer) *FollowRepo {
	return &FollowRepo{db: db}
}

func (repo *FollowRepo) Follow(fromUserId, toUserId string) error {
	if fromUserId == "" || toUserId == "" {
		return local.DBError("invalid follow params")
	}
	if fromUserId == toUserId {
		return local.DBError("cannot follow yourself")
	}

	fixedFollowingKey := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followingSubName).
		AddRootID(toUserId).
		AddRange(local.FixedRangeKey).
		AddParentId(fromUserId).
		Build()

	fixedFollowerKey := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followerSubName).
		AddRootID(fromUserId).
		AddRange(local.FixedRangeKey).
		AddParentId(toUserId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	_, err = txn.Get(fixedFollowingKey)
	if err == nil {
		return ErrAlreadyFollowed
	}

	sortableFollowingKey := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followingSubName).
		AddRootID(toUserId).
		AddReversedTimestamp(time.Now()).
		AddParentId(fromUserId).
		Build()

	sortableFollowerKey := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followerSubName).
		AddRootID(fromUserId).
		AddReversedTimestamp(time.Now()).
		AddParentId(toUserId).
		Build()

	followingsCountKey := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followingCountSubName).
		AddRootID(fromUserId).
		Build()

	followersCountKey := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followerCountSubName).
		AddRootID(toUserId).
		Build()

	if err := txn.Set(sortableFollowerKey, []byte{}); err != nil {
		return err
	}
	if err := txn.Set(sortableFollowingKey, []byte{}); err != nil {
		return err
	}
	if err := txn.Set(fixedFollowerKey, []byte(sortableFollowerKey)); err != nil {
		return err
	}
	if err := txn.Set(fixedFollowingKey, []byte(sortableFollowingKey)); err != nil {
		return err
	}
	if _, err := txn.Increment(followersCountKey); err != nil {
		return err
	}
	if _, err := txn.Increment(followingsCountKey); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *FollowRepo) IsFollowing(ownerId, otherUserId string) bool {
	key := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followingSubName).
		AddRootID(otherUserId).
		AddRange(local.FixedRangeKey).
		AddParentId(ownerId).
		Build()

	_, err := repo.db.Get(key)
	if local.IsNotFoundError(err) {
		return false
	}
	if err != nil {
		return false
	}
	return true
}

func (repo *FollowRepo) IsFollower(ownerId, otherUserId string) bool {
	key := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followerSubName).
		AddRootID(otherUserId). // follower
		AddRange(local.FixedRangeKey).
		AddParentId(ownerId). // followee
		Build()

	_, err := repo.db.Get(key)
	if local.IsNotFoundError(err) {
		return false
	}
	if err != nil {
		return false
	}
	return true
}

func (repo *FollowRepo) Unfollow(fromUserId, toUserId string) error {
	fixedFollowingKey := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followingSubName).
		AddRootID(toUserId).
		AddRange(local.FixedRangeKey).
		AddParentId(fromUserId).
		Build()

	fixedFollowerKey := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followerSubName).
		AddRootID(fromUserId).
		AddRange(local.FixedRangeKey).
		AddParentId(toUserId).
		Build()

	followingsCountKey := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followingCountSubName).
		AddRootID(fromUserId).
		Build()

	followersCountKey := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followerCountSubName).
		AddRootID(toUserId).
		Build()

	sortableFollowingKey, err := repo.db.Get(fixedFollowingKey)
	if err != nil && !local.IsNotFoundError(err) {
		return err
	}
	sortableFollowerKey, err := repo.db.Get(fixedFollowerKey)
	if err != nil && !local.IsNotFoundError(err) {
		return err
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err := txn.Delete(fixedFollowingKey); err != nil {
		return err
	}
	if err := txn.Delete(local.DatabaseKey(sortableFollowingKey)); err != nil {
		return err
	}
	if err := txn.Delete(fixedFollowerKey); err != nil {
		return err
	}
	if err := txn.Delete(local.DatabaseKey(sortableFollowerKey)); err != nil {
		return err
	}
	if _, err := txn.Decrement(followingsCountKey); err != nil {
		return err
	}
	if _, err := txn.Decrement(followersCountKey); err != nil {
		return err
	}

	return txn.Commit()
}

func (repo *FollowRepo) GetFollowersCount(userId string) (uint64, error) {
	if userId == "" {
		return 0, local.DBError("followers count: empty userID")
	}
	followersCountKey := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followerCountSubName).
		AddRootID(userId).
		Build()
	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()
	bt, err := txn.Get(followersCountKey)
	if local.IsNotFoundError(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count := binary.BigEndian.Uint64(bt)
	return count, txn.Commit()
}

func (repo *FollowRepo) GetFollowingsCount(userId string) (uint64, error) {
	if userId == "" {
		return 0, local.DBError("followings count: empty userID")
	}
	followingsCountKey := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followingCountSubName).
		AddRootID(userId).
		Build()
	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()
	bt, err := txn.Get(followingsCountKey)
	if local.IsNotFoundError(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count := binary.BigEndian.Uint64(bt)
	return count, txn.Commit()
}

func (repo *FollowRepo) GetFollowers(userId string, limit *uint64, cursor *string) ([]string, string, error) {
	followingPrefix := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followingSubName).
		AddRootID(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	keys, cur, err := txn.ListKeys(followingPrefix, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	if err := txn.Commit(); err != nil {
		return nil, "", err
	}

	followers := make([]string, 0, len(keys))
	for _, key := range keys {
		splitted := strings.Split(key, local.Delimeter)
		if len(splitted) < 2 {
			continue
		}
		// taking a last part of a key
		followers = append(followers, splitted[len(splitted)-1])
	}

	return followers, cur, nil
}

// GetFollowings following - one who is followed (has his/her posts monitored by another user)
func (repo *FollowRepo) GetFollowings(userId string, limit *uint64, cursor *string) ([]string, string, error) {
	followerPrefix := local.NewPrefixBuilder(FollowRepoName).
		AddSubPrefix(followerSubName).
		AddRootID(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	keys, cur, err := txn.ListKeys(followerPrefix, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	if err := txn.Commit(); err != nil {
		return nil, "", err
	}

	followings := make([]string, 0, len(keys))
	for _, key := range keys {
		splitted := strings.Split(key, local.Delimeter)
		if len(splitted) < 2 {
			continue
		}
		// taking a last part of a key
		followings = append(followings, splitted[len(splitted)-1])
	}

	return followings, cur, nil
}

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

 WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
 Use at your own risk. The maintainers shall not be liable for any damages or data loss
 resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package database

import (
	"github.com/Warp-net/warpnet/core/crdt"
	"github.com/Warp-net/warpnet/database/local"
)

// CRDTLikeRepo manages likes with CRDT-based statistics
type CRDTLikeRepo struct {
	db        LikeStorer
	crdtStore *crdt.CRDTStatsStore
}

// NewCRDTLikeRepo creates a new CRDT-enabled like repository
func NewCRDTLikeRepo(db LikeStorer, crdtStore *crdt.CRDTStatsStore) *CRDTLikeRepo {
	return &CRDTLikeRepo{
		db:        db,
		crdtStore: crdtStore,
	}
}

// Like records a like and updates CRDT counter
func (repo *CRDTLikeRepo) Like(tweetId, userId string) (likesCount uint64, err error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}
	if userId == "" {
		return 0, local.DBError("empty user id")
	}

	// Store the user-like relationship locally (who liked what)
	likerKey := local.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(LikerSubNamespace).
		AddRootID(tweetId).
		AddRange(local.NoneRangeKey).
		AddParentId(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()

	// Check if already liked
	_, err = txn.Get(likerKey)
	if !local.IsNotFoundError(err) {
		_ = txn.Commit()
		// Already liked, return current count from CRDT
		return repo.LikesCount(tweetId)
	}

	// Store the like relationship
	if err = txn.Set(likerKey, []byte(userId)); err != nil {
		return 0, err
	}
	
	if err = txn.Commit(); err != nil {
		return 0, err
	}

	// Update CRDT counter (increment for this node)
	if repo.crdtStore != nil {
		current, _ := repo.crdtStore.Get(tweetId, crdt.StatTypeLikes)
		newValue := current + 1
		if err := repo.crdtStore.Put(tweetId, crdt.StatTypeLikes, newValue); err != nil {
			return 0, err
		}
	}

	return repo.LikesCount(tweetId)
}

// Unlike removes a like and updates CRDT counter
func (repo *CRDTLikeRepo) Unlike(tweetId, userId string) (likesCount uint64, err error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}
	if userId == "" {
		return 0, local.DBError("empty user id")
	}

	likerKey := local.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(LikerSubNamespace).
		AddRootID(tweetId).
		AddRange(local.NoneRangeKey).
		AddParentId(userId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return 0, err
	}
	defer txn.Rollback()

	// Check if like exists
	_, err = txn.Get(likerKey)
	if local.IsNotFoundError(err) {
		_ = txn.Commit()
		// Already unliked, return current count
		return repo.LikesCount(tweetId)
	}

	// Remove the like relationship
	if err = txn.Delete(likerKey); err != nil {
		return 0, err
	}
	
	if err = txn.Commit(); err != nil {
		return 0, err
	}

	// Update CRDT counter (decrement for this node)
	if repo.crdtStore != nil {
		current, _ := repo.crdtStore.Get(tweetId, crdt.StatTypeLikes)
		if current > 0 {
			newValue := current - 1
			if err := repo.crdtStore.Put(tweetId, crdt.StatTypeLikes, newValue); err != nil {
				return 0, err
			}
		}
	}

	return repo.LikesCount(tweetId)
}

// LikesCount returns the aggregated like count from CRDT
func (repo *CRDTLikeRepo) LikesCount(tweetId string) (likesNum uint64, err error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}

	if repo.crdtStore != nil {
		return repo.crdtStore.GetAggregatedStat(tweetId, crdt.StatTypeLikes)
	}

	return 0, nil
}

// Likers returns the list of users who liked a tweet
func (repo *CRDTLikeRepo) Likers(tweetId string, limit *uint64, cursor *string) (_ []string, cur string, err error) {
	if tweetId == "" {
		return nil, "", local.DBError("empty tweet id")
	}

	likePrefix := local.NewPrefixBuilder(LikeRepoName).
		AddSubPrefix(LikerSubNamespace).
		AddRootID(tweetId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	items, cur, err := txn.List(likePrefix, limit, cursor)
	if local.IsNotFoundError(err) {
		return nil, "", ErrLikesNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if err = txn.Commit(); err != nil {
		return nil, "", err
	}

	likers := make([]string, 0, len(items))
	for _, item := range items {
		userId := string(item.Value)
		likers = append(likers, userId)
	}
	return likers, cur, nil
}

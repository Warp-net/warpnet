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
	log "github.com/sirupsen/logrus"
)

// CRDTLikeRepo wraps LikeRepo with CRDT-based statistics
type CRDTLikeRepo struct {
	*LikeRepo
	crdtStore *crdt.CRDTStatsStore
}

// NewCRDTLikeRepo creates a new CRDT-enabled like repository
func NewCRDTLikeRepo(db LikeStorer, crdtStore *crdt.CRDTStatsStore) *CRDTLikeRepo {
	return &CRDTLikeRepo{
		LikeRepo:  NewLikeRepo(db),
		crdtStore: crdtStore,
	}
}

// Like increments the like count using both local storage and CRDT
func (repo *CRDTLikeRepo) Like(tweetId, userId string) (likesCount uint64, err error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}
	if userId == "" {
		return 0, local.DBError("empty user id")
	}

	// Store like locally first
	_, err = repo.LikeRepo.Like(tweetId, userId)
	if err != nil {
		return 0, err
	}

	// Update CRDT counter
	if repo.crdtStore != nil {
		count, err := repo.crdtStore.IncrementStat(tweetId, crdt.StatTypeLikes)
		if err != nil {
			log.Warnf("failed to increment CRDT like count: %v", err)
			// Don't fail the operation if CRDT update fails
			return repo.LikeRepo.LikesCount(tweetId)
		}
		return count, nil
	}

	return repo.LikeRepo.LikesCount(tweetId)
}

// Unlike decrements the like count using both local storage and CRDT
func (repo *CRDTLikeRepo) Unlike(tweetId, userId string) (likesCount uint64, err error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}
	if userId == "" {
		return 0, local.DBError("empty user id")
	}

	// Remove like locally first
	_, err = repo.LikeRepo.Unlike(tweetId, userId)
	if err != nil {
		return 0, err
	}

	// Update CRDT counter
	if repo.crdtStore != nil {
		count, err := repo.crdtStore.DecrementStat(tweetId, crdt.StatTypeLikes)
		if err != nil {
			log.Warnf("failed to decrement CRDT like count: %v", err)
			// Don't fail the operation if CRDT update fails
			return repo.LikeRepo.LikesCount(tweetId)
		}
		return count, nil
	}

	return repo.LikeRepo.LikesCount(tweetId)
}

// LikesCount returns the aggregated like count from CRDT if available, otherwise from local storage
func (repo *CRDTLikeRepo) LikesCount(tweetId string) (likesNum uint64, err error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}

	// Try to get aggregated count from CRDT first
	if repo.crdtStore != nil {
		count, err := repo.crdtStore.GetAggregatedStat(tweetId, crdt.StatTypeLikes)
		if err != nil {
			log.Warnf("failed to get CRDT like count, falling back to local: %v", err)
			return repo.LikeRepo.LikesCount(tweetId)
		}
		return count, nil
	}

	// Fallback to local count
	return repo.LikeRepo.LikesCount(tweetId)
}

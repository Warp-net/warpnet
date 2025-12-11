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
	"github.com/Warp-net/warpnet/domain"
	log "github.com/sirupsen/logrus"
)

// CRDTReplyRepo wraps ReplyRepo with CRDT-based statistics
type CRDTReplyRepo struct {
	*ReplyRepo
	crdtStore *crdt.CRDTStatsStore
}

// NewCRDTReplyRepo creates a new CRDT-enabled reply repository
func NewCRDTReplyRepo(db ReplyStorer, crdtStore *crdt.CRDTStatsStore) *CRDTReplyRepo {
	return &CRDTReplyRepo{
		ReplyRepo: NewRepliesRepo(db),
		crdtStore: crdtStore,
	}
}

// AddReply adds a reply and updates CRDT counter
func (repo *CRDTReplyRepo) AddReply(reply domain.Tweet) (domain.Tweet, error) {
	// Add reply locally first
	added, err := repo.ReplyRepo.AddReply(reply)
	if err != nil {
		return domain.Tweet{}, err
	}

	// Update CRDT counter for the root tweet
	if repo.crdtStore != nil {
		_, err := repo.crdtStore.IncrementStat(reply.RootId, crdt.StatTypeReplies)
		if err != nil {
			log.Warnf("failed to increment CRDT reply count: %v", err)
		}
	}

	return added, nil
}

// DeleteReply deletes a reply and updates CRDT counter
func (repo *CRDTReplyRepo) DeleteReply(rootID, parentID, replyID string) error {
	// Delete reply locally first
	err := repo.ReplyRepo.DeleteReply(rootID, parentID, replyID)
	if err != nil {
		return err
	}

	// Update CRDT counter
	if repo.crdtStore != nil {
		_, err := repo.crdtStore.DecrementStat(rootID, crdt.StatTypeReplies)
		if err != nil {
			log.Warnf("failed to decrement CRDT reply count: %v", err)
		}
	}

	return nil
}

// RepliesCount returns the aggregated reply count from CRDT if available
func (repo *CRDTReplyRepo) RepliesCount(tweetId string) (uint64, error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}

	// Try to get aggregated count from CRDT first
	if repo.crdtStore != nil {
		count, err := repo.crdtStore.GetAggregatedStat(tweetId, crdt.StatTypeReplies)
		if err != nil {
			log.Warnf("failed to get CRDT reply count, falling back to local: %v", err)
			return repo.ReplyRepo.RepliesCount(tweetId)
		}
		return count, nil
	}

	// Fallback to local count
	return repo.ReplyRepo.RepliesCount(tweetId)
}

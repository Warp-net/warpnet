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
	"github.com/Warp-net/warpnet/json"
)

// CRDTReplyRepo manages replies with CRDT-based statistics
type CRDTReplyRepo struct {
	db        ReplyStorer
	crdtStore *crdt.CRDTStatsStore
}

// NewCRDTReplyRepo creates a new CRDT-enabled reply repository
func NewCRDTReplyRepo(db ReplyStorer, crdtStore *crdt.CRDTStatsStore) *CRDTReplyRepo {
	return &CRDTReplyRepo{
		db:        db,
		crdtStore: crdtStore,
	}
}

// AddReply adds a reply and updates CRDT counter
func (repo *CRDTReplyRepo) AddReply(reply domain.Tweet) (domain.Tweet, error) {
	// Add reply using base repo
	baseRepo := &ReplyRepo{db: repo.db}
	added, err := baseRepo.AddReply(reply)
	if err != nil {
		return domain.Tweet{}, err
	}

	// Update CRDT counter for the root tweet
	if repo.crdtStore != nil {
		current, _ := repo.crdtStore.Get(reply.RootId, crdt.StatTypeReplies)
		newValue := current + 1
		if err := repo.crdtStore.Put(reply.RootId, crdt.StatTypeReplies, newValue); err != nil {
			return added, err
		}
	}

	return added, nil
}

// DeleteReply deletes a reply and updates CRDT counter
func (repo *CRDTReplyRepo) DeleteReply(rootID, parentID, replyID string) error {
	// Delete reply using base repo
	baseRepo := &ReplyRepo{db: repo.db}
	err := baseRepo.DeleteReply(rootID, parentID, replyID)
	if err != nil {
		return err
	}

	// Update CRDT counter
	if repo.crdtStore != nil {
		current, _ := repo.crdtStore.Get(rootID, crdt.StatTypeReplies)
		if current > 0 {
			newValue := current - 1
			if err := repo.crdtStore.Put(rootID, crdt.StatTypeReplies, newValue); err != nil {
				return err
			}
		}
	}

	return nil
}

// RepliesCount returns the aggregated reply count from CRDT
func (repo *CRDTReplyRepo) RepliesCount(tweetId string) (uint64, error) {
	if tweetId == "" {
		return 0, local.DBError("empty tweet id")
	}

	if repo.crdtStore != nil {
		return repo.crdtStore.GetAggregatedStat(tweetId, crdt.StatTypeReplies)
	}

	return 0, nil
}

// Get retrieves a reply
func (repo *CRDTReplyRepo) Get(replyID, rootID string) (domain.Tweet, error) {
	// The original ReplyRepo doesn't have a Get method, we need to query directly
	treeKey := local.NewPrefixBuilder(RepliesNamespace).
		AddRootID(rootID).
		AddRange(local.FixedRangeKey).
		AddParentId(replyID).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return domain.Tweet{}, err
	}
	defer txn.Rollback()

	value, err := txn.Get(treeKey)
	if err != nil {
		return domain.Tweet{}, err
	}

	var reply domain.Tweet
	if err := json.Unmarshal(value, &reply); err != nil {
		return domain.Tweet{}, err
	}

	return reply, txn.Commit()
}

// List retrieves replies for a tweet
func (repo *CRDTReplyRepo) List(rootID, parentID string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
	// Use existing List implementation
	prefix := local.NewPrefixBuilder(RepliesNamespace).
		AddRootID(rootID).
		AddParentId(parentID).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	items, nextCursor, err := txn.List(prefix, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	replies := make([]domain.Tweet, 0, len(items))
	for _, item := range items {
		var reply domain.Tweet
		if err := json.Unmarshal(item.Value, &reply); err != nil {
			continue
		}
		replies = append(replies, reply)
	}

	_ = txn.Commit()
	return replies, nextCursor, nil
}

// GetRepliesTree retrieves the full tree of replies
func (repo *CRDTReplyRepo) GetRepliesTree(rootID, parentID string, limit *uint64, cursor *string) ([]domain.ReplyNode, string, error) {
	baseRepo := &ReplyRepo{db: repo.db}
	return baseRepo.GetRepliesTree(rootID, parentID, limit, cursor)
}

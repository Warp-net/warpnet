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
	"fmt"
	"sort"
	"time"

	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/oklog/ulid/v2"
	log "github.com/sirupsen/logrus"
)

var ErrReplyNotFound = local_store.DBError("reply not found")

const (
	RepliesNamespace     = "/REPLY"
	repliesCountSubspace = "REPLIESCOUNT"
)

type ReplyStorer interface {
	Set(key local_store.DatabaseKey, value []byte) error
	Get(key local_store.DatabaseKey) ([]byte, error)
	Delete(key local_store.DatabaseKey) error
	NewTxn() (local_store.WarpTransactioner, error)
}

type ReplyStatsStorer interface {
	GetAggregatedStat(key ds.Key) (uint64, error)
	Increment(key ds.Key) error
	Decrement(key ds.Key) error
}

type ReplyRepo struct {
	db      ReplyStorer
	statsDb ReplyStatsStorer
}

func NewRepliesRepo(db ReplyStorer, statsDb ReplyStatsStorer) *ReplyRepo {
	return &ReplyRepo{db: db, statsDb: statsDb}
}

func (repo *ReplyRepo) AddReply(reply domain.Tweet) (domain.Tweet, error) {
	if reply == (domain.Tweet{}) {
		return reply, local_store.DBError("empty reply")
	}
	if reply.RootId == "" {
		return reply, local_store.DBError("empty root")
	}
	if reply.ParentId == nil {
		return reply, local_store.DBError("empty parent")
	}
	if reply.Id == "" {
		reply.Id = ulid.Make().String()
	}
	if reply.Id == reply.RootId {
		return reply, local_store.DBError("this is tweet not reply")
	}
	if reply.CreatedAt.IsZero() {
		now := time.Now()
		reply.CreatedAt = now
	}

	data, err := json.Marshal(reply)
	if err != nil {
		return reply, fmt.Errorf("marshalling reply meta: %w", err)
	}

	treeKey := local_store.NewPrefixBuilder(RepliesNamespace).
		AddRootID(reply.RootId).
		AddRange(local_store.FixedRangeKey).
		AddParentId(reply.Id).
		Build()

	parentSortableKey := local_store.NewPrefixBuilder(RepliesNamespace).
		AddRootID(reply.RootId).
		AddParentId(*reply.ParentId).
		AddId(reply.Id).
		AddReversedTimestamp(reply.CreatedAt).
		Build()

	replyCountKey := local_store.NewPrefixBuilder(RepliesNamespace).
		AddSubPrefix(repliesCountSubspace).
		AddRootID(*reply.ParentId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return reply, fmt.Errorf("creating transaction: %w", err)
	}
	defer txn.Rollback()

	if err := txn.Set(treeKey, parentSortableKey.Bytes()); err != nil {
		return reply, fmt.Errorf("adding reply sortable key: %w", err)
	}
	if err := txn.Set(parentSortableKey, data); err != nil {
		return reply, fmt.Errorf("adding reply data: %w", err)
	}
	_, err = txn.Increment(replyCountKey)
	if err != nil {
		return reply, err
	}
	if err := txn.Commit(); err != nil {
		return reply, err
	}
	if repo.statsDb == nil {
		return reply, nil
	}
	if err := repo.statsDb.Increment(replyCountKey.DatastoreKey()); err != nil {
		log.Warnf("reply: stats db increment: %v", err)
	}
	return reply, nil
}

func (repo *ReplyRepo) GetReply(rootID string, replyId string) (tweet domain.Tweet, err error) {
	if rootID == "" || replyId == "" {
		return tweet, local_store.DBError("rootID and replyId cannot be empty")
	}

	treeKey := local_store.NewPrefixBuilder(RepliesNamespace).
		AddRootID(rootID).
		AddRange(local_store.FixedRangeKey).
		AddParentId(replyId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return tweet, fmt.Errorf("creating transaction: %w", err)
	}
	defer txn.Rollback()

	sortableKey, err := txn.Get(treeKey)
	if err != nil {
		return tweet, err
	}

	data, err := txn.Get(local_store.DatabaseKey(sortableKey))
	if err != nil {
		return tweet, err
	}

	if err = json.Unmarshal(data, &tweet); err != nil {
		return tweet, fmt.Errorf("unmarshalling reply: %w", err)
	}
	return tweet, txn.Commit()
}

func (repo *ReplyRepo) RepliesCount(tweetId string) (likesNum uint64, err error) {
	if tweetId == "" {
		return 0, local_store.DBError("empty tweet id")
	}
	replyCountKey := local_store.NewPrefixBuilder(RepliesNamespace).
		AddSubPrefix(repliesCountSubspace).
		AddRootID(tweetId).
		Build()

	if repo.statsDb != nil {
		total, err := repo.statsDb.GetAggregatedStat(replyCountKey.DatastoreKey())
		if err == nil {
			return total, nil
		}
		log.Warnf("get replies count stat: %v", err)
	}

	bt, err := repo.db.Get(replyCountKey)

	if local_store.IsNotFoundError(err) {
		return 0, ErrReplyNotFound
	}
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(bt), nil
}

func (repo *ReplyRepo) DeleteReply(rootID, parentID, replyID string) error {
	if rootID == "" || parentID == "" || replyID == "" {
		return local_store.DBError("rootID, parent ID or replyID cannot be empty")
	}

	treeKey := local_store.NewPrefixBuilder(RepliesNamespace).
		AddRootID(rootID).
		AddRange(local_store.FixedRangeKey).
		AddParentId(replyID).
		Build()

	replyCountKey := local_store.NewPrefixBuilder(RepliesNamespace).
		AddSubPrefix(repliesCountSubspace).
		AddRootID(parentID).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return fmt.Errorf("creating transaction: %w", err)
	}
	defer txn.Rollback()

	sortableKey, err := txn.Get(treeKey)
	if err != nil {
		return fmt.Errorf("getting sortable key: %w", err)
	}
	if err := txn.Delete(treeKey); err != nil {
		return fmt.Errorf("deleting tree key: %w", err)
	}
	if err := txn.Delete(local_store.DatabaseKey(sortableKey)); err != nil {
		return fmt.Errorf(""+
			"deleting sortable key: %w", err)
	}
	_, err = txn.Decrement(replyCountKey)
	if err != nil {
		return err
	}
	if err := txn.Commit(); err != nil {
		return err
	}
	if repo.statsDb == nil {
		return nil
	}
	if err := repo.statsDb.Decrement(replyCountKey.DatastoreKey()); err != nil {
		log.Warnf("reply: stats db decrement: %v", err)
	}
	return nil
}

func (repo *ReplyRepo) GetRepliesTree(rootId, parentId string, limit *uint64, cursor *string) ([]domain.ReplyNode, string, error) {
	if rootId == "" {
		return nil, "", local_store.DBError("root ID cannot be blank")
	}

	prefix := local_store.NewPrefixBuilder(RepliesNamespace).
		AddRootID(rootId).
		AddParentId(parentId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", fmt.Errorf("creating transaction: %w", err)
	}
	defer txn.Rollback()

	items, cur, err := txn.List(prefix, limit, cursor)
	if err != nil {
		return nil, "", fmt.Errorf("listing replies: %w", err)
	}

	if err := txn.Commit(); err != nil {
		return nil, "", fmt.Errorf("committing transaction: %w", err)
	}

	replies := make([]domain.Tweet, 0, len(items))
	for _, item := range items {
		var t domain.Tweet
		if err = json.Unmarshal(item.Value, &t); err != nil {
			return nil, "", fmt.Errorf("unmarshalling reply: %w", err)
		}
		replies = append(replies, t)
	}

	return buildRepliesTree(replies), cur, nil
}

func buildRepliesTree(replies []domain.Tweet) []domain.ReplyNode {
	if len(replies) == 0 {
		return []domain.ReplyNode{}
	}

	nodeMap := make(map[string]domain.ReplyNode, len(replies))
	roots := make([]domain.ReplyNode, 0, len(replies))

	for _, reply := range replies {
		if reply.Id == "" {
			continue
		}
		nodeMap[reply.Id] = domain.ReplyNode{
			Reply:    reply,
			Children: make([]domain.ReplyNode, 0, 3),
		}
	}

	for _, reply := range replies {
		if reply.Id == "" {
			continue
		}

		node, ok := nodeMap[reply.Id]
		if !ok {
			continue
		}

		if reply.ParentId == nil {
			roots = append(roots, node)
			continue
		}

		parentNode, ok := nodeMap[*reply.ParentId]
		if !ok {
			roots = append(roots, node)
			continue
		}

		parentNode.Children = append(parentNode.Children, node)
		nodeMap[*reply.ParentId] = parentNode
	}

	sort.SliceStable(roots, func(i, j int) bool {
		return roots[i].Reply.CreatedAt.After(roots[j].Reply.CreatedAt)
	})

	return roots
}

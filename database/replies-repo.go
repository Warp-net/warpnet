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
	"fmt"
	"sort"
	"time"

	"github.com/Warp-net/warpnet/database/storage"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/oklog/ulid/v2"
)

var ErrReplyNotFound = errors.New("reply not found")

const (
	RepliesNamespace     = "/REPLY"
	repliesCountSubspace = "REPLIESCOUNT"
)

type ReplyStorer interface {
	Set(key storage.DatabaseKey, value []byte) error
	Get(key storage.DatabaseKey) ([]byte, error)
	Delete(key storage.DatabaseKey) error
	NewTxn() (storage.WarpTransactioner, error)
}

type ReplyRepo struct {
	db ReplyStorer
}

func NewRepliesRepo(db ReplyStorer) *ReplyRepo {
	return &ReplyRepo{db: db}
}

func (repo *ReplyRepo) AddReply(reply domain.Tweet) (domain.Tweet, error) {
	if reply == (domain.Tweet{}) {
		return reply, errors.New("empty reply")
	}
	if reply.RootId == "" {
		return reply, errors.New("empty root")
	}
	if reply.ParentId == nil {
		return reply, errors.New("empty parent")
	}
	if reply.Id == "" {
		reply.Id = ulid.Make().String()
	}
	if reply.Id == reply.RootId {
		return reply, errors.New("this is tweet not reply")
	}
	if reply.CreatedAt.IsZero() {
		now := time.Now()
		reply.CreatedAt = now
	}

	data, err := json.JSON.Marshal(reply)
	if err != nil {
		return reply, fmt.Errorf("error marshalling reply meta: %w", err)
	}

	treeKey := storage.NewPrefixBuilder(RepliesNamespace).
		AddRootID(reply.RootId).
		AddRange(storage.FixedRangeKey).
		AddParentId(reply.Id).
		Build()

	parentSortableKey := storage.NewPrefixBuilder(RepliesNamespace).
		AddRootID(reply.RootId).
		AddParentId(*reply.ParentId).
		AddId(reply.Id).
		AddReversedTimestamp(reply.CreatedAt).
		Build()

	replyCountKey := storage.NewPrefixBuilder(RepliesNamespace).
		AddSubPrefix(repliesCountSubspace).
		AddRootID(*reply.ParentId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return reply, fmt.Errorf("error creating transaction: %w", err)
	}
	defer txn.Rollback()

	if err := txn.Set(treeKey, parentSortableKey.Bytes()); err != nil {
		return reply, fmt.Errorf("error adding reply sortable key: %w", err)
	}
	if err := txn.Set(parentSortableKey, data); err != nil {
		return reply, fmt.Errorf("error adding reply data: %w", err)
	}
	if _, err := txn.Increment(replyCountKey); err != nil {
		return reply, err
	}

	return reply, txn.Commit()
}

func (repo *ReplyRepo) GetReply(rootID string, replyId string) (tweet domain.Tweet, err error) {
	if rootID == "" || replyId == "" {
		return tweet, errors.New("rootID and replyId cannot be empty")
	}

	treeKey := storage.NewPrefixBuilder(RepliesNamespace).
		AddRootID(rootID).
		AddRange(storage.FixedRangeKey).
		AddParentId(replyId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return tweet, fmt.Errorf("error creating transaction: %w", err)
	}
	defer txn.Rollback()

	sortableKey, err := txn.Get(treeKey)
	if err != nil {
		return tweet, err
	}

	data, err := txn.Get(storage.DatabaseKey(sortableKey))
	if err != nil {
		return tweet, err
	}

	if err = json.JSON.Unmarshal(data, &tweet); err != nil {
		return tweet, fmt.Errorf("error unmarshalling reply: %w", err)
	}
	return tweet, txn.Commit()
}

func (repo *ReplyRepo) RepliesCount(tweetId string) (likesNum uint64, err error) {
	if tweetId == "" {
		return 0, errors.New("empty tweet id")
	}
	replyCountKey := storage.NewPrefixBuilder(RepliesNamespace).
		AddSubPrefix(repliesCountSubspace).
		AddRootID(tweetId).
		Build()

	bt, err := repo.db.Get(replyCountKey)

	if errors.Is(err, storage.ErrKeyNotFound) {
		return 0, ErrReplyNotFound
	}
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(bt), nil
}

func (repo *ReplyRepo) DeleteReply(rootID, parentID, replyID string) error {
	if rootID == "" || parentID == "" || replyID == "" {
		return errors.New("rootID, parent ID or replyID cannot be empty")
	}

	treeKey := storage.NewPrefixBuilder(RepliesNamespace).
		AddRootID(rootID).
		AddRange(storage.FixedRangeKey).
		AddParentId(replyID).
		Build()

	replyCountKey := storage.NewPrefixBuilder(RepliesNamespace).
		AddSubPrefix(repliesCountSubspace).
		AddRootID(parentID).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return fmt.Errorf("error creating transaction: %w", err)
	}
	defer txn.Rollback()

	sortableKey, err := txn.Get(treeKey)
	if err != nil {
		return fmt.Errorf("error getting sortable key: %w", err)
	}
	if err := txn.Delete(treeKey); err != nil {
		return fmt.Errorf("error deleting tree key: %w", err)
	}
	if err := txn.Delete(storage.DatabaseKey(sortableKey)); err != nil {
		return fmt.Errorf("error deleting sortable key: %w", err)
	}
	if _, err = txn.Decrement(replyCountKey); err != nil {
		return err
	}

	return txn.Commit()
}

func (repo *ReplyRepo) GetRepliesTree(rootId, parentId string, limit *uint64, cursor *string) ([]domain.ReplyNode, string, error) {
	if rootId == "" {
		return nil, "", errors.New("root ID cannot be blank")
	}

	prefix := storage.NewPrefixBuilder(RepliesNamespace).
		AddRootID(rootId).
		AddParentId(parentId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", fmt.Errorf("error creating transaction: %w", err)
	}
	defer txn.Rollback()

	items, cur, err := txn.List(prefix, limit, cursor)
	if err != nil {
		return nil, "", fmt.Errorf("error listing replies: %w", err)
	}

	if err := txn.Commit(); err != nil {
		return nil, "", fmt.Errorf("error committing transaction: %w", err)
	}

	replies := make([]domain.Tweet, 0, len(items))
	for _, item := range items {
		var t domain.Tweet
		if err = json.JSON.Unmarshal(item.Value, &t); err != nil {
			return nil, "", fmt.Errorf("error unmarshalling reply: %w", err)
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

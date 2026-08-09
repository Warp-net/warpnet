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
	"strings"
	"sync"
	"time"

	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/oklog/ulid/v2"
)

const OutboxNamespace = "/OUTBOX"

const outboxTTL = time.Hour * 24 * 7

type OutboxStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
}

type OutboxRepo struct {
	db OutboxStorer
	mx *sync.Mutex
}

func NewOutboxRepo(db OutboxStorer) *OutboxRepo {
	return &OutboxRepo{db: db, mx: new(sync.Mutex)}
}

func (repo *OutboxRepo) messageKey(destNodeId, messageId string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(OutboxNamespace).
		AddRootID(destNodeId).
		AddParentId(messageId).
		Build()
}

func (repo *OutboxRepo) Enqueue(destNodeId, route string, payload []byte) (event.Message, error) {
	if destNodeId == "" || route == "" {
		return event.Message{}, local_store.DBError("outbox: dest node id or route is empty")
	}

	repo.mx.Lock()
	defer repo.mx.Unlock()

	msg := event.Message{
		MessageId:   ulid.Make().String(),
		Destination: route,
		Body:        payload,
		Timestamp:   time.Now(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return msg, fmt.Errorf("outbox: marshal: %w", err)
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return msg, err
	}
	defer txn.Rollback()

	if err = txn.SetWithTTL(repo.messageKey(destNodeId, string(msg.MessageId)), data, outboxTTL); err != nil {
		return msg, err
	}
	return msg, txn.Commit()
}

func (repo *OutboxRepo) ListByNode(destNodeId string) ([]event.Message, error) {
	if destNodeId == "" {
		return nil, local_store.DBError("outbox: dest node id is empty")
	}

	prefix := local_store.NewPrefixBuilder(OutboxNamespace).
		AddRootID(destNodeId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, err
	}
	defer txn.Rollback()

	var (
		messages []event.Message
		limit    = uint64(100) //nolint:mnd
		cursor   string
	)
	for {
		items, cur, err := txn.List(prefix, &limit, &cursor)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			var msg event.Message
			if err := json.Unmarshal(item.Value, &msg); err != nil {
				return nil, fmt.Errorf("outbox: unmarshal message %s: %w", item.Key, err)
			}
			messages = append(messages, msg)
		}
		if uint64(len(items)) < limit || cur == "" || cur == "end" {
			break
		}
		cursor = cur
	}
	if err := txn.Commit(); err != nil {
		return nil, err
	}
	return messages, nil
}

func (repo *OutboxRepo) Delete(destNodeId, messageId string) error {
	if destNodeId == "" || messageId == "" {
		return local_store.DBError("outbox: dest node id or message id is empty")
	}
	repo.mx.Lock()
	defer repo.mx.Unlock()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err = txn.Delete(repo.messageKey(destNodeId, messageId)); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *OutboxRepo) ListNodes() ([]string, error) {
	prefix := local_store.NewPrefixBuilder(OutboxNamespace).Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, err
	}
	defer txn.Rollback()

	seen := make(map[string]struct{})
	nodes := make([]string, 0)
	head := OutboxNamespace + local_store.Delimeter
	err = txn.IterateKeys(prefix, func(key string) error {
		rest := strings.TrimPrefix(key, head)
		nodeId, _, ok := strings.Cut(rest, local_store.Delimeter)
		if !ok || nodeId == "" {
			return nil
		}
		if _, dup := seen[nodeId]; dup {
			return nil
		}
		seen[nodeId] = struct{}{}
		nodes = append(nodes, nodeId)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := txn.Commit(); err != nil {
		return nil, err
	}
	return nodes, nil
}

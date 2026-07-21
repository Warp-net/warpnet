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
	"github.com/Warp-net/warpnet/json"
	"github.com/oklog/ulid/v2"
)

// OutboxNamespace holds outgoing messages that could not be delivered because
// the destination node was offline. Entries are keyed per destination node and
// replayed once that node is seen online again.
const OutboxNamespace = "/OUTBOX"

// outboxTTL caps how long an undelivered message lingers before it is dropped,
// so the queue can't grow without bound for permanently dead nodes.
const outboxTTL = time.Hour * 24 * 7

// OutboxEntry is one undelivered outgoing stream request. Id is a ULID that
// doubles as the FIFO ordering key and as the stable envelope message id reused
// across redelivery attempts, so the receiver's idempotency layer can dedupe.
type OutboxEntry struct {
	Id         string    `json:"id"`
	DestNodeId string    `json:"dest_node_id"`
	Route      string    `json:"route"`
	Payload    []byte    `json:"payload"`
	Attempts   int       `json:"attempts"`
	CreatedAt  time.Time `json:"created_at"`
}

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

func (repo *OutboxRepo) entryKey(destNodeId, id string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(OutboxNamespace).
		AddRootID(destNodeId).
		AddParentId(id).
		Build()
}

// Enqueue stores an undelivered request for destNodeId and returns the created
// entry. The ULID key preserves per-node FIFO order for later replay.
func (repo *OutboxRepo) Enqueue(destNodeId, route string, payload []byte) (OutboxEntry, error) {
	if destNodeId == "" || route == "" {
		return OutboxEntry{}, local_store.DBError("outbox: dest node id or route is empty")
	}

	repo.mx.Lock()
	defer repo.mx.Unlock()

	entry := OutboxEntry{
		Id:         ulid.Make().String(),
		DestNodeId: destNodeId,
		Route:      route,
		Payload:    payload,
		CreatedAt:  time.Now(),
	}
	return entry, repo.put(entry)
}

// Save rewrites an existing entry in place (same key), used to persist an
// incremented attempt counter after a failed redelivery.
func (repo *OutboxRepo) Save(entry OutboxEntry) error {
	if entry.Id == "" || entry.DestNodeId == "" {
		return local_store.DBError("outbox: entry id or dest node id is empty")
	}
	repo.mx.Lock()
	defer repo.mx.Unlock()
	return repo.put(entry)
}

func (repo *OutboxRepo) put(entry OutboxEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("outbox: marshal: %w", err)
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err = txn.SetWithTTL(repo.entryKey(entry.DestNodeId, entry.Id), data, outboxTTL); err != nil {
		return err
	}
	return txn.Commit()
}

// ListByNode returns every queued entry for destNodeId in FIFO (oldest-first)
// order.
func (repo *OutboxRepo) ListByNode(destNodeId string) ([]OutboxEntry, error) {
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

	items, _, err := txn.List(prefix, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := txn.Commit(); err != nil {
		return nil, err
	}

	entries := make([]OutboxEntry, 0, len(items))
	for _, item := range items {
		var entry OutboxEntry
		if err := json.Unmarshal(item.Value, &entry); err != nil {
			return nil, fmt.Errorf("outbox: unmarshal entry %s: %w", item.Key, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// Delete removes a delivered entry.
func (repo *OutboxRepo) Delete(destNodeId, id string) error {
	if destNodeId == "" || id == "" {
		return local_store.DBError("outbox: dest node id or id is empty")
	}
	repo.mx.Lock()
	defer repo.mx.Unlock()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	if err = txn.Delete(repo.entryKey(destNodeId, id)); err != nil {
		return err
	}
	return txn.Commit()
}

// ListNodes returns the distinct destination node ids that currently have at
// least one queued entry, used to flush everything on startup.
func (repo *OutboxRepo) ListNodes() ([]string, error) {
	prefix := local_store.NewPrefixBuilder(OutboxNamespace).Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, err
	}
	defer txn.Rollback()

	keys, _, err := txn.ListKeys(prefix, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := txn.Commit(); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(keys))
	nodes := make([]string, 0, len(keys))
	head := OutboxNamespace + local_store.Delimeter
	for _, key := range keys {
		rest := strings.TrimPrefix(key, head)
		nodeId, _, ok := strings.Cut(rest, local_store.Delimeter)
		if !ok || nodeId == "" {
			continue
		}
		if _, dup := seen[nodeId]; dup {
			continue
		}
		seen[nodeId] = struct{}{}
		nodes = append(nodes, nodeId)
	}
	return nodes, nil
}

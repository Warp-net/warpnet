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

package crdt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/ipfs/go-cid"
	crdt "github.com/ipfs/go-ds-crdt"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	log "github.com/sirupsen/logrus"
)

const (
	StatsRepoName = "/STATS"

	incrNamespace = "incr"
	decrNamespace = "decr"

	// eventIDBytes is the size in bytes of the random nonce that
	// uniquely identifies a single increment/decrement event in the
	// CRDT keyspace. 128 bits is more than enough to avoid collisions
	// across the lifetime of any realistic warpnet deployment, even
	// after total local-data loss and node re-bootstraps.
	eventIDBytes = 16
)

// Broadcaster interface for CRDT synchronization
type Broadcaster interface {
	Broadcast(ctx context.Context, data []byte) error
	Next(ctx context.Context) ([]byte, error)
}

type CRDTStorer interface {
	ds.Datastore
}

type CRDTRouter interface {
	FindProvidersAsync(context.Context, cid.Cid, int) <-chan peer.AddrInfo
}

// CRDTStatsStore implements a canonical PN-counter on top of go-ds-crdt.
//
// Design.
// Each Increment / Decrement appends one immutable "event" entry to
// the CRDT keyspace under a globally-unique key:
//
//	/STATS/{incr|decr}/{dataKey}/{nodeID}/{eventID}
//
// where eventID is a fresh 128-bit cryptographic nonce. Values are an
// inert one-byte marker; only the presence of the key matters. The
// aggregate is then `count(incr) - count(decr)` (clamped at zero).
//
// Properties.
//   - No read-modify-write: every operation is a pure add. Concurrent
//     calls on the same node produce different keys, so they cannot
//     collide and require no in-process synchronisation.
//   - Survives total local-data loss: on a fresh restart with the
//     same nodeID, previously broadcast events are still held by
//     peers and resync via go-ds-crdt's DAG. New events use brand-new
//     nonces and never collide with re-incoming history. Aggregates
//     converge once the DAG syncs.
//   - Restart durability: a write either reached durable storage /
//     peers or it did not. There is never double-counting and never
//     "ghost" overwrites of earlier higher values.
//
// Trade-off: storage and read cost grow linearly with the number of
// events for a key. For high-volume counters this should be paired
// with a periodic checkpoint/compaction layer outside this type.
type CRDTStatsStore struct {
	crdt        *crdt.Datastore
	broadcaster Broadcaster
	ctx         context.Context
	cancel      context.CancelFunc
	prefix      string
	nodeID      string
}

// NewCRDTStatsStore creates a new CRDT-based statistics store
func NewCRDTStatsStore(
	ctx context.Context,
	broadcaster Broadcaster,
	datastore CRDTStorer,
	node host.Host,
	router CRDTRouter,
) (*CRDTStatsStore, error) {
	ctx, cancel := context.WithCancel(ctx)

	baseStore := ds.MutexWrap(datastore)
	blockstore := ds.NewBlockstore(baseStore)
	bitswapNetwork := warpnet.NewBitswapNetwork(node)
	bitswapExchange := warpnet.NewBitswapExchange(ctx, bitswapNetwork, router, blockstore)
	blockService := warpnet.NewBlockService(blockstore, bitswapExchange)
	dagService := warpnet.NewDAGService(blockService)

	l := log.StandardLogger().WithContext(ctx)

	opts := crdt.DefaultOptions()
	opts.Logger = l
	opts.PutHook = func(k ds.Key, _ []byte) {
		l.Infof("crdt: item put: %s", k.String())
	}
	opts.DeleteHook = func(k ds.Key) {
		l.Infof("crdt: item deleted: %s", k.String())
	}
	opts.RebroadcastInterval = time.Minute
	opts.DAGSyncerTimeout = time.Minute
	opts.MultiHeadProcessing = true

	crdtStore, err := crdt.New(
		baseStore,
		ds.NewKey(""), // node repo's already set the prefix
		dagService,
		broadcaster,
		opts,
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create CRDT store: %w", err)
	}

	store := &CRDTStatsStore{
		crdt:        crdtStore,
		broadcaster: broadcaster,
		ctx:         ctx,
		cancel:      cancel,
		nodeID:      node.ID().String(),
		prefix:      StatsRepoName,
	}

	return store, nil
}

// GetAggregatedStat returns the cluster-wide PN-counter for key,
// computed as `count(incr events) - count(decr events)`, clamped at
// zero. The result is eventually consistent with the union of events
// any reachable replica has merged into the local CRDT view.
func (s *CRDTStatsStore) GetAggregatedStat(key ds.Key) (uint64, error) {
	positive, err := s.countEvents(incrNamespace, key)
	if err != nil {
		return 0, err
	}
	negative, err := s.countEvents(decrNamespace, key)
	if err != nil {
		return 0, err
	}
	if positive < negative {
		return 0, nil
	}
	return positive - negative, nil
}

// Increment records a fresh "+1" event for key under this node's
// identity. Survives total local-data loss: each call writes a new,
// unique event entry; replays of historical events from peers cannot
// collide with newly issued ones.
func (s *CRDTStatsStore) Increment(key ds.Key) error {
	return s.recordEvent(incrNamespace, key)
}

// Decrement records a fresh "-1" event for key under this node's
// identity. Same uniqueness/recovery semantics as Increment.
func (s *CRDTStatsStore) Decrement(key ds.Key) error {
	return s.recordEvent(decrNamespace, key)
}

// recordEvent appends one immutable add-only event under
// /<prefix>/<namespace>/<dataKey>/<nodeID>/<eventID>. The value is a
// single inert byte: only the presence of the key participates in
// the aggregate.
func (s *CRDTStatsStore) recordEvent(namespace string, dataKey ds.Key) error {
	eventID, err := newEventID()
	if err != nil {
		return fmt.Errorf("crdt stats: generate event id: %w", err)
	}
	eventKey := ds.NewKey(fmt.Sprintf(
		"/%s/%s/%s/%s/%s",
		s.prefix, namespace, dataKey.String(), s.nodeID, eventID,
	))
	if err := s.crdt.Put(s.ctx, eventKey, []byte{1}); err != nil {
		return fmt.Errorf("crdt stats: record %s event %s: %w", namespace, eventKey, err)
	}
	return nil
}

// countEvents returns the number of distinct event keys under the
// (namespace, key) prefix. Since every event key is globally unique
// across the lifetime of the network, the count is exactly the
// number of `+1` (or `-1`) operations that have been merged into
// the local CRDT view, regardless of how many nodes contributed
// them or how many times any of them restarted.
func (s *CRDTStatsStore) countEvents(namespace string, key ds.Key) (uint64, error) {
	prefix := ds.NewKey(
		fmt.Sprintf("/%s/%s/%s", s.prefix, namespace, key.String()),
	)
	results, err := s.crdt.Query(s.ctx, ds.Query{
		Prefix:   prefix.String(),
		KeysOnly: true,
	})
	if err != nil {
		return 0, fmt.Errorf("crdt stats: query %s: %w", prefix, err)
	}
	defer func() { _ = results.Close() }()

	var n uint64
	for r := range results.Next() {
		if r.Error != nil {
			return 0, fmt.Errorf("crdt stats: iterate %s: %w", prefix, r.Error)
		}
		n++
	}
	return n, nil
}

// newEventID returns a hex-encoded random nonce used as the leaf
// component of an event key. crypto/rand keeps the IDs unpredictable
// and collision-resistant without any persistent local state, which
// is what makes the counter survive total local-data loss.
func newEventID() (string, error) {
	var buf [eventIDBytes]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// Close stops the CRDT store
func (s *CRDTStatsStore) Close() error {
	if s == nil {
		return nil
	}
	s.cancel()
	return s.crdt.Close()
}

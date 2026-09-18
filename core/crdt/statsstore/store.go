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

// Package statsstore replicates network-wide counters over a CRDT.
package statsstore

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	ds "github.com/Warp-net/warpnet/database/datastore"
	crdt "github.com/ipfs/go-ds-crdt"
	log "github.com/sirupsen/logrus"
)

// Broadcaster carries this store's deltas to the other replicas.
type Broadcaster interface {
	Broadcast(ctx context.Context, data []byte) error
	Next(ctx context.Context) ([]byte, error)
}

// Datastore is the local storage the replica is built on.
type Datastore interface {
	Get(ctx context.Context, key ds.Key) ([]byte, error)
	Has(ctx context.Context, key ds.Key) (bool, error)
	GetSize(ctx context.Context, key ds.Key) (int, error)
	Query(ctx context.Context, q ds.Query) (ds.Results, error)
	Put(ctx context.Context, key ds.Key, value []byte) error
	Delete(ctx context.Context, key ds.Key) error
	Sync(ctx context.Context, prefix ds.Key) error
	Close() error
}

// Router finds the peers holding a block.
type Router interface {
	FindProvidersAsync(context.Context, warpnet.WarpCID, int) <-chan warpnet.WarpAddrInfo
}

// GossipTopic is the pubsub topic this store's replicas converge on.
const GossipTopic = "/warpnet/stats/1.0.0"

// bitswapPrefix keeps this store's block exchange off the protocols the
// rating store's own bitswap registers on the same host.
const bitswapPrefix = "/warpnet/stats"

const (
	// counterPrefix separates the counters from the datastore's own keys.
	counterPrefix = "/STATS"

	incrNamespace = "incr"
	decrNamespace = "decr"

	// generationIDBytes is the size of the random nonce that tags
	// every value this process writes to the CRDT. 128 bits make
	// collisions across the lifetime of the network infeasible, so
	// no two process lifetimes ever share a sub-counter — even after
	// total local-data loss followed by a re-bootstrap with the same
	// nodeID.
	generationIDBytes = 16

	flushInterval       = 30 * time.Second
	rebroadcastInterval = 5 * time.Minute
	dagSyncerTimeout    = 1 * time.Minute
	numWorkers          = 16
)

type counter struct {
	total   uint64
	flushed uint64
}

// Store is a PN-counter replicated over go-ds-crdt: every process owns
// the keys of its own generation, and a read sums them all.
type Store struct {
	crdt       *crdt.Datastore
	ctx        context.Context
	cancel     context.CancelFunc
	nodeID     string
	generation string
	stopChan   chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup

	flushMu sync.Mutex

	mu       sync.Mutex
	counters map[string]*counter
}

// New creates a new CRDT-based statistics store
func New(
	ctx context.Context,
	broadcaster Broadcaster,
	datastore Datastore,
	node warpnet.P2PNode,
	router Router,
) (*Store, error) {
	ctx, cancel := context.WithCancel(ctx)

	baseStore := ds.MutexWrap(datastore)

	blockstore := ds.NewIdStore(ds.NewBlockstore(baseStore, ds.WriteThrough(true)))

	bitswapNetwork := warpnet.NewBitswapNetwork(node, warpnet.BitswapPrefix(bitswapPrefix))
	bitswapExchange := warpnet.NewBitswapExchange(ctx, bitswapNetwork, router, blockstore)

	for _, p := range node.Network().Peers() {
		bitswapExchange.PeerConnected(p)
	}

	blockService := warpnet.NewBlockService(blockstore, bitswapExchange)
	dagService := warpnet.NewDAGService(blockService)

	generation, err := newGenerationID()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to generate stats generation: %w", err)
	}

	store := &Store{
		ctx:        ctx,
		cancel:     cancel,
		nodeID:     node.ID().String(),
		generation: generation,
		stopChan:   make(chan struct{}),
		counters:   make(map[string]*counter),
	}

	opts := crdt.DefaultOptions()
	opts.Logger = log.StandardLogger().WithContext(ctx)
	opts.RebroadcastInterval = rebroadcastInterval
	opts.DAGSyncerTimeout = dagSyncerTimeout
	opts.NumWorkers = numWorkers
	opts.RepairInterval = 0
	opts.MultiHeadProcessing = true

	crdtStore, err := crdt.New(
		baseStore,
		ds.NewKey(""), // the repo has already set the prefix
		dagService,
		broadcaster,
		opts,
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create CRDT store: %w", err)
	}
	store.crdt = crdtStore

	store.wg.Add(1)
	go store.run()

	return store, nil
}

func (s *Store) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := s.flush(); err != nil {
				log.Warnf("crdt stats: %v", err)
			}
		}
	}
}

func (s *Store) GetAggregatedStat(key ds.Key) (uint64, error) {
	positive, err := s.sumNamespace(incrNamespace, key)
	if err != nil {
		return 0, err
	}
	negative, err := s.sumNamespace(decrNamespace, key)
	if err != nil {
		return 0, err
	}
	if positive < negative {
		return 0, nil
	}
	return positive - negative, nil
}

func (s *Store) Increment(key ds.Key) error {
	s.bump(incrNamespace, key)
	return nil
}

func (s *Store) Decrement(key ds.Key) error {
	s.bump(decrNamespace, key)
	return nil
}

func (s *Store) bump(namespace string, dataKey ds.Key) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cacheKey := s.counterKey(namespace, dataKey).String()
	c := s.counters[cacheKey]
	if c == nil {
		c = &counter{}
		s.counters[cacheKey] = c
	}
	c.total++
}

func (s *Store) flush() error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	s.mu.Lock()
	pending := make(map[string]uint64, len(s.counters))
	for key, c := range s.counters {
		if c.total != c.flushed {
			pending[key] = c.total
		}
	}
	s.mu.Unlock()

	if len(pending) == 0 {
		return nil
	}

	batch, err := s.crdt.Batch(s.ctx)
	if err != nil {
		return fmt.Errorf("open counter batch: %w", err)
	}
	for key, value := range pending {
		if err := batch.Put(s.ctx, ds.NewKey(key), encodeCounter(value)); err != nil {
			return fmt.Errorf("write counter %s: %w", key, err)
		}
	}
	if err := batch.Commit(s.ctx); err != nil {
		return fmt.Errorf("commit %d counters: %w", len(pending), err)
	}

	s.mu.Lock()
	for key, value := range pending {
		s.counters[key].flushed = value
	}
	s.mu.Unlock()
	return nil
}

func (s *Store) counterKey(namespace string, dataKey ds.Key) ds.Key {
	return ds.NewKey(fmt.Sprintf(
		"%s/%s/%s/%s/%s",
		counterPrefix, namespace, dataKey.String(), s.nodeID, s.generation,
	))
}

func (s *Store) sumNamespace(namespace string, key ds.Key) (uint64, error) {
	prefix := ds.NewKey(
		fmt.Sprintf("%s/%s/%s", counterPrefix, namespace, key.String()),
	)
	results, err := s.crdt.Query(s.ctx, ds.Query{Prefix: prefix.String()})
	if err != nil {
		return 0, fmt.Errorf("crdt stats: query %s: %w", prefix, err)
	}
	defer func() { _ = results.Close() }()

	var total uint64
	for r := range results.Next() {
		if r.Error != nil {
			return 0, fmt.Errorf("crdt stats: iterate %s: %w", prefix, r.Error)
		}
		total += decodeCounter(r.Value)
	}

	s.mu.Lock()
	if c := s.counters[s.counterKey(namespace, key).String()]; c != nil {
		total += c.total - c.flushed
	}
	s.mu.Unlock()

	return total, nil
}

// newGenerationID returns a hex-encoded 128-bit random nonce that
// tags every value this process writes. Survival of total
// local-data loss depends on this nonce being unique per process
// lifetime; crypto/rand without any persistent local state is what
// makes that hold.
func newGenerationID() (string, error) {
	var buf [generationIDBytes]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// Close stops the CRDT store
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	if err := s.flush(); err != nil {
		log.Warnf("crdt stats: final flush: %v", err)
	}
	s.stopOnce.Do(func() { close(s.stopChan) })
	s.wg.Wait()
	s.cancel()
	return s.crdt.Close()
}

func encodeCounter(value uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, value)
	return buf
}

func decodeCounter(data []byte) uint64 {
	if len(data) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(data)
}

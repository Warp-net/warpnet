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
	"github.com/ipfs/go-cid"
	crdt "github.com/ipfs/go-ds-crdt"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	log "github.com/sirupsen/logrus"
)

const (
	repoName = "/STATS"

	incrNamespace = "incr"
	decrNamespace = "decr"

	// generationIDBytes is the size of the random nonce that tags
	// every value this process writes to the CRDT. 128 bits make
	// collisions across the lifetime of the network infeasible, so
	// no two process lifetimes ever share a sub-counter — even after
	// total local-data loss followed by a re-bootstrap with the same
	// nodeID.
	generationIDBytes = 16
)

// Broadcaster carries this store's deltas to the other replicas.
type Broadcaster interface {
	Broadcast(ctx context.Context, data []byte) error
	Next(ctx context.Context) ([]byte, error)
}

// Datastore is the local storage the replica is built on.
type Datastore interface {
	ds.Datastore
}

// Router finds the peers holding a block.
type Router interface {
	FindProvidersAsync(context.Context, cid.Cid, int) <-chan peer.AddrInfo
}

// Store is a PN-counter replicated over go-ds-crdt: every process owns
// the keys of its own generation, and a read sums them all.
type Store struct {
	crdt        *crdt.Datastore
	broadcaster Broadcaster
	ctx         context.Context
	cancel      context.CancelFunc
	prefix      string
	nodeID      string
	generation  string

	mu           sync.Mutex
	incrCounters map[string]uint64 // dataKey.String() -> this generation's running incr count
	decrCounters map[string]uint64
}

// New creates a new CRDT-based statistics store
func New(
	ctx context.Context,
	broadcaster Broadcaster,
	datastore Datastore,
	node host.Host,
	router Router,
) (*Store, error) {
	ctx, cancel := context.WithCancel(ctx)

	baseStore := ds.MutexWrap(datastore)

	blockstore := ds.NewIdStore(ds.NewBlockstore(baseStore, ds.WriteThrough(true)))

	bitswapNetwork := warpnet.NewBitswapNetwork(node)
	bitswapExchange := warpnet.NewBitswapExchange(ctx, bitswapNetwork, router, blockstore)

	for _, p := range node.Network().Peers() {
		bitswapExchange.PeerConnected(p)
	}

	blockService := warpnet.NewBlockService(blockstore, bitswapExchange)
	dagService := warpnet.NewDAGService(blockService)

	l := log.StandardLogger().WithContext(ctx)

	opts := crdt.DefaultOptions()
	opts.Logger = l
	opts.PutHook = func(k ds.Key, _ []byte) {
		// l.Infof("crdt: item put: %s", k.String())
	}
	opts.DeleteHook = func(k ds.Key) {
		// l.Infof("crdt: item deleted: %s", k.String())
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

	gen, err := newGenerationID()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to generate stats generation: %w", err)
	}

	store := &Store{
		crdt:         crdtStore,
		broadcaster:  broadcaster,
		ctx:          ctx,
		cancel:       cancel,
		nodeID:       node.ID().String(),
		prefix:       repoName,
		generation:   gen,
		incrCounters: make(map[string]uint64),
		decrCounters: make(map[string]uint64),
	}

	return store, nil
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
	return s.bump(incrNamespace, key, s.incrCounters)
}

func (s *Store) Decrement(key ds.Key) error {
	return s.bump(decrNamespace, key, s.decrCounters)
}

func (s *Store) bump(namespace string, dataKey ds.Key, cache map[string]uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cacheKey := dataKey.String()
	newValue := cache[cacheKey] + 1

	fullKey := ds.NewKey(fmt.Sprintf(
		"/%s/%s/%s/%s/%s",
		s.prefix, namespace, cacheKey, s.nodeID, s.generation,
	))
	if err := s.crdt.Put(s.ctx, fullKey, encodeCounter(newValue)); err != nil {
		return fmt.Errorf("crdt stats: write %s counter %s: %w", namespace, fullKey, err)
	}
	cache[cacheKey] = newValue
	return nil
}

func (s *Store) sumNamespace(namespace string, key ds.Key) (uint64, error) {
	prefix := ds.NewKey(
		fmt.Sprintf("/%s/%s/%s", s.prefix, namespace, key.String()),
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

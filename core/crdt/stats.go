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
	StatsRepoName = "/STATS"

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

// CRDTStatsStore implements a PN-counter on top of go-ds-crdt that
// stays correct under total local-data loss while keeping storage
// proportional to (nodes × restarts × keys) instead of (operations).
//
// Layout.
// Every value lives under a key of the shape
//
//	/STATS/{incr|decr}/{dataKey}/{nodeID}/{generation}
//
// where {generation} is a fresh 128-bit cryptographic nonce minted
// once per process start. Each (namespace, dataKey, nodeID,
// generation) tuple is owned by exactly one writer — the process
// that minted that generation — so write semantics inside it are
// trivially safe: we keep the value in memory, bump it under a
// mutex, and Put the new value to the CRDT. We never read our own
// value back from the CRDT, so eventual-consistency lag in the
// local DAG view cannot lose increments.
//
// GetAggregatedStat sums values across all (nodeID, generation)
// sub-counters under the prefix and returns
// `Σ incr − Σ decr` (clamped at zero).
//
// Crash-safety.
//   - Soft crash (process restart, disk intact): the next process
//     boots with a brand-new generation. The previous generation's
//     last-persisted value is still in the CRDT (and on peers); the
//     new generation starts from 0 and is summed into the aggregate
//     on top of the old one. The only loss is whatever +1's the
//     dying process buffered locally without persisting/broadcasting
//     — same fundamental durability boundary as the underlying
//     datastore.
//   - Total local-data loss: identical recovery path. Old generations
//     are pulled back via the CRDT DAG from peers; the new
//     generation cannot collide with any of them because its nonce
//     is fresh, so peers' replayed history is preserved verbatim and
//     this process simply accrues a new sub-counter alongside.
type CRDTStatsStore struct {
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

	// Match the canonical ipfs-lite blockstore wiring for go-ds-crdt:
	//   - WriteThrough(true) skips the redundant Has() check on every
	//     Put. CRDT writes blocks once and never overwrites them, so
	//     the check is pure overhead.
	//   - NewIdStore synthesises blocks for "identity" multihashes
	//     (small payloads encoded directly in the CID). go-ds-crdt
	//     occasionally produces such inline blocks for tiny deltas;
	//     without IdStore, bitswap cannot satisfy WANTs for those
	//     CIDs and replication can stall in small clusters.
	blockstore := ds.NewIdStore(ds.NewBlockstore(baseStore, ds.WriteThrough(true)))

	bitswapNetwork := warpnet.NewBitswapNetwork(node)
	bitswapExchange := warpnet.NewBitswapExchange(ctx, bitswapNetwork, router, blockstore)

	// Replay any libp2p connections that were already established
	// when bitswap registered as a network notifier. libp2p's
	// swarm.Notify only fires for FUTURE events, so peers that
	// connected during the window between libp2p.New (the host
	// starts listening) and bitswap.New (handlers wired) would
	// otherwise be invisible to bitswap's PeerManager — leading to
	// "No peers - broadcasting" loops that never converge in a small
	// cluster. ipfs-lite avoids this by ensuring nothing inbound can
	// connect before bitswap is up; here the host is already exposed
	// by the time NewCRDTStatsStore runs, so we have to replay
	// explicitly.
	for _, p := range node.Network().Peers() {
		bitswapExchange.PeerConnected(p)
	}

	blockService := warpnet.NewBlockService(blockstore, bitswapExchange)
	dagService := warpnet.NewDAGService(blockService)

	l := log.StandardLogger().WithContext(ctx)

	opts := crdt.DefaultOptions()
	opts.Logger = l
	opts.PutHook = func(k ds.Key, _ []byte) {
		//l.Infof("crdt: item put: %s", k.String())
	}
	opts.DeleteHook = func(k ds.Key) {
		//l.Infof("crdt: item deleted: %s", k.String())
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

	store := &CRDTStatsStore{
		crdt:         crdtStore,
		broadcaster:  broadcaster,
		ctx:          ctx,
		cancel:       cancel,
		nodeID:       node.ID().String(),
		prefix:       StatsRepoName,
		generation:   gen,
		incrCounters: make(map[string]uint64),
		decrCounters: make(map[string]uint64),
	}

	return store, nil
}

// GetAggregatedStat returns the cluster-wide PN-counter for key.
// Sum is taken across every (nodeID, generation) sub-counter that
// has been merged into the local CRDT view.
func (s *CRDTStatsStore) GetAggregatedStat(key ds.Key) (uint64, error) {
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

// Increment bumps this process's `incr` sub-counter for key by 1
// and persists the new running total to the CRDT.
func (s *CRDTStatsStore) Increment(key ds.Key) error {
	return s.bump(incrNamespace, key, s.incrCounters)
}

// Decrement bumps this process's `decr` sub-counter for key by 1
// and persists the new running total to the CRDT.
func (s *CRDTStatsStore) Decrement(key ds.Key) error {
	return s.bump(decrNamespace, key, s.decrCounters)
}

// bump increases this process's sub-counter for (namespace, dataKey)
// by 1 and writes the new running total to the CRDT under the
// generation-tagged key. The (nodeID, generation) sub-counter is
// owned exclusively by this process, so the in-memory value is
// always authoritative — no CRDT read, no eventual-consistency
// window, no read-modify-write hazard.
func (s *CRDTStatsStore) bump(namespace string, dataKey ds.Key, cache map[string]uint64) error {
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

// sumNamespace queries every sub-counter under the (namespace, key)
// prefix — across all known nodes and all known generations — and
// returns their sum. This is what makes a fresh post-disaster
// generation simply layer on top of the prior history that peers
// replay back to us.
func (s *CRDTStatsStore) sumNamespace(namespace string, key ds.Key) (uint64, error) {
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
func (s *CRDTStatsStore) Close() error {
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

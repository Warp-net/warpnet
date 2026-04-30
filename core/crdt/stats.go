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
	"encoding/binary"
	"errors"
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

// CRDTStatsStore manages CRDT-based tweet statistics
type CRDTStatsStore struct {
	crdt        *crdt.Datastore
	broadcaster Broadcaster
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
	prefix      string
	nodeID      string

	// localCounters keeps the authoritative value of THIS node's own
	// per-key contribution (separate maps for incr / decr namespaces).
	// It is seeded once from the CRDT store on first access and then
	// bumped in memory under s.mu, so a subsequent increment never has
	// to re-read from the CRDT view — which under go-ds-crdt may lag
	// behind our own latest Put while the DAG is being built.
	localCounters map[string]uint64
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
		crdt:          crdtStore,
		broadcaster:   broadcaster,
		ctx:           ctx,
		cancel:        cancel,
		nodeID:        node.ID().String(),
		prefix:        StatsRepoName,
		localCounters: make(map[string]uint64),
	}

	return store, nil
}

// GetAggregatedStat gets the aggregated count for a specific statistic across all nodes
func (s *CRDTStatsStore) GetAggregatedStat(key ds.Key) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var positive uint64
	var negative uint64

	for _, namespace := range []string{incrNamespace, decrNamespace} {
		prefix := ds.NewKey(
			fmt.Sprintf("/%s/%s/%s", s.prefix, namespace, key.String()),
		)

		results, err := s.crdt.Query(s.ctx, ds.Query{
			Prefix: prefix.String(),
		})
		if err != nil {
			return 0, err
		}

		entries, err := results.Rest()
		_ = results.Close()
		if err != nil {
			return 0, err
		}

		for _, entry := range entries {
			value := s.decodeCounter(entry.Value)

			if namespace == incrNamespace {
				positive += value
			} else {
				negative += value
			}
		}
	}
	if positive < negative {
		return 0, nil
	}
	return positive - negative, nil
}

func (s *CRDTStatsStore) Increment(key ds.Key) error {
	return s.bump(s.makeIncrKey(key))
}

func (s *CRDTStatsStore) Decrement(key ds.Key) error {
	return s.bump(s.makeDecrKey(key))
}

// bump increases the per-node entry under dsKey by 1 and persists it
// to the CRDT. Because every node only ever writes to keys suffixed
// with its own peer ID, intra-cluster writes never collide; the only
// remaining race is between concurrent calls on the same node, which
// s.mu serialises.
//
// The previous implementation read the current value back from the
// CRDT on every call and silently treated ANY error (including
// transient I/O failures or context cancellation) as "current = 0",
// which would clobber a real value. It also re-read state that — under
// go-ds-crdt's async DAG processing — may not yet reflect this node's
// most recent Put, opening a window for lost increments.
//
// The fix:
//   - cache the per-key value in memory under s.mu, seeded once from
//     the CRDT and bumped locally afterwards;
//   - on the seeding read, only ds.ErrNotFound is treated as 0; every
//     other error is propagated.
func (s *CRDTStatsStore) bump(dsKey ds.Key) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cacheKey := dsKey.String()
	current, seeded := s.localCounters[cacheKey]
	if !seeded {
		existing, err := s.crdt.Get(s.ctx, dsKey)
		switch {
		case err == nil:
			current = s.decodeCounter(existing)
		case errors.Is(err, ds.ErrNotFound):
			current = 0
		default:
			return fmt.Errorf("crdt stats: read counter %s: %w", cacheKey, err)
		}
	}

	newValue := current + 1
	if err := s.crdt.Put(s.ctx, dsKey, s.encodeCounter(newValue)); err != nil {
		return fmt.Errorf("crdt stats: write counter %s: %w", cacheKey, err)
	}
	s.localCounters[cacheKey] = newValue
	return nil
}

// Close stops the CRDT store
func (s *CRDTStatsStore) Close() error {
	if s == nil {
		return nil
	}
	s.cancel()
	return s.crdt.Close()
}

func (s *CRDTStatsStore) makeIncrKey(dataKey ds.Key) ds.Key {
	return ds.NewKey(fmt.Sprintf("/%s/%s/%s/%s", s.prefix, incrNamespace, dataKey.String(), s.nodeID)) // node ID must be last!
}

func (s *CRDTStatsStore) makeDecrKey(dataKey ds.Key) ds.Key {
	return ds.NewKey(fmt.Sprintf("/%s/%s/%s/%s", s.prefix, decrNamespace, dataKey.String(), s.nodeID)) // node ID must be last!
}

// encodeCounter encodes a uint64 counter value
func (s *CRDTStatsStore) encodeCounter(value uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, value)
	return buf
}

// decodeCounter decodes a uint64 counter value
func (s *CRDTStatsStore) decodeCounter(data []byte) uint64 {
	if len(data) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(data)
}

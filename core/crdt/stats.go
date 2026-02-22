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

const StatsRepoName = "/STATS"

// Broadcaster interface for CRDT synchronization
type Broadcaster interface {
	Broadcast(ctx context.Context, data []byte) error
	Next(ctx context.Context) ([]byte, error)
}

type CRDTStorer interface {
	ds.Datastore
	Prefix() string
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
}

// NewCRDTStatsStore creates a new CRDT-based statistics store
func NewCRDTStatsStore(
	ctx context.Context,
	broadcaster Broadcaster,
	datastore CRDTStorer,
	node host.Host,
	router CRDTRouter,
) (*CRDTStatsStore, error) {
	prefix := datastore.Prefix()
	if prefix != "/CRDT" {
		return nil, warpnet.WarpError("CRDT datastore namespace must start with '/CRDT' prefix")
	}
	ctx, cancel := context.WithCancel(ctx)

	baseStore := ds.MutexWrap(datastore)
	blockstore := ds.NewBlockstore(baseStore)
	bitswapNetwork := warpnet.NewBitswapNetwork(node)
	bitswapExchange := warpnet.NewBitswapExchange(ctx, bitswapNetwork, router, blockstore)
	blockService := warpnet.NewBlockService(blockstore, bitswapExchange)
	dagService := warpnet.NewDAGService(blockService)

	opts := crdt.DefaultOptions()
	l := log.StandardLogger()
	opts.Logger = l

	opts.RebroadcastInterval = time.Minute

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

// GetAggregatedStat gets the aggregated count for a specific statistic across all nodes
func (s *CRDTStatsStore) GetAggregatedStat(key ds.Key) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := s.makeKeyPrefix(key)

	results, err := s.crdt.Query(s.ctx, ds.Query{
		Prefix: prefix.String(),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to query stats: %w", err)
	}
	defer func() {
		_ = results.Close()
	}()

	var total uint64

	entries, err := results.Rest()
	if err != nil {
		return 0, fmt.Errorf("failed to collect query stats: %w", err)
	}
	if len(entries) == 0 {
		return 0, ds.ErrNotFound
	}
	for _, entry := range entries {
		value := s.decodeCounter(entry.Value)
		total += value
	}

	return total, nil
}

// Put stores a counter value
func (s *CRDTStatsStore) Put(key ds.Key, value uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dsKey := s.makeKey(key)
	data := s.encodeCounter(value)
	return s.crdt.Put(s.ctx, dsKey, data)
}

// Close stops the CRDT store
func (s *CRDTStatsStore) Close() error {
	if s == nil {
		return nil
	}
	s.cancel()
	return s.crdt.Close()
}

// makeKey creates a datastore key for a specific tweet stat on a specific node
func (s *CRDTStatsStore) makeKey(dataKey ds.Key) ds.Key {
	return ds.NewKey(fmt.Sprintf("/%s/%s/%s", s.prefix, dataKey.String(), s.nodeID)) // node ID must be last!
}

// makeKeyPrefix creates a prefix for querying all nodes' stats for a tweet
func (s *CRDTStatsStore) makeKeyPrefix(dataKey ds.Key) ds.Key {
	return ds.NewKey(fmt.Sprintf("/%s/%s", s.prefix, dataKey.String()))
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

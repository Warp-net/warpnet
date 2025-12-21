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

package stats_store

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/ipfs/boxo/bitswap"
	"github.com/ipfs/boxo/bitswap/network/bsnet"
	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/libp2p/go-libp2p/core/routing"

	"github.com/ipfs/boxo/blockstore"
	ds "github.com/ipfs/go-datastore"
	dsquery "github.com/ipfs/go-datastore/query"
	dssync "github.com/ipfs/go-datastore/sync"
	crdt "github.com/ipfs/go-ds-crdt"
	"github.com/libp2p/go-libp2p/core/host"
	log "github.com/sirupsen/logrus"
)

const (
	// namespace for CRDT stats
	namespace = "/STATS/crdt"
)

// TweetStats represents aggregated statistics for a tweet using CRDT
type TweetStats struct {
	TweetID       string
	LikesCount    uint64
	RetweetsCount uint64
	RepliesCount  uint64
	ViewsCount    uint64
}

// Broadcaster interface for CRDT synchronization
type Broadcaster interface {
	Broadcast(ctx context.Context, data []byte) error
	Next(ctx context.Context) ([]byte, error)
}

// CRDTStatsStore manages CRDT-based tweet statistics
type CRDTStatsStore struct {
	crdt        *crdt.Datastore
	broadcaster Broadcaster
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
	nodeID      string
}

// NewCRDTStatsStore creates a new CRDT-based statistics store
func NewCRDTStatsStore(
	ctx context.Context,
	broadcaster Broadcaster,
	datastore ds.Datastore,
	node host.Host,
	router routing.ContentDiscovery,
) (*CRDTStatsStore, error) {
	ctx, cancel := context.WithCancel(ctx)
	baseStore := dssync.MutexWrap(datastore)
	crdtBlockstore := blockstore.NewBlockstore(baseStore)
	bitswapNetwork := bsnet.NewFromIpfsHost(node)
	bitswapExchange := bitswap.New(ctx, bitswapNetwork, router, crdtBlockstore)
	blockService := blockservice.New(crdtBlockstore, bitswapExchange)
	dagService := merkledag.NewDAGService(blockService)

	opts := crdt.DefaultOptions()
	l := log.New()
	l.SetLevel(log.DebugLevel)
	opts.Logger = l
	opts.RebroadcastInterval = time.Minute

	crdtStore, err := crdt.New(
		baseStore,
		ds.NewKey(namespace),
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
	}

	return store, nil
}

// GetAggregatedStat gets the aggregated count for a specific statistic across all nodes
func (s *CRDTStatsStore) GetAggregatedStat(key ds.Key) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := s.makeKeyPrefix(key)

	results, err := s.crdt.Query(s.ctx, dsquery.Query{
		Prefix: prefix.String(),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to query stats: %w", err)
	}
	defer results.Close()

	var total uint64
	for result := range results.Next() {
		if result.Error != nil {
			log.Warnf("error reading result: %v", result.Error)
			continue
		}

		value := s.decodeCounter(result.Value)
		total += value
	}

	return total, nil
}

// Put stores a counter value
func (s *CRDTStatsStore) Put(key ds.Key, value uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dsKey := s.makeKey(key, s.nodeID)
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
func (s *CRDTStatsStore) makeKey(dataKey ds.Key, nodeID string) ds.Key {
	return ds.NewKey(fmt.Sprintf("/%s/%s/%s", namespace, nodeID, dataKey.String()))
}

// makeKeyPrefix creates a prefix for querying all nodes' stats for a tweet
func (s *CRDTStatsStore) makeKeyPrefix(dataKey ds.Key) ds.Key {
	return ds.NewKey(fmt.Sprintf("/%s/%s", namespace, dataKey.String()))
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

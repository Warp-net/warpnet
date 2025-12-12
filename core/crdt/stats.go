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

	blockservice "github.com/ipfs/boxo/blockservice"
	blockstore "github.com/ipfs/boxo/blockstore"
	offline "github.com/ipfs/boxo/exchange/offline"
	dagservice "github.com/ipfs/boxo/ipld/merkledag"
	ds "github.com/ipfs/go-datastore"
	dsquery "github.com/ipfs/go-datastore/query"
	dssync "github.com/ipfs/go-datastore/sync"
	crdt "github.com/ipfs/go-ds-crdt"
	log "github.com/sirupsen/logrus"
)

const (
	// StatsTopicPrefix is the pubsub topic prefix for CRDT stats synchronization
	StatsTopicPrefix = "/warpnet/stats/crdt/1.0.0"

	// namespace for CRDT stats
	namespace = "crdt"
)

// TweetStats represents aggregated statistics for a tweet using CRDT
type TweetStats struct {
	TweetID       string
	LikesCount    uint64
	RetweetsCount uint64
	RepliesCount  uint64
	ViewsCount    uint64
}

// StatType represents the type of statistics
type StatType string

const (
	StatTypeLikes    StatType = "likes"
	StatTypeRetweets StatType = "retweets"
	StatTypeReplies  StatType = "replies"
	StatTypeViews    StatType = "views"
)

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
	nodeID string,
) (*CRDTStatsStore, error) {
	ctx, cancel := context.WithCancel(ctx)

	// Create in-memory datastore for CRDT
	baseStore := dssync.MutexWrap(ds.NewMapDatastore())

	// Create a blockstore for IPLD storage
	bstore := blockstore.NewBlockstore(baseStore)

	// Create block service with offline exchange (no network fetching)
	bsrv := blockservice.New(bstore, offline.Exchange(bstore))

	// Create a DAGService for IPLD operations
	dagSyncer := dagservice.NewDAGService(bsrv)

	// Create CRDT datastore
	opts := crdt.DefaultOptions()
	opts.Logger = log.StandardLogger()
	opts.RebroadcastInterval = 0

	crdtStore, err := crdt.New(
		baseStore,
		ds.NewKey(namespace),
		dagSyncer,
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
		nodeID:      nodeID,
	}

	return store, nil
}

// GetAggregatedStat gets the aggregated count for a specific statistic across all nodes
func (s *CRDTStatsStore) GetAggregatedStat(tweetID string, statType StatType) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := s.makeKeyPrefix(tweetID, statType)

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

// GetTweetStats returns all statistics for a tweet
func (s *CRDTStatsStore) GetTweetStats(tweetID string) (*TweetStats, error) {
	likes, err := s.GetAggregatedStat(tweetID, StatTypeLikes)
	if err != nil {
		return nil, err
	}

	retweets, err := s.GetAggregatedStat(tweetID, StatTypeRetweets)
	if err != nil {
		return nil, err
	}

	replies, err := s.GetAggregatedStat(tweetID, StatTypeReplies)
	if err != nil {
		return nil, err
	}

	views, err := s.GetAggregatedStat(tweetID, StatTypeViews)
	if err != nil {
		return nil, err
	}

	return &TweetStats{
		TweetID:       tweetID,
		LikesCount:    likes,
		RetweetsCount: retweets,
		RepliesCount:  replies,
		ViewsCount:    views,
	}, nil
}

// Put stores a counter value
func (s *CRDTStatsStore) Put(tweetID string, statType StatType, value uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.makeKey(tweetID, statType, s.nodeID)
	data := s.encodeCounter(value)
	return s.crdt.Put(s.ctx, key, data)
}

// Get retrieves a counter value for this node
func (s *CRDTStatsStore) Get(tweetID string, statType StatType) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := s.makeKey(tweetID, statType, s.nodeID)
	data, err := s.crdt.Get(s.ctx, key)
	if err != nil {
		if err == ds.ErrNotFound {
			return 0, nil
		}
		return 0, err
	}
	return s.decodeCounter(data), nil
}

// Close stops the CRDT store
func (s *CRDTStatsStore) Close() error {
	s.cancel()
	return s.crdt.Close()
}

// makeKey creates a datastore key for a specific tweet stat on a specific node
func (s *CRDTStatsStore) makeKey(tweetID string, statType StatType, nodeID string) ds.Key {
	return ds.NewKey(fmt.Sprintf("/%s/%s/%s/%s", namespace, tweetID, statType, nodeID))
}

// makeKeyPrefix creates a prefix for querying all nodes' stats for a tweet
func (s *CRDTStatsStore) makeKeyPrefix(tweetID string, statType StatType) ds.Key {
	return ds.NewKey(fmt.Sprintf("/%s/%s/%s", namespace, tweetID, statType))
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

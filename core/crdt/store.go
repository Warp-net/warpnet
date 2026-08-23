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
	"fmt"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	ds "github.com/Warp-net/warpnet/database/datastore"
	crdt "github.com/ipfs/go-ds-crdt"
	"github.com/libp2p/go-libp2p/core/host"
	log "github.com/sirupsen/logrus"
)

// Store is the node's CRDT replica, and a node has exactly one.
// Everything it replicates to its peers — stat counters, peer ratings
// — lives in it under its own key prefix. One blockstore, one bitswap
// exchange, one DAG and one gossip topic is all it takes to replicate
// all of them; a second replica would only buy a second copy of that
// machinery, and a second set of blocks to keep in sync.
type Store struct {
	*crdt.Datastore

	mx      sync.RWMutex
	puts    []func(k ds.Key, v []byte)
	deletes []func(k ds.Key)
}

// OnPut registers a hook fired for every merged delta. go-ds-crdt takes
// one PutHook and copies its options at construction, so the stores
// sharing this replica subscribe here instead of through them.
func (s *Store) OnPut(f func(k ds.Key, v []byte)) {
	if s == nil || f == nil {
		return
	}
	s.mx.Lock()
	s.puts = append(s.puts, f)
	s.mx.Unlock()
}

// OnDelete registers a hook fired for every merged deletion.
func (s *Store) OnDelete(f func(k ds.Key)) {
	if s == nil || f == nil {
		return
	}
	s.mx.Lock()
	s.deletes = append(s.deletes, f)
	s.mx.Unlock()
}

func (s *Store) firePut(k ds.Key, v []byte) {
	s.mx.RLock()
	hooks := s.puts
	s.mx.RUnlock()
	for _, h := range hooks {
		h(k, v)
	}
}

func (s *Store) fireDelete(k ds.Key) {
	s.mx.RLock()
	hooks := s.deletes
	s.mx.RUnlock()
	for _, h := range hooks {
		h(k)
	}
}

// NewStore builds the node's CRDT replica on top of its bitswap
// exchange.
func NewStore(
	ctx context.Context,
	broadcaster Broadcaster,
	datastore CRDTStorer,
	node host.Host,
	router CRDTRouter,
) (*Store, error) {
	s := new(Store)

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
	// by the time the store is built, so we have to replay explicitly.
	for _, p := range node.Network().Peers() {
		bitswapExchange.PeerConnected(p)
	}

	blockService := warpnet.NewBlockService(blockstore, bitswapExchange)
	dagService := warpnet.NewDAGService(blockService)

	l := log.StandardLogger().WithContext(ctx)

	opts := crdt.DefaultOptions()
	opts.Logger = l
	opts.PutHook = func(k ds.Key, v []byte) {
		// l.Infof("crdt: item put: %s", k.String())
		s.firePut(k, v)
	}
	opts.DeleteHook = func(k ds.Key) {
		// l.Infof("crdt: item deleted: %s", k.String())
		s.fireDelete(k)
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
		return nil, fmt.Errorf("failed to create CRDT store: %w", err)
	}
	s.Datastore = crdtStore
	return s, nil
}

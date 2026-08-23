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
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	ds "github.com/Warp-net/warpnet/database/datastore"
	crdt "github.com/ipfs/go-ds-crdt"
	"github.com/libp2p/go-libp2p/core/host"
	log "github.com/sirupsen/logrus"
)

// DatastoreHooks are fired by go-ds-crdt on every merged delta. The
// stats store leaves them nil; the rating store uses them to keep its
// in-memory index current without re-querying the DAG.
type DatastoreHooks struct {
	Put    func(k ds.Key, v []byte)
	Delete func(k ds.Key)
}

// NewDatastore builds a go-ds-crdt datastore on top of the node's
// bitswap exchange. Shared by the stats and rating stores: both need
// exactly the same replication plumbing and differ only in what they
// write into it.
func NewDatastore(
	ctx context.Context,
	broadcaster Broadcaster,
	datastore CRDTStorer,
	node host.Host,
	router CRDTRouter,
	hooks DatastoreHooks,
) (*crdt.Datastore, error) {
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

	opts := crdt.DefaultOptions()
	opts.Logger = log.StandardLogger().WithContext(ctx)
	opts.RebroadcastInterval = time.Minute
	opts.DAGSyncerTimeout = time.Minute
	opts.MultiHeadProcessing = true
	if hooks.Put != nil {
		opts.PutHook = hooks.Put
	}
	if hooks.Delete != nil {
		opts.DeleteHook = hooks.Delete
	}

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
	return crdtStore, nil
}

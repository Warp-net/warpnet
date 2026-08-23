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

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

package node

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"sync/atomic"

	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/crdt"
	"github.com/Warp-net/warpnet/core/dht"
	"github.com/Warp-net/warpnet/core/handler"
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/security"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	log "github.com/sirupsen/logrus"
)

type DistributedHashTableDiscoverer interface {
	ClosestPeers() []warpnet.WarpPeerID
	// FindProvidersAsync is what the rating CRDT's bitswap exchange
	// routes through; *distributedHashTable already implements it.
	FindProvidersAsync(ctx context.Context, key cid.Cid, count int) <-chan peer.AddrInfo
	Close()
}

type ModeratorNode struct {
	ctx context.Context

	node    *node.WarpNode
	options []libp2p.Option
	mw      *middleware.WarpMiddleware

	dHashTable DistributedHashTableDiscoverer

	// ratingStore backs the rating CRDT. A moderator holds no disk
	// either, so its view is restored from the DAG after a restart
	// rather than from anything local.
	ratingStore crdt.CRDTStorer
	rating      *rating.Store

	memoryStoreCloseF func() error

	version *semver.Version
	psk     security.PSK
	privKey ed25519.PrivateKey

	isClosed *atomic.Bool
}

func NewModeratorNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	ownNodeId warpnet.WarpPeerID,
) (_ *ModeratorNode, err error) {
	memoryStore, err := pstoremem.NewPeerstore()
	if err != nil {
		return nil, fmt.Errorf("moderator: fail creating memory peerstore: %w", err)
	}
	mapStore := datastore.NewMapDatastore()
	// Separate from the DHT's store: provider records and CRDT block
	// bookkeeping have no business sharing a keyspace.
	ratingStore := datastore.NewMapDatastore()

	closeF := func() error {
		_ = memoryStore.Close()
		_ = ratingStore.Close()
		return mapStore.Close()
	}

	infos, err := config.Config().Node.AddrInfos()
	if err != nil {
		return nil, err
	}

	dHashTable := dht.NewDHTable(
		ctx,
		dht.RoutingStore(mapStore),
		dht.BootstrapNodes(infos...),
		dht.Network(config.Config().Node.Network),
	)

	opts := []libp2p.Option{ //nolint:prealloc
		node.WarpIdentity(privKey),
		libp2p.Peerstore(memoryStore),
		libp2p.PrivateNetwork(warpnet.PSK(psk)),
		libp2p.ListenAddrStrings(
			[]string{
				fmt.Sprintf("/ip6/%s/tcp/%s", config.Config().Node.HostV6, config.Config().Node.Port),
				fmt.Sprintf("/ip4/%s/tcp/%s", config.Config().Node.HostV4, config.Config().Node.Port),
			}...,
		),
		libp2p.Routing(dHashTable.StartRouting),
		node.EnableAutoRelayWithStaticRelays(infos, ownNodeId)(),
	}

	opts = append(opts, node.CommonOptions...)

	mn := &ModeratorNode{
		ctx:               ctx,
		dHashTable:        dHashTable,
		ratingStore:       ratingStore,
		memoryStoreCloseF: closeF,
		psk:               psk,
		privKey:           privKey,
		version:           config.Config().Version,
		options:           opts,
		isClosed:          new(atomic.Bool),
	}

	return mn, nil
}

func (mn *ModeratorNode) Start() (err error) {
	if mn == nil {
		panic("moderator: nil node")
	}

	mn.node, err = node.NewWarpNode(mn.ctx, mn.options...)
	if err != nil {
		return fmt.Errorf("node: failed to init node: %w", err)
	}

	mn.mw = middleware.NewWarpMiddleware(mn.node.Node().ID(), nil)
	mn.mw.SetRating(mn.Rating())
	mn.node.SetStreamMiddlewares(
		mn.mw.LoggingMiddleware,
		mn.mw.RateLimiterMiddleware,
		mn.mw.AuthMiddleware,
		mn.mw.IdempotencyMiddleware,
	)

	//nolint:govet
	mn.node.SetStreamHandlers(
		warpnet.WarpStreamHandler{ //nolint:govet
			event.PUBLIC_GET_INFO,
			handler.StreamGetInfoHandler(mn, nil),
		},
	)

	nodeInfo := mn.NodeInfo()

	println()
	fmt.Printf(
		"\033[1mMODERATOR NODE STARTED WITH ID %s AND ADDRESSES %v\033[0m\n",
		nodeInfo.ID.String(), nodeInfo.Addresses,
	)
	println()
	return nil
}

// StartRating brings the rating store up once gossip exists. The
// moderator node itself has no pubsub — the moderator process owns it
// — so this cannot happen inside Start.
func (mn *ModeratorNode) StartRating(gossip crdt.GossipPubSuber, shadow rating.ShadowReporter) error {
	if mn == nil || mn.node == nil {
		return warpnet.WarpError("moderator: rating: node is not started")
	}
	store, err := node.NewRatingStore(node.RatingDeps{
		Ctx:      mn.ctx,
		Self:     mn.node.Node().ID(),
		PrivKey:  mn.privKey,
		NodeType: warpnet.ModeratorNode,
		Host:     mn.node.Node(),
		Router:   mn.dHashTable,
		Store:    mn.ratingStore,
		Gossip:   gossip,
		Shadow:   shadow,
	})
	if err != nil {
		return err
	}
	mn.rating = store
	mn.node.SetRating(store)
	mn.mw.SetRating(store)
	return nil
}

// Rating never returns nil: a moderator whose store failed to build
// must penalise nobody.
func (mn *ModeratorNode) Rating() rating.Rater {
	if mn == nil || mn.rating == nil {
		return rating.Nop{}
	}
	return mn.rating
}

// SetStreamHandlers registers additional routes after the node is up. The
// moderator uses it for routes whose handler needs the engine, which only
// exists once the moderator itself is running.
func (mn *ModeratorNode) SetStreamHandlers(handlers ...warpnet.WarpStreamHandler) {
	mn.node.SetStreamHandlers(handlers...)
}

func (mn *ModeratorNode) ID() warpnet.WarpPeerID {
	return mn.node.Node().ID()
}

func (mn *ModeratorNode) ClosestPeers() []warpnet.WarpPeerID {
	return mn.dHashTable.ClosestPeers()
}

func (mn *ModeratorNode) Node() warpnet.P2PNode {
	return mn.node.Node()
}

func (mn *ModeratorNode) NodeInfo() warpnet.NodeInfo {
	baseInfo := mn.node.BaseNodeInfo()
	baseInfo.OwnerId = "None"
	baseInfo.Type = warpnet.ModeratorNode
	return baseInfo
}

func (mn *ModeratorNode) GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error) {
	nodeId := warpnet.FromStringToPeerID(nodeIdStr)
	if nodeId == "" {
		return nil, fmt.Errorf("moderator: stream: %w: %s", warpnet.ErrMalformedNodeId, nodeIdStr)
	}
	return mn.node.Stream(nodeId, path, data)
}

func (mn *ModeratorNode) SelfStream(_, _ warpnet.WarpPeerID, _ stream.WarpRoute, _ any) (_ []byte, err error) {
	return nil, warpnet.ErrNotImplemented
}

func (mn *ModeratorNode) Stop() {
	defer func() { _ = recover() }()
	if mn == nil {
		return
	}
	mn.isClosed.Store(true)

	if mn.rating != nil {
		_ = mn.rating.Close()
	}
	if mn.dHashTable != nil {
		mn.dHashTable.Close()
	}

	if mn.memoryStoreCloseF != nil {
		if err := mn.memoryStoreCloseF(); err != nil {
			log.Errorf("moderator: failed to close memory store: %v", err)
		}
	}
	if mn.mw != nil {
		mn.mw.Close()
	}

	mn.node.StopNode()
}

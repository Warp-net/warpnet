/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU General Public License for more details.

 You should have received a copy of the GNU General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

package bootstrap

import (
	"context"
	"fmt"
	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/consensus"
	dht "github.com/Warp-net/warpnet/core/dht"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/handler"
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/node/base"
	"github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/security"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	log "github.com/sirupsen/logrus"
)

type BootstrapNode struct {
	*base.WarpNode

	discService       DiscoveryHandler
	pubsubService     PubSubProvider
	raft              ConsensusProvider
	dHashTable        DistributedHashTableCloser
	memoryStoreCloseF func() error
	psk               security.PSK
}

func NewBootstrapNode(
	ctx context.Context,
	isInMemory bool,
	seed []byte,
	psk security.PSK,
) (_ *BootstrapNode, err error) {
	privKey, err := security.GenerateKeyFromSeed(seed)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: fail generating key: %v", err)
	}
	warpPrivKey := privKey.(warpnet.WarpPrivateKey)

	raft, err := consensus.NewBootstrapRaft(ctx, isInMemory)
	if err != nil {
		return nil, err
	}

	discService := discovery.NewBootstrapDiscoveryService(ctx, raft.AddVoter)

	pubsubService := pubsub.NewPubSubBootstrap(ctx, discService.DefaultDiscoveryHandler)

	memoryStore, err := pstoremem.NewPeerstore()
	if err != nil {
		return nil, fmt.Errorf("bootstrap: fail creating memory peerstore: %w", err)
	}

	mapStore := datastore.NewMapDatastore()

	closeF := func() error {
		memoryStore.Close()
		return mapStore.Close()
	}

	dHashTable := dht.NewDHTable(
		ctx, mapStore,
		raft.RemoveVoter, discService.DefaultDiscoveryHandler, raft.AddVoter,
	)

	node, err := base.NewWarpNode(
		ctx,
		warpPrivKey,
		memoryStore,
		warpnet.BootstrapOwner,
		psk,
		fmt.Sprintf("/ip4/%s/tcp/%s", config.Config().Node.Host, config.Config().Node.Port),
		dHashTable.StartRouting,
	)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: failed to init node: %v", err)
	}

	bn := &BootstrapNode{
		WarpNode:          node,
		discService:       discService,
		pubsubService:     pubsubService,
		raft:              raft,
		dHashTable:        dHashTable,
		memoryStoreCloseF: closeF,
		psk:               psk,
	}

	mw := middleware.NewWarpMiddleware()
	logMw := mw.LoggingMiddleware
	bn.SetStreamHandler(
		event.PUBLIC_POST_NODE_VERIFY,
		logMw(mw.UnwrapStreamMiddleware(handler.StreamVerifyHandler(bn.raft))),
	)
	bn.SetStreamHandler(
		event.PUBLIC_GET_INFO,
		logMw(handler.StreamGetInfoHandler(bn, discService.DefaultDiscoveryHandler)),
	)
	bn.SetStreamHandler(
		event.PUBLIC_GET_NODE_CHALLENGE,
		logMw(mw.UnwrapStreamMiddleware(handler.StreamChallengeHandler(root.GetCodeBase(), warpPrivKey))),
	)
	return bn, nil
}

func (bn *BootstrapNode) Start() error {
	bn.pubsubService.Run(bn, nil)
	if err := bn.discService.Run(bn); err != nil {
		return err
	}

	if err := bn.raft.Start(bn); err != nil {
		return err
	}

	nodeInfo := bn.NodeInfo()

	println()
	fmt.Printf(
		"\033[1mBOOTSTRAP NODE STARTED WITH ID %s AND ADDRESSES %v\033[0m\n",
		nodeInfo.ID.String(), nodeInfo.Addresses,
	)
	println()
	return nil
}

func (bn *BootstrapNode) GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error) {
	// stub
	return nil, nil
}

func (bn *BootstrapNode) Stop() {
	if bn == nil {
		return
	}
	if bn.discService != nil {
		bn.discService.Close()
	}

	if bn.pubsubService != nil {
		if err := bn.pubsubService.Close(); err != nil {
			log.Errorf("bootstrap: failed to close pubsub: %v", err)
		}
	}

	if bn.dHashTable != nil {
		bn.dHashTable.Close()
	}
	if bn.raft != nil {
		bn.raft.Shutdown()
	}
	if bn.memoryStoreCloseF != nil {
		if err := bn.memoryStoreCloseF(); err != nil {
			log.Errorf("bootstrap: failed to close memory store: %v", err)
		}
	}

	bn.WarpNode.StopNode()
}

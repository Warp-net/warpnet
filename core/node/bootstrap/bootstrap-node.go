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

package bootstrap

import (
	"context"
	"crypto/ed25519"
	"errors"
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
	"github.com/Warp-net/warpnet/database"
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
	selfHashHex       string
}

func NewBootstrapNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	isInMemory bool,
	psk security.PSK,
	selfHashHex string,
) (_ *BootstrapNode, err error) {
	raft, err := consensus.NewBootstrapRaft(
		ctx, isInMemory,
		(&database.NodeRepo{BootstrapSelfHashHex: selfHashHex}).ValidateSelfHash,
	)
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
		privKey,
		memoryStore,
		psk,
		nil,
		[]string{
			fmt.Sprintf("/ip6/%s/tcp/%s", config.Config().Node.HostV6, config.Config().Node.Port),
			fmt.Sprintf("/ip4/%s/tcp/%s", config.Config().Node.HostV4, config.Config().Node.Port),
		},
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
		selfHashHex:       selfHashHex,
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
		logMw(mw.UnwrapStreamMiddleware(handler.StreamChallengeHandler(root.GetCodeBase(), privKey))),
	)
	return bn, nil
}

func (bn *BootstrapNode) NodeInfo() warpnet.NodeInfo {
	bi := bn.BaseNodeInfo()
	bi.OwnerId = warpnet.BootstrapOwner
	return bi
}

func (bn *BootstrapNode) Start() error {
	if bn == nil {
		return errors.New("bootstrap: nil node")
	}
	bn.pubsubService.Run(bn, nil)
	if err := bn.discService.Run(bn); err != nil {
		return err
	}

	if err := bn.raft.Start(bn); err != nil {
		return err
	}

	if err := bn.raft.AskSelfHashValidation(bn.selfHashHex); err != nil {
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
	if bn == nil {
		return
	}
	nodeId := warpnet.FromStringToPeerID(nodeIdStr)
	bt, err := bn.Stream(nodeId, path, data)
	if errors.Is(err, warpnet.ErrNodeIsOffline) {
		return bt, nil
	}
	return bt, err
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

func validateSelfHash(k, selfHashHexOwn, selfHashHexRemote string) error {
	if k != database.SelfHashConsensusKey {
		return nil
	}

	if len(selfHashHexRemote) == 0 {
		return errors.New("empty codebase hash")
	}
	if selfHashHexOwn != selfHashHexRemote {
		return errors.New("self hash is not in the consensus records")
	}

	return nil
}

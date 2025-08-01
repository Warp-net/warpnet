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
	"github.com/Warp-net/warpnet/core/node/base"
	"github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/security"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	log "github.com/sirupsen/logrus"
	"os"
)

type BootstrapNode struct {
	ctx               context.Context
	node              *base.WarpNode
	opts              []warpnet.WarpOption
	discService       DiscoveryHandler
	pubsubService     PubSubProvider
	dHashTable        DistributedHashTableCloser
	consensusService  ConsensusServicer
	memoryStoreCloseF func() error
	privKey           ed25519.PrivateKey
	psk               security.PSK
	selfHashHex       string
}

func NewBootstrapNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	selfHashHex string,
	interruptChan chan os.Signal,
) (_ *BootstrapNode, err error) {
	if len(privKey) == 0 {
		return nil, errors.New("private key is required")
	}
	discService := discovery.NewBootstrapDiscoveryService(ctx)
	pubsubService := pubsub.NewPubSubBootstrap(
		ctx,
		pubsub.NewDiscoveryTopicHandler(
			discService.WrapPubSubDiscovery(discService.DefaultDiscoveryHandler),
		),
		pubsub.NewTransitModerationHandler(),
	)
	memoryStore, err := pstoremem.NewPeerstore()
	if err != nil {
		return nil, fmt.Errorf("bootstrap: fail creating memory peerstore: %w", err)
	}
	mapStore := datastore.NewMapDatastore()

	closeF := func() error {
		_ = memoryStore.Close()
		return mapStore.Close()
	}

	infos, err := config.Config().Node.AddrInfos()
	if err != nil {
		return nil, err
	}

	dHashTable := dht.NewDHTable(
		ctx,
		dht.RoutingStore(mapStore),
		dht.EnableRendezvous(),
		dht.AddPeerCallbacks(discService.DefaultDiscoveryHandler),
		dht.BootstrapNodes(infos...),
	)

	currentNodeID, err := warpnet.IDFromPublicKey(privKey.Public().(ed25519.PublicKey))
	if err != nil {
		return nil, err
	}

	opts := []warpnet.WarpOption{
		base.WarpIdentity(privKey),
		libp2p.Peerstore(memoryStore),
		libp2p.PrivateNetwork(warpnet.PSK(psk)),
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip6/%s/tcp/%s", config.Config().Node.HostV6, config.Config().Node.Port),
			fmt.Sprintf("/ip4/%s/tcp/%s", config.Config().Node.HostV4, config.Config().Node.Port),
		),
		libp2p.Routing(dHashTable.StartRouting),
		base.EnableAutoRelayWithStaticRelays(infos, currentNodeID)(),
	}
	opts = append(opts, base.CommonOptions...)

	bn := &BootstrapNode{
		ctx:               ctx,
		opts:              opts,
		discService:       discService,
		pubsubService:     pubsubService,
		dHashTable:        dHashTable,
		memoryStoreCloseF: closeF,
		psk:               psk,
		selfHashHex:       selfHashHex,
		privKey:           privKey,
	}

	bn.consensusService = consensus.NewGossipConsensus(
		ctx, pubsubService, interruptChan, func(ev event.ValidationEvent) error {
			if len(selfHashHex) == 0 {
				return errors.New("empty codebase hash")
			}
			if selfHashHex != ev.SelfHashHex {
				return errors.New("self hash is not valid")
			}
			return nil
		},
	)

	return bn, nil
}

func (bn *BootstrapNode) NodeInfo() warpnet.NodeInfo {
	bi := bn.node.BaseNodeInfo()
	bi.OwnerId = warpnet.BootstrapOwner
	return bi
}

func (bn *BootstrapNode) Start() (err error) {
	if bn == nil {
		panic("bootstrap: nil node")
	}
	bn.node, err = base.NewWarpNode(
		bn.ctx,
		bn.opts...,
	)
	if err != nil {
		return fmt.Errorf("bootstrap: failed to init node: %v", err)
	}
	bn.setupHandlers()

	bn.pubsubService.Run(bn)
	if err := bn.discService.Run(bn); err != nil {
		return err
	}

	nodeInfo := bn.NodeInfo()

	if err := bn.consensusService.Start(bn); err != nil {
		return err
	}

	bn.consensusService.AskValidation(event.ValidationEvent{nodeInfo.ID.String(), bn.selfHashHex, nil}) // blocking call

	println()
	fmt.Printf(
		"\033[1mBOOTSTRAP NODE STARTED WITH ID %s AND ADDRESSES %v\033[0m\n",
		nodeInfo.ID.String(), nodeInfo.Addresses,
	)
	println()
	return nil
}

func (bn *BootstrapNode) setupHandlers() {
	if bn.node == nil {
		panic("bootstrap: nil bootstrap node")
	}

	bn.node.SetStreamHandlers(
		warpnet.WarpStreamHandler{
			event.INTERNAL_POST_NODE_VALIDATE,
			handler.StreamValidateHandler(bn.consensusService),
		},
		warpnet.WarpStreamHandler{
			event.PUBLIC_POST_NODE_VALIDATION_RESULT,
			handler.StreamValidationResponseHandler(bn.consensusService),
		},
		warpnet.WarpStreamHandler{
			event.PUBLIC_GET_INFO,
			handler.StreamGetInfoHandler(bn, bn.discService.DefaultDiscoveryHandler),
		},
		warpnet.WarpStreamHandler{
			event.PUBLIC_POST_NODE_CHALLENGE,
			handler.StreamChallengeHandler(root.GetCodeBase(), bn.privKey),
		},
	)
}

func (bn *BootstrapNode) SelfStream(path stream.WarpRoute, data any) (_ []byte, err error) {
	if bn == nil || bn.node == nil {
		return nil, nil
	}
	return bn.node.SelfStream(path, data)
}

func (bn *BootstrapNode) GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error) {
	if bn == nil {
		return
	}
	nodeId := warpnet.FromStringToPeerID(nodeIdStr)
	bt, err := bn.node.Stream(nodeId, path, data)
	if errors.Is(err, warpnet.ErrNodeIsOffline) {
		return bt, nil
	}
	return bt, err
}

func (bn *BootstrapNode) Node() warpnet.P2PNode {
	if bn == nil || bn.node == nil {
		return nil
	}
	return bn.node.Node()
}

func (bn *BootstrapNode) Peerstore() warpnet.WarpPeerstore {
	if bn == nil || bn.node == nil {
		return nil
	}
	return bn.node.Node().Peerstore()
}

func (bn *BootstrapNode) Network() warpnet.WarpNetwork {
	if bn == nil || bn.node == nil {
		return nil
	}
	return bn.node.Node().Network()
}

func (bn *BootstrapNode) SimpleConnect(info warpnet.WarpAddrInfo) error {
	return bn.node.Node().Connect(bn.ctx, info)
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
	if bn.memoryStoreCloseF != nil {
		if err := bn.memoryStoreCloseF(); err != nil {
			log.Errorf("bootstrap: failed to close memory store: %v", err)
		}
	}
	if bn.consensusService != nil {
		bn.consensusService.Close()
	}

	bn.node.StopNode()
}

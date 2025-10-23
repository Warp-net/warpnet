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
	"errors"
	"fmt"

	root "github.com/Warp-net/warpnet"
	bootstrapPubSub "github.com/Warp-net/warpnet/cmd/node/bootstrap/pubsub"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/dht"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/handler"
	"github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/security"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	log "github.com/sirupsen/logrus"
)

type BootstrapNode struct {
	ctx               context.Context
	node              *node.WarpNode
	opts              []warpnet.WarpOption
	discService       DiscoveryHandler
	pubsubService     PubSubProvider
	dHashTable        DistributedHashTableCloser
	memoryStoreCloseF func() error
	privKey           ed25519.PrivateKey
	psk               security.PSK
	// validation block
	selfHashHex string
}

func NewBootstrapNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	selfHashHex string,
) (_ *BootstrapNode, err error) {
	if len(privKey) == 0 {
		return nil, errors.New("private key is required")
	}
	discService := discovery.NewBootstrapDiscoveryService(ctx)
	pubsubService := bootstrapPubSub.NewPubSubBootstrap(ctx)
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
		dht.AddPeerCallbacks(discService.DefaultDiscoveryHandler),
		dht.BootstrapNodes(infos...),
	)

	currentNodeID, err := warpnet.IDFromPublicKey(privKey.Public().(ed25519.PublicKey))
	if err != nil {
		return nil, err
	}

	// WebRTC and QUIC don't support private networks yet
	opts := []warpnet.WarpOption{
		node.WarpIdentity(privKey),
		libp2p.Peerstore(memoryStore),
		libp2p.PrivateNetwork(warpnet.PSK(psk)),
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/%s/tcp/4443/ws", config.Config().Node.HostV4),
			fmt.Sprintf("/ip6/%s/tcp/%s", config.Config().Node.HostV6, config.Config().Node.Port),
			fmt.Sprintf("/ip4/%s/tcp/%s", config.Config().Node.HostV4, config.Config().Node.Port),
		),
		libp2p.Routing(dHashTable.StartRouting),
		libp2p.Transport(warpnet.NewWebsocketTransport),
		node.EnableAutoRelayWithStaticRelays(infos, currentNodeID)(),
	}
	opts = append(opts, node.CommonOptions...)

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

	return bn, nil
}

func (bn *BootstrapNode) RoutingDiscovery() warpnet.Discovery {
	return bn.dHashTable.Discovery()
}

func (bn *BootstrapNode) NodeInfo() warpnet.NodeInfo {
	bi := bn.node.BaseNodeInfo()
	bi.OwnerId = warpnet.BootstrapOwner
	bi.Hash = bn.selfHashHex
	return bi
}

func (bn *BootstrapNode) Start() (err error) {
	if bn == nil {
		panic("bootstrap: nil node")
	}
	bn.node, err = node.NewWarpNode(
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
	if nodeId == "" {
		return nil, fmt.Errorf("bootstrap: stream: node id is malformed: %s", nodeIdStr)
	}
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

	bn.node.StopNode()
}

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
	"github.com/Warp-net/warpnet/core/challenge"

	"github.com/Warp-net/warpnet/cmd/node/relay/pubsub"
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

type DiscoveryHandler interface {
	DiscoveryHandlerStream(pi warpnet.WarpAddrInfo)
	Run(n discovery.DiscoveryInfoStorer) error
	Close()
}

type PubSubProvider interface {
	Run(m pubsub.PubsubServerNodeConnector)
	Close() error
	OwnerID() string
}

type DistributedHashTableCloser interface {
	Close()
}
type MetricsOnlinePusher interface {
	PushStatusOnline(nodeId string)
	PushStatusOffline(nodeId string)
}

type RelayNode struct {
	ctx               context.Context
	node              *node.WarpNode
	opts              []warpnet.WarpOption
	discService       DiscoveryHandler
	pubsubService     PubSubProvider
	dHashTable        DistributedHashTableCloser
	memoryStoreCloseF func() error
	privKey           ed25519.PrivateKey
	psk               security.PSK
	selfHashHex       string
}

func NewRelayNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	ownNodeId warpnet.WarpPeerID,
	selfHashHex string,
	m MetricsOnlinePusher,
) (_ *RelayNode, err error) {
	if len(privKey) == 0 {
		return nil, node.ErrPrivateKeyRequired
	}
	challenger := challenge.NewSpoofChallenger(ctx)

	discService := discovery.NewRelayDiscoveryService(ctx, challenger, m)

	pubsubService := pubsub.NewPubSubRelay(
		ctx,
		pubsub.NewMemberDiscoveryTopicHandler(discService.DiscoveryHandlerPubSub),
	)

	memoryStore, err := pstoremem.NewPeerstore()
	if err != nil {
		return nil, fmt.Errorf("relay: fail creating memory peerstore: %w", err)
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
		dht.AddPeerCallbacks(discService.DiscoveryHandlerDHT),
		dht.BootstrapNodes(infos...),
		dht.Network(config.Config().Node.Network),
	)

	// WebRTC and QUIC don't support private networks yet
	opts := []warpnet.WarpOption{ //nolint:prealloc
		node.WarpIdentity(privKey),
		libp2p.Peerstore(memoryStore),
		libp2p.PrivateNetwork(warpnet.PSK(psk)),
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip6/%s/tcp/%s", config.Config().Node.HostV6, config.Config().Node.Port),
			fmt.Sprintf("/ip4/%s/tcp/%s", config.Config().Node.HostV4, config.Config().Node.Port),
		),
		libp2p.Routing(dHashTable.StartRouting),
		node.EnableAutoRelayWithStaticRelays(infos, ownNodeId)(),
	}
	opts = append(opts, node.CommonOptions...)

	rn := &RelayNode{
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

	return rn, nil
}

func (rn *RelayNode) NodeInfo() warpnet.NodeInfo {
	bi := rn.node.BaseNodeInfo()
	bi.OwnerId = "bootstrap" // relay wire marker so peers detecting relays by OwnerId (pre-Type) still skip challenging us
	bi.Type = warpnet.RelayNode
	bi.Hash = rn.selfHashHex
	return bi
}

func (rn *RelayNode) Start() (err error) {
	if rn == nil {
		panic("relay: nil node")
	}
	rn.node, err = node.NewWarpNode(
		rn.ctx,
		rn.opts...,
	)
	if err != nil {
		return fmt.Errorf("relay: failed to init node: %w", err)
	}
	rn.setupHandlers()

	rn.pubsubService.Run(rn)
	if err := rn.discService.Run(rn); err != nil {
		return err
	}

	nodeInfo := rn.NodeInfo()
	println()
	fmt.Printf(
		"\033[1mRELAY NODE STARTED WITH ID %s AND ADDRESSES %v\033[0m\n",
		nodeInfo.ID.String(), nodeInfo.Addresses,
	)
	println()
	return nil
}

func (rn *RelayNode) setupHandlers() {
	if rn.node == nil {
		panic("relay: nil relay node")
	}

	//nolint:govet
	rn.node.SetStreamHandlers(
		warpnet.WarpStreamHandler{ //nolint:govet
			event.PUBLIC_GET_INFO,
			handler.StreamGetInfoHandler(rn, rn.discService.DiscoveryHandlerStream),
		},
	)
}

func (rn *RelayNode) SelfStream(path stream.WarpRoute, data any) (_ []byte, err error) {
	if rn == nil || rn.node == nil {
		return nil, nil
	}
	return rn.node.SelfStream(path, data)
}

func (rn *RelayNode) GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error) {
	if rn == nil {
		return
	}
	nodeId := warpnet.FromStringToPeerID(nodeIdStr)
	if nodeId == "" {
		return nil, fmt.Errorf("relay: stream:%w: %s", warpnet.ErrMalformedNodeId, nodeIdStr)
	}
	bt, err := rn.node.Stream(nodeId, path, data)
	if errors.Is(err, warpnet.ErrNodeIsOffline) {
		return bt, nil
	}
	return bt, err
}

func (rn *RelayNode) Node() warpnet.P2PNode {
	if rn == nil || rn.node == nil {
		return nil
	}
	return rn.node.Node()
}

func (rn *RelayNode) SetNodePriority(pid warpnet.WarpPeerID, r warpnet.WarpReachability) {
	rn.node.Prioritizer().SetPriority(pid, r)
}

func (rn *RelayNode) SetMaxNodePriority(pid warpnet.WarpPeerID) {
	rn.node.Prioritizer().SetMaxPriority(pid)
}

func (rn *RelayNode) SetMinNodePriority(pid warpnet.WarpPeerID) {
	rn.node.Prioritizer().SetMinPriority(pid)
}

func (rn *RelayNode) Peerstore() warpnet.WarpPeerstore {
	if rn == nil || rn.node == nil {
		return nil
	}
	return rn.node.Node().Peerstore()
}

func (rn *RelayNode) Network() warpnet.WarpNetwork {
	if rn == nil || rn.node == nil {
		return nil
	}
	return rn.node.Node().Network()
}

func (rn *RelayNode) SimpleConnect(info warpnet.WarpAddrInfo) error {
	return rn.node.Node().Connect(rn.ctx, info)
}

func (rn *RelayNode) Stop() {
	if rn == nil {
		return
	}
	if rn.discService != nil {
		rn.discService.Close()
	}

	if rn.pubsubService != nil {
		if err := rn.pubsubService.Close(); err != nil {
			log.Errorf("relay: failed to close pubsub: %v", err)
		}
	}
	if rn.dHashTable != nil {
		rn.dHashTable.Close()
	}
	if rn.memoryStoreCloseF != nil {
		if err := rn.memoryStoreCloseF(); err != nil {
			log.Errorf("relay: failed to close memory store: %v", err)
		}
	}

	rn.node.StopNode()
}

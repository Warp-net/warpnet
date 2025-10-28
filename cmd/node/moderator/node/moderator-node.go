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
	"sync/atomic"

	"github.com/Masterminds/semver/v3"
	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/dht"
	"github.com/Warp-net/warpnet/core/handler"
	"github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/security"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p"
	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	log "github.com/sirupsen/logrus"
)

type DistributedHashTableDiscoverer interface {
	ClosestPeers() []warpnet.WarpPeerID
	Close()
}

type ModeratorNode struct {
	ctx context.Context

	node    *node.WarpNode
	options []libp2p.Option

	dHashTable DistributedHashTableDiscoverer

	memoryStoreCloseF func() error

	version     *semver.Version
	psk         security.PSK
	privKey     ed25519.PrivateKey
	selfHashHex string

	isClosed *atomic.Bool
}

func NewModeratorNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	selfHashHex string,
) (_ *ModeratorNode, err error) {
	memoryStore, err := pstoremem.NewPeerstore()
	if err != nil {
		return nil, fmt.Errorf("moderator: fail creating memory peerstore: %w", err)
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
		dht.BootstrapNodes(infos...),
	)

	currentNodeID, err := warpnet.IDFromPublicKey(privKey.Public().(ed25519.PublicKey))
	if err != nil {
		return nil, err
	}

	p2pPrivKey, err := p2pCrypto.UnmarshalEd25519PrivateKey(privKey)
	if err != nil {
		return nil, err
	}

	mn := &ModeratorNode{
		ctx:               ctx,
		dHashTable:        dHashTable,
		memoryStoreCloseF: closeF,
		psk:               psk,
		privKey:           privKey,
		selfHashHex:       selfHashHex,
		version:           config.Config().Version,
		options: []libp2p.Option{
			libp2p.ListenAddrStrings(
				[]string{
					fmt.Sprintf("/ip6/%s/tcp/%s", config.Config().Node.HostV6, config.Config().Node.Port),
					fmt.Sprintf("/ip4/%s/tcp/%s", config.Config().Node.HostV4, config.Config().Node.Port),
				}...,
			),
			libp2p.Transport(warpnet.NewTCPTransport),
			libp2p.Identity(p2pPrivKey),
			libp2p.Ping(false),
			libp2p.Security(warpnet.NoiseID, warpnet.NewNoise),
			libp2p.Peerstore(memoryStore),
			libp2p.PrivateNetwork(warpnet.PSK(psk)),
			libp2p.UserAgent(warpnet.WarpnetName + "-moderator"),
			libp2p.Routing(dHashTable.StartRouting),
			node.EnableAutoRelayWithStaticRelays(infos, currentNodeID)(),
		},
		isClosed: new(atomic.Bool),
	}

	return mn, nil
}

func (mn *ModeratorNode) Start() (err error) {
	if mn == nil {
		panic("moderator: nil node")
	}

	mn.node, err = node.NewWarpNode(mn.ctx, mn.options...)
	if err != nil {
		return fmt.Errorf("node: failed to init node: %v", err)
	}

	for len(mn.ClosestPeers()) == 0 {
		log.Infoln("waiting for closest peers to connect", mn.ClosestPeers())
	}

	mn.node.SetStreamHandlers(
		warpnet.WarpStreamHandler{
			event.PUBLIC_GET_INFO,
			handler.StreamGetInfoHandler(mn, nil),
		},
		warpnet.WarpStreamHandler{
			event.PUBLIC_POST_NODE_CHALLENGE,
			handler.StreamChallengeHandler(root.GetCodeBase(), mn.privKey),
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
	baseInfo.OwnerId = warpnet.ModeratorOwner
	baseInfo.Hash = mn.selfHashHex
	return baseInfo
}

func (mn *ModeratorNode) GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error) {
	nodeId := warpnet.FromStringToPeerID(nodeIdStr)
	if nodeId == "" {
		return nil, fmt.Errorf("moderator: stream: node id is malformed: %s", nodeIdStr)
	}
	return mn.node.Stream(nodeId, path, data)
}

func (mn *ModeratorNode) SelfStream(_ stream.WarpRoute, _ any) (_ []byte, err error) {
	return nil, errors.New("not implemented")
}

func (mn *ModeratorNode) Stop() {
	defer func() { recover() }()
	if mn == nil {
		return
	}
	mn.isClosed.Store(true)

	if mn.dHashTable != nil {
		mn.dHashTable.Close()
	}

	if mn.memoryStoreCloseF != nil {
		if err := mn.memoryStoreCloseF(); err != nil {
			log.Errorf("moderator: failed to close memory store: %v", err)
		}
	}

	mn.node.StopNode()
}

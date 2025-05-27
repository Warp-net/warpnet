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

package base

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/backoff"
	_ "github.com/Warp-net/warpnet/core/logging"
	"github.com/Warp-net/warpnet/core/relay"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p"
	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	log "github.com/sirupsen/logrus"
	"strings"
	"sync/atomic"
	"time"
)

const DefaultTimeout = 60 * time.Second

type Streamer interface {
	Send(peerAddr warpnet.WarpAddrInfo, r stream.WarpRoute, data []byte) ([]byte, error)
}

type BackoffEnabler interface {
	IsBackoffEnabled(id peer.ID) bool
	Reset(id peer.ID)
}

type WarpNode struct {
	ctx      context.Context
	node     warpnet.P2PNode
	relay    warpnet.WarpRelayCloser
	streamer Streamer
	backoff  BackoffEnabler

	ownerId  string
	isClosed *atomic.Bool
	version  *semver.Version

	startTime time.Time
}

func NewWarpNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	store warpnet.WarpPeerstore,
	ownerId string,
	psk security.PSK,
	listenAddrs []string,
	routingFn func(node warpnet.P2PNode) (warpnet.WarpPeerRouting, error),
) (*WarpNode, error) {

	limiter := warpnet.NewAutoScaledLimiter()

	manager, err := warpnet.NewConnManager(limiter)
	if err != nil {
		return nil, err
	}

	rm, err := warpnet.NewResourceManager(limiter)
	if err != nil {
		return nil, err
	}

	infos, err := config.Config().Node.AddrInfos()
	if err != nil {
		return nil, err
	}

	currentNodeID, err := warpnet.IDFromPublicKey(privKey.Public())
	if err != nil {
		return nil, err
	}

	reachibilityOption := EmptyOption()
	autoStaticRelaysOption := EnableAutoRelayWithStaticRelays(infos, currentNodeID)
	if ownerId == warpnet.BootstrapOwner {
		reachibilityOption = libp2p.ForceReachabilityPublic
		autoStaticRelaysOption = EmptyOption()
	}

	p2pPrivKey, err := p2pCrypto.UnmarshalEd25519PrivateKey(privKey)
	if err != nil {
		return nil, err
	}

	node, err := warpnet.NewP2PNode(
		libp2p.WithDialTimeout(DefaultTimeout),
		libp2p.ListenAddrStrings(
			listenAddrs...,
		),
		libp2p.SwarmOpts(
			WithDialTimeout(DefaultTimeout),
			WithDialTimeoutLocal(DefaultTimeout),
		),
		libp2p.Transport(warpnet.NewTCPTransport, WithDefaultTCPConnectionTimeout(DefaultTimeout)),
		libp2p.Identity(p2pPrivKey),
		libp2p.Ping(true),
		libp2p.Security(warpnet.NoiseID, warpnet.NewNoise),
		libp2p.Peerstore(store),
		libp2p.ResourceManager(rm),
		libp2p.PrivateNetwork(warpnet.PSK(psk)),
		libp2p.UserAgent(warpnet.WarpnetName),
		libp2p.ConnectionManager(manager),
		libp2p.Routing(routingFn),

		libp2p.EnableAutoNATv2(),
		libp2p.EnableRelay(),
		libp2p.EnableRelayService(relay.WithDefaultResources()), // for member nodes that have static IP
		libp2p.EnableHolePunching(),
		libp2p.EnableNATService(),
		libp2p.NATPortMap(),

		autoStaticRelaysOption(),
		reachibilityOption(),
	)
	if err != nil {
		return nil, fmt.Errorf("node: failed to init node: %v", err)
	}

	relayService, err := relay.NewRelay(node)
	if err != nil {
		return nil, fmt.Errorf("node: failed to create relay	: %v", err)
	}

	wn := &WarpNode{
		ctx:       ctx,
		node:      node,
		relay:     relayService,
		ownerId:   ownerId,
		streamer:  stream.NewStreamPool(ctx, node),
		isClosed:  new(atomic.Bool),
		version:   config.Config().Version,
		startTime: time.Now(),
		backoff:   backoff.NewSimpleBackoff(ctx, time.Minute, 5),
	}

	return wn, nil //wn.validateSupportedProtocols()
}

func (n *WarpNode) Connect(p warpnet.WarpAddrInfo) error {
	if n == nil || n.node == nil {
		return nil
	}

	peerState := n.node.Network().Connectedness(p.ID)
	isConnected := peerState == warpnet.Connected || peerState == warpnet.Limited
	if isConnected {
		return nil
	}
	if n.backoff.IsBackoffEnabled(p.ID) {
		return backoff.ErrBackoffEnabled
	}

	log.Debugf("node: connect attempt to node: %s", p.String())
	if err := n.node.Connect(n.ctx, p); err != nil {
		return fmt.Errorf("failed to connect to node: %w", err)
	}

	n.backoff.Reset(p.ID)
	log.Debugf("node: connect attempt successful: %s", p.ID.String())

	return nil
}

func (n *WarpNode) SetStreamHandler(route stream.WarpRoute, handler warpnet.WarpStreamHandler) {
	if !stream.IsValidRoute(route) {
		log.Fatalf("node: invalid route: %v", route)
	}
	n.node.SetStreamHandler(route.ProtocolID(), handler)
}

func (n *WarpNode) validateSupportedProtocols() error {
	protocols := n.node.Mux().Protocols()
	log.Infoln("node: supported protocols:", protocols)
	var (
		isAutoNatBackFound, isAutoNatRequestFound, isRelayHopFound, isRelayStopFound bool
	)

	for _, proto := range protocols {
		if strings.Contains(string(proto), "autonat/2/dial-back") {
			isAutoNatBackFound = true
		}
		if strings.Contains(string(proto), "autonat/2/dial-request") {
			isAutoNatRequestFound = true
		}
		if strings.Contains(string(proto), "relay/0.2.0/hop") {
			isRelayHopFound = true
		}
		if strings.Contains(string(proto), "relay/0.2.0/stop") {
			isRelayStopFound = true
		}
	}
	if isAutoNatBackFound && isAutoNatRequestFound && isRelayHopFound && isRelayStopFound {
		return nil
	}
	return fmt.Errorf(
		"node: not all supported protocols: autonat/dial-back=%t, autonat/dial-request=%t, relay/hop=%t, relay/stop=%t",
		isAutoNatBackFound, isAutoNatRequestFound, isRelayHopFound, isRelayStopFound,
	)
}

const (
	relayStatusWaiting = "waiting"
	relayStatusRunning = "running"
)

func (n *WarpNode) NodeInfo() warpnet.NodeInfo {
	if n == nil || n.node == nil || n.node.Network() == nil || n.node.Peerstore() == nil {
		return warpnet.NodeInfo{}
	}
	relayState := relayStatusWaiting

	addrs := n.node.Peerstore().Addrs(n.node.ID())
	addresses := make([]string, 0, len(addrs))
	for _, ma := range addrs {
		if !warpnet.IsPublicMultiAddress(ma) {
			continue
		}
		if warpnet.IsRelayMultiaddress(ma) {
			relayState = relayStatusRunning
		}
		addresses = append(addresses, ma.String())
	}

	return warpnet.NodeInfo{
		ID:         n.node.ID(),
		Addresses:  addresses,
		Version:    n.version,
		OwnerId:    n.ownerId,
		StartTime:  n.startTime,
		RelayState: relayState,
	}
}

func (n *WarpNode) Node() warpnet.P2PNode {
	if n == nil || n.node == nil {
		return nil
	}
	return n.node
}

func (n *WarpNode) Peerstore() warpnet.WarpPeerstore {
	if n == nil || n.node == nil {
		return nil
	}
	return n.node.Peerstore()
}

func (n *WarpNode) Network() warpnet.WarpNetwork {
	if n == nil || n.node == nil {
		return nil
	}
	return n.node.Network()
}

func (n *WarpNode) Mux() warpnet.WarpProtocolSwitch {
	return n.node.Mux()
}

const ErrSelfRequest = warpnet.WarpError("self request is not allowed")

func (n *WarpNode) Stream(nodeId warpnet.WarpPeerID, path stream.WarpRoute, data any) (_ []byte, err error) {
	if n == nil || n.streamer == nil {
		return nil, warpnet.WarpError("node is not initialized")
	}
	if nodeId == "" {
		return nil, warpnet.WarpError("node: empty node id")
	}
	if n.NodeInfo().ID == nodeId {
		return nil, ErrSelfRequest
	}

	peerInfo := n.Peerstore().PeerInfo(nodeId)
	if len(peerInfo.Addrs) == 0 {
		log.Warningf("node %v is offline", nodeId)
		return nil, warpnet.ErrNodeIsOffline
	}

	var bt []byte
	if data != nil {
		var ok bool
		bt, ok = data.([]byte)
		if !ok {
			bt, err = json.JSON.Marshal(data)
			if err != nil {
				return nil, fmt.Errorf("node: generic stream: marshal data %v %s", err, data)
			}
		}
	}
	return n.streamer.Send(peerInfo, path, bt)
}

func (n *WarpNode) StopNode() {
	log.Infoln("node: shutting down node...")
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("node: recovered: %v\n", r)
		}
	}()
	if n == nil || n.node == nil {
		return
	}

	if n.relay != nil {
		_ = n.relay.Close()
	}

	if err := n.node.Close(); err != nil {
		log.Errorf("node: failed to close: %v", err)
	}
	n.isClosed.Store(true)
	n.node = nil
	return
}

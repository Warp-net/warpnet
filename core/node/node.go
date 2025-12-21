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
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/backoff"
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/relay"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/json"
	"github.com/cockroachdb/errors"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/event"
	log "github.com/sirupsen/logrus"
)

const (
	DefaultTimeout                          = 60 * time.Second
	ErrPrivateKeyRequired warpnet.WarpError = "private key is required"
)

type Streamer interface {
	Send(peerAddr warpnet.WarpAddrInfo, r stream.WarpRoute, data []byte) ([]byte, error)
}

type BackoffEnabler interface {
	IsBackoffEnabled(id warpnet.WarpPeerID) bool
	Reset(id warpnet.WarpPeerID)
}

type WarpNode struct {
	ctx      context.Context
	node     warpnet.P2PNode
	relay    warpnet.WarpRelayCloser
	streamer Streamer
	backoff  BackoffEnabler

	isClosed     *atomic.Bool
	version      *semver.Version
	reachability atomic.Int64

	startTime        time.Time
	eventsSub        event.Subscription
	mw               *middleware.WarpMiddleware
	internalHandlers map[warpnet.WarpProtocolID]warpnet.StreamHandler
}

func NewWarpNode(
	ctx context.Context,
	opts ...warpnet.WarpOption,
) (*WarpNode, error) {
	limiter := warpnet.NewConfigurableLimiter(nil) // TODO

	manager, err := warpnet.NewConnManager(limiter)
	if err != nil {
		return nil, err
	}

	rm, err := warpnet.NewResourceManager(limiter)
	if err != nil {
		return nil, err
	}

	managersOpts := []libp2p.Option{
		libp2p.ResourceManager(rm),
		libp2p.ConnectionManager(manager),
		libp2p.DisableMetrics(), // TODO move to settings
	}

	opts = append(opts, managersOpts...)

	node, err := warpnet.NewP2PNode(opts...)
	if err != nil {
		return nil, fmt.Errorf("node: failed to init node: %w", err)
	}

	pool, err := stream.NewStreamPool(ctx, node)
	if err != nil {
		return nil, err
	}

	sub, err := node.EventBus().Subscribe(event.WildcardSubscription)
	if err != nil {
		return nil, fmt.Errorf("node: failed to subscribe: %w", err)
	}

	relayService, err := relay.NewRelay(node)
	if err != nil {
		return nil, fmt.Errorf("node: failed to create relay: %w", err)
	}
	version := config.Config().Version

	wn := &WarpNode{
		ctx:              ctx,
		node:             node,
		relay:            relayService,
		streamer:         pool,
		isClosed:         new(atomic.Bool),
		version:          version,
		startTime:        time.Now(),
		backoff:          backoff.NewSimpleBackoff(ctx, time.Minute, 5),
		eventsSub:        sub,
		mw:               middleware.NewWarpMiddleware(node.ID()),
		internalHandlers: make(map[warpnet.WarpProtocolID]warpnet.StreamHandler),
	}

	go wn.trackIncomingEvents()
	return wn, nil
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

func (n *WarpNode) SetStreamHandlers(handlers ...warpnet.WarpStreamHandler) {
	logMw := n.mw.LoggingMiddleware
	authMw := n.mw.AuthMiddleware
	unwrapMw := n.mw.UnwrapStreamMiddleware

	for _, h := range handlers {
		streamHandler := logMw(authMw(unwrapMw(h.Handler)))

		if !h.IsValid() {
			panic(fmt.Sprintf("node: invalid stream handler: %s", h.String()))
		}
		n.node.SetStreamHandler(h.Path, streamHandler)
		n.internalHandlers[h.Path] = streamHandler
	}
}

var localAddrActions = map[int]string{
	0: "unknown",
	1: "added",
	2: "maintained",
	3: "removed",
}

func (n *WarpNode) trackIncomingEvents() {
	for {
		select {
		case <-n.ctx.Done():
			return
		case ev, ok := <-n.eventsSub.Out():
			if !ok {
				return
			}
			switch typedEvent := ev.(type) {
			case event.EvtPeerProtocolsUpdated:
				if len(typedEvent.Added) != 0 {
					log.Infof("node: event: protocol added: %v", typedEvent.Added)
				}
				if len(typedEvent.Removed) != 0 {
					log.Infof("node: event: protocol removed: %v", typedEvent.Removed)
				}
			case event.EvtLocalProtocolsUpdated:
				if len(typedEvent.Added) != 0 {
					log.Infof("node: event: protocol added: %v", typedEvent.Added)
				} else {
					log.Infof("node: event: protocol removed: %v", typedEvent.Removed)
				}
			case event.EvtPeerConnectednessChanged:
				pid := typedEvent.Peer.String()
				if typedEvent.Connectedness == warpnet.Limited {
					return
				}
				log.Infof(
					"node: event: peer ...%s connectedness updated: %s",
					pid[len(pid)-6:],
					typedEvent.Connectedness.String(),
				)

			case event.EvtPeerIdentificationFailed:
				pid := typedEvent.Peer.String()
				log.Errorf(
					"node: event: peer ...%s identification failed, reason: %s",
					pid[len(pid)-6:], typedEvent.Reason,
				)

			case event.EvtPeerIdentificationCompleted:
				pid := typedEvent.Peer.String()
				log.Debugf(
					"node: event: peer ...%s identification completed, observed address: %s",
					pid[len(pid)-6:], typedEvent.ObservedAddr.String(),
				)
			case event.EvtLocalReachabilityChanged:
				r := typedEvent.Reachability // it's int32 under the hood
				log.Infof(
					"node: event: own node reachability changed: %s",
					strings.ToLower(r.String()),
				)
				n.reachability.Store(int64(r))
			case event.EvtNATDeviceTypeChanged:
				log.Infof(
					"node: event: NAT device type changed: %s, transport: %s",
					typedEvent.NatDeviceType.String(), typedEvent.TransportProtocol.String(),
				)
			case event.EvtAutoRelayAddrsUpdated:
				if len(typedEvent.RelayAddrs) != 0 {
					log.Infoln("node: event: relay address added")
				}
			case event.EvtLocalAddressesUpdated:
				for _, addr := range typedEvent.Current {
					log.Debugf(
						"node: event: local address %s: %s",
						addr.Address.String(), localAddrActions[int(addr.Action)],
					)
				}
			case event.EvtHostReachableAddrsChanged:
				log.Infof(
					`node: event: peer reachability changed: reachable: %v, unreachable: %v, unknown: %v`,
					typedEvent.Reachable,
					typedEvent.Unreachable,
					typedEvent.Unknown,
				)
			default:
				bt, _ := json.Marshal(ev)
				log.Infof("node: event: %T %s", ev, bt)
			}
		}
	}
}

func (n *WarpNode) BaseNodeInfo() warpnet.NodeInfo {
	if n == nil || n.node == nil || n.node.Network() == nil || n.node.Peerstore() == nil {
		return warpnet.NodeInfo{}
	}

	relayState := warpnet.RelayStatusWaiting

	addrs := n.node.Peerstore().Addrs(n.node.ID())
	addresses := make([]string, 0, len(addrs))
	for _, ma := range addrs {
		if !warpnet.IsPublicMultiAddress(ma) {
			continue
		}
		if warpnet.IsRelayMultiaddress(ma) {
			relayState = warpnet.RelayStatusRunning
		}
		addresses = append(addresses, ma.String())
	}

	return warpnet.NodeInfo{
		ID:           n.node.ID(),
		Addresses:    addresses,
		Version:      n.version,
		StartTime:    n.startTime,
		RelayState:   relayState,
		Reachability: warpnet.WarpReachability(n.reachability.Load()),
		Protocols:    n.node.Mux().Protocols(),
	}
}

func (n *WarpNode) Node() warpnet.P2PNode {
	if n == nil || n.node == nil {
		return nil
	}
	return n.node
}

func (n *WarpNode) SelfStream(path stream.WarpRoute, data any) (_ []byte, err error) {
	if data == nil {
		return nil, errors.New("node: selfstream: empty data")
	}
	handler, ok := n.internalHandlers[warpnet.WarpProtocolID(path)]
	if !ok {
		return nil, errors.Errorf(
			"node: selfstream: no handler for path %s, available handlers %d \n",
			path, len(n.internalHandlers),
		)
	}

	streamClient, streamServer := stream.NewLoopbackStream(n.node.ID(), warpnet.WarpProtocolID(path))
	defer func() {
		_ = streamClient.Close()
	}()

	_ = streamServer.SetDeadline(time.Now().Add(time.Minute))
	go handler(streamServer) // handler closes server stream by itself

	bt, ok := data.([]byte)
	if !ok {
		bt, err = json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("node: selfstream: marshal data %w %s", err, data)
		}
	}

	_ = streamClient.SetDeadline(time.Now().Add(time.Minute))
	if _, err := streamClient.Write(bt); err != nil {
		return nil, err
	}

	_ = streamClient.CloseWrite()

	result, err := io.ReadAll(streamClient)
	if err != nil && !errors.Is(err, io.EOF) && errors.Is(err, io.ErrClosedPipe) {
		return result, err
	}
	return result, nil
}

const ErrSelfRequest = warpnet.WarpError("self request is not allowed")

func (n *WarpNode) Stream(nodeId warpnet.WarpPeerID, path stream.WarpRoute, data any) (_ []byte, err error) {
	if n == nil || n.streamer == nil {
		return nil, warpnet.WarpError("node is not initialized")
	}

	if n.node.ID() == nodeId {
		return nil, ErrSelfRequest
	}

	var bt []byte
	if data != nil {
		var ok bool
		bt, ok = data.([]byte)
		if !ok {
			bt, err = json.Marshal(data)
			if err != nil {
				return nil, fmt.Errorf("node: generic stream: marshal data %w %s", err, data)
			}
		}
	}

	return n.streamer.Send(n.node.Peerstore().PeerInfo(nodeId), path, bt)
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

	if n.eventsSub != nil {
		_ = n.eventsSub.Close()
	}
	log.Infoln("node: event sub closed")

	if n.relay != nil {
		_ = n.relay.Close()
	}
	log.Infoln("node: relay closed")

	if err := n.node.Close(); err != nil {
		log.Errorf("node: failed to close: %v", err)
	}
	log.Infoln("node: stopped")

	n.isClosed.Store(true)
	n.node = nil

	// pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
}

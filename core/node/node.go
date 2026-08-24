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
	"errors"
	"fmt"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"io"
	"runtime/debug"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/backoff"
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/relay"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	warpevent "github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/hashicorp/golang-lru/v2/expirable"
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
	SetStreamable(id warpnet.WarpPeerID)
	SetUnstreamable(id warpnet.WarpPeerID)
}

type OfflineOutbox interface {
	Enqueue(nodeIdStr string, route stream.WarpRoute, payload []byte)
	NotifyOnline(nodeIdStr string)
	Close()
}

type BackoffEnabler interface {
	IsBackoffEnabled(id warpnet.WarpPeerID) bool
	Reset(id warpnet.WarpPeerID)
}

type Prioritizer interface {
	SetPriority(pid warpnet.WarpPeerID, r warpnet.WarpReachability)
	SetMinPriority(pid warpnet.WarpPeerID)
	SetMaxPriority(pid warpnet.WarpPeerID)
	SetRatingPriority(pid warpnet.WarpPeerID, band rating.Band)
}

const (
	connFlapWindow    = time.Minute
	connFlapThreshold = 4
	connFlapCacheSize = 256
)

type WarpNode struct {
	ctx      context.Context
	node     warpnet.P2PNode
	relay    warpnet.WarpRelayCloser
	streamer Streamer
	outbox   OfflineOutbox
	backoff  BackoffEnabler

	isClosed *atomic.Bool
	version  *semver.Version

	reachability atomic.Int64
	prioritizer  Prioritizer

	rating   *rating.Handle
	connFlap *expirable.LRU[string, *atomic.Int64]

	startTime        time.Time
	eventsSub        event.Subscription
	middlewares      []StreamMiddleware
	internalHandlers map[warpnet.WarpProtocolID]warpnet.StreamHandler
}

// recordOffence charges an offence to a remote peer.
func (n *WarpNode) recordOffence(s warpnet.WarpStream, kind rating.Kind) {
	if n == nil || s == nil || s.Conn() == nil {
		return
	}
	remote := s.Conn().RemotePeer()
	if remote == "" || remote == s.Conn().LocalPeer() {
		return
	}
	n.rating.Record(remote, kind)
}

func NewWarpNode(
	ctx context.Context,
	ratingHandle *rating.Handle,
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

	ya := *yamux.DefaultTransport
	ya.KeepAliveInterval = 15 * time.Second
	ya.ConnectionWriteTimeout = 30 * time.Second

	managersOpts := []libp2p.Option{
		libp2p.ResourceManager(rm),
		libp2p.ConnectionManager(manager),
		libp2p.DisableMetrics(), // TODO move to settings
		libp2p.Muxer(yamux.ID, &ya),
	}

	opts = append(opts, managersOpts...)

	node, err := warpnet.NewP2PNode(opts...)
	if err != nil {
		return nil, fmt.Errorf("node: failed to init node: %w", err)
	}

	node.Network().Notify(connTracer{})

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

	if ratingHandle == nil {
		ratingHandle = rating.NewHandle()
	}
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
		internalHandlers: make(map[warpnet.WarpProtocolID]warpnet.StreamHandler),
		prioritizer:      newNodeReachabilityManager(node.ConnManager()),
		rating:           ratingHandle,
		connFlap: expirable.NewLRU[string, *atomic.Int64](
			connFlapCacheSize, nil, connFlapWindow,
		),
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

func (n *WarpNode) SetOutbox(store stream.OutboxStore) {
	if n == nil || store == nil {
		return
	}
	outbox := stream.NewOutbox(n.ctx, store)
	outbox.Run(n.streamer)
	n.outbox = outbox
}

type StreamMiddleware func(next warpnet.WarpHandlerFunc) warpnet.WarpHandlerFunc

func (n *WarpNode) SetStreamMiddlewares(mws ...StreamMiddleware) {
	if n == nil || len(mws) == 0 {
		return
	}
	n.middlewares = mws
}

func (n *WarpNode) SetStreamHandlers(handlers ...warpnet.WarpStreamHandler) {
	for _, h := range handlers {
		if !h.IsValid() {
			panic(fmt.Sprintf("node: invalid stream handler: %s", h.String()))
		}

		handler := h.Handler
		for _, mw := range slices.Backward(n.middlewares) {
			handler = mw(handler)
		}

		streamHandler := n.unwrap(handler)
		n.node.SetStreamHandler(h.Path, streamHandler)
		n.internalHandlers[h.Path] = streamHandler
	}
}

func (n *WarpNode) unwrap(handler warpnet.WarpHandlerFunc) warpnet.StreamHandler {
	return func(s warpnet.WarpStream) {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("node: unwrap: panic: %v %s", r, debug.Stack())
			}
			_ = s.Close()
		}()

		data, err := stream.ReadRequest(s)
		if errors.Is(err, stream.ErrPayloadTooLarge) {
			log.Errorf("node: unwrap: %s: %v", s.Protocol(), err)
			n.recordOffence(s, rating.KindOversizePayload)
			_ = s.Reset()
			return
		}
		if err != nil {
			log.Errorf("node: unwrap: reading from stream: %v", err)
			n.recordOffence(s, rating.KindMalformedFrame)
			_ = json.NewEncoder(s).Encode(warpevent.ResponseError{Message: middleware.ErrStreamReadError.Error()})
			return
		}

		log.Debugf(">>> STREAM REQUEST %s %s\n", string(s.Protocol()), string(data))

		response, err := handler(data, s)
		if err == nil && s.Protocol() == warpevent.PRIVATE_POST_PAIR {
			log.Debugf("node: unwrap: paired alias: %s", s.Conn().RemotePeer())
		}
		if errors.Is(err, warpnet.ErrForeignAuthor) {
			n.recordOffence(s, rating.KindForeignAuthorship)
		}

		if err != nil && !errors.Is(err, warpnet.ErrNodeIsOffline) {
			clip := data
			if len(clip) > 500 { //nolint:mnd
				clip = clip[:500]
			}
			log.Errorf("node: unwrap: handling of %s %s message: %s failed: %v\n",
				s.Protocol(), s.Conn().RemotePeer(), string(clip), err)
			response = warpevent.ResponseError{Code: middleware.InternalNodeErrorCode, Message: err.Error()}
		}

		payload, err := marshalResponse(response)
		if err != nil {
			log.Errorf("node: unwrap: encoding response: %v %v", response, err)
			return
		}

		log.Debugf("<<< STREAM RESPONSE: %s %s\n", string(s.Protocol()), string(payload))
		if len(payload) == 0 {
			return
		}
		if _, werr := s.Write(payload); werr != nil {
			log.Errorf("node: unwrap: writing response to stream: %v", werr)
		}
	}
}

func marshalResponse(response any) ([]byte, error) {
	switch typed := response.(type) {
	case nil:
		return json.Marshal(warpevent.ResponseError{Message: "empty response"})
	case []byte:
		return typed, nil
	case string:
		return []byte(typed), nil
	default:
		return json.Marshal(response)
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
				log.Infof(
					"node: event: peer ...%s connectedness updated: %s",
					pid[len(pid)-6:],
					typedEvent.Connectedness.String(),
				)
				isOnline := typedEvent.Connectedness == warpnet.Connected ||
					typedEvent.Connectedness == warpnet.Limited
				if isOnline {
					n.streamer.SetStreamable(typedEvent.Peer)
					if n.outbox != nil {
						n.outbox.NotifyOnline(pid)
					}
					n.prioritizer.SetRatingPriority(
						typedEvent.Peer, n.rating.Band(typedEvent.Peer),
					)
				}
				n.trackConnectionFlap(typedEvent.Peer)
			case event.EvtPeerIdentificationFailed:
				pid := typedEvent.Peer
				addrs := n.node.Peerstore().Addrs(pid)
				// The remote refused identify or went away mid-handshake:
				// transient and out of our control, so warn instead of error.
				log.Warnf(
					"node: event: peer %s %v identification failed, reason: %s",
					pid.String(), addrs, typedEvent.Reason,
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

func (n *WarpNode) trackConnectionFlap(pid warpnet.WarpPeerID) {
	if n == nil || n.connFlap == nil {
		return
	}
	key := pid.String()
	counter, ok := n.connFlap.Get(key)
	if !ok {
		counter = new(atomic.Int64)
		n.connFlap.Add(key, counter)
	}
	if counter.Add(1) == connFlapThreshold {
		// Once per window: the entry expires and starts over.
		n.rating.Record(pid, rating.KindConnectionFlap)
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

func (n *WarpNode) Prioritizer() Prioritizer {
	return n.prioritizer
}

// importStreamDeadline is the loopback-stream I/O deadline for the Twitter
// archive import route, which parses and stores a whole archive and needs
// far longer than the default one-minute self-stream budget.
const importStreamDeadline = 10 * time.Minute

func (n *WarpNode) SelfStream(
	from, to warpnet.WarpPeerID, path stream.WarpRoute, data any,
) (_ []byte, err error) {
	if data == nil {
		return nil, fmt.Errorf("node: selfstream: empty data") //nolint:err113
	}
	handler, ok := n.internalHandlers[warpnet.WarpProtocolID(path)]
	if !ok {
		return nil, fmt.Errorf( //nolint:err113
			"node: selfstream: no handler for path %s, available handlers %d",
			path, len(n.internalHandlers),
		)
	}

	bt, ok := data.([]byte)
	if !ok {
		bt, err = json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("node: selfstream: marshal data %w %s", err, data)
		}
	}

	streamClient, streamServer := stream.NewLoopbackStream(to, from, warpnet.WarpProtocolID(path))
	defer func() {
		_ = streamClient.Close()
	}()

	deadline := time.Minute
	if string(path) == warpevent.PRIVATE_POST_IMPORT_TWITTER_TWEET {
		deadline = importStreamDeadline
	}

	_ = streamServer.SetDeadline(time.Now().Add(deadline))
	go handler(streamServer) // handler closes server stream by itself

	_ = streamClient.SetDeadline(time.Now().Add(deadline))
	if _, err := streamClient.Write(bt); err != nil {
		return nil, err
	}

	_ = streamClient.CloseWrite()

	result, err := io.ReadAll(streamClient)
	if !isBenignStreamCloseErr(err) {
		return result, err
	}
	return result, nil
}

// isBenignStreamCloseErr reports whether err returned from reading a stream is a
// normal end-of-stream signal rather than a real I/O failure. io.ReadAll never
// surfaces io.EOF, but a loopback stream may return io.ErrClosedPipe once the
// peer half is closed; in both cases the response has already been read in full.
func isBenignStreamCloseErr(err error) bool {
	return err == nil ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrClosedPipe)
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

	resp, err := n.streamer.Send(n.node.Peerstore().PeerInfo(nodeId), path, bt)
	if n.outbox != nil && errors.Is(err, warpnet.ErrNodeIsOffline) {
		n.outbox.Enqueue(nodeId.String(), path, bt)
	}
	return resp, err
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

	if n.outbox != nil {
		n.outbox.Close()
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

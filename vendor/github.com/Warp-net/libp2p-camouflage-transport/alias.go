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

WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package camouflage

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/Warp-net/libp2p-camouflage-transport/aliasresolver"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/transport"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

// P_WARPID is the multiaddr protocol code for /warpid/<hex>.
const P_WARPID = 0x0300

const WarpIDName = "warpid"
const WarpIDByteLen = 32

var AliasDialTimeout = 30 * time.Second

var errInvalidWarpID = errors.New("warpid: invalid value")

func init() {
	byCode := ma.ProtocolWithCode(P_WARPID)
	byName := ma.ProtocolWithName(WarpIDName)
	if byCode.Code != 0 && byCode.Name != WarpIDName {
		panic(fmt.Sprintf("camouflage/alias: P_WARPID (%#x) already registered as %q", P_WARPID, byCode.Name))
	}
	if byName.Code != 0 && byName.Code != P_WARPID {
		panic(fmt.Sprintf("camouflage/alias: protocol name %q already registered with code %#x", WarpIDName, byName.Code))
	}
	if byCode.Code == P_WARPID {
		return
	}
	if err := ma.AddProtocol(ma.Protocol{
		Name:  WarpIDName,
		Code:  P_WARPID,
		VCode: ma.CodeToVarint(P_WARPID),
		Size:  WarpIDByteLen * 8,
		Transcoder: ma.NewTranscoderFromFunctions(
			warpIDStrToBytes, warpIDBytesToStr, warpIDValidate,
		),
	}); err != nil {
		panic(fmt.Sprintf("camouflage/alias: register /warpid/: %v", err))
	}
}

func warpIDStrToBytes(s string) ([]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errInvalidWarpID, err)
	}
	if len(b) != WarpIDByteLen {
		return nil, fmt.Errorf("%w: expected %d bytes, got %d", errInvalidWarpID, WarpIDByteLen, len(b))
	}
	return b, nil
}

func warpIDBytesToStr(b []byte) (string, error) {
	if len(b) != WarpIDByteLen {
		return "", fmt.Errorf("%w: expected %d bytes, got %d", errInvalidWarpID, WarpIDByteLen, len(b))
	}
	return hex.EncodeToString(b), nil
}

func warpIDValidate(b []byte) error {
	if len(b) != WarpIDByteLen {
		return fmt.Errorf("%w: expected %d bytes, got %d", errInvalidWarpID, WarpIDByteLen, len(b))
	}
	return nil
}

func hasWarpID(a ma.Multiaddr) bool {
	_, err := a.ValueForProtocol(P_WARPID)
	return err == nil
}

type spoofStreamFn func(s network.Stream, local, remote ma.Multiaddr) *SpoofConn
type wrapStreamFn func(s network.Stream, local, remote ma.Multiaddr, isClient bool) (manet.Conn, error)

type aliasMode struct {
	host     host.Host
	privKey  crypto.PrivKey // nil for dial-only nodes
	warpID   string         // empty => dial-only
	upgrader transport.Upgrader

	spoofStream spoofStreamFn
	wrapStream  wrapStreamFn

	mu        sync.Mutex
	listeners map[peer.ID]*streamListener // keyed by relay peer

	finderCtx    context.Context
	finderCancel context.CancelFunc
}

func newAliasMode(h host.Host, upgrader transport.Upgrader, warpID string, spoof spoofStreamFn, wrap wrapStreamFn) *aliasMode {
	a := &aliasMode{
		host:        h,
		privKey:     h.Peerstore().PrivKey(h.ID()),
		warpID:      warpID,
		upgrader:    upgrader,
		spoofStream: spoof,
		wrapStream:  wrap,
		listeners:   make(map[peer.ID]*streamListener),
	}
	h.SetStreamHandler(aliasresolver.StopProtocol, a.handleStop)
	h.Network().Notify(a)

	if warpID != "" {
		a.finderCtx, a.finderCancel = context.WithCancel(context.Background())
		go a.runRelayFinder()
	}
	return a
}

func (a *aliasMode) canDial(addr ma.Multiaddr) bool {
	_, _, _, err := splitAliasDialAddr(addr)
	return err == nil
}

func (a *aliasMode) dial(ctx context.Context, t transport.Transport, raddr ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	relayID, warpID, target, err := splitAliasDialAddr(raddr)
	if err != nil {
		return nil, err
	}
	if target != "" && target != p {
		return nil, fmt.Errorf("camouflage/alias: dial multiaddr target %s != peer arg %s", target, p)
	}

	// usefd=false: alias rides on an existing libp2p stream on an
	// existing TCP conn to the relay — no new FD. Passing true would
	// phantom-charge the ResourceManager's system.fd budget.
	scope, err := a.host.Network().ResourceManager().OpenConnection(network.DirOutbound, false, raddr)
	if err != nil {
		return nil, err
	}
	if err := scope.SetPeer(p); err != nil {
		scope.Done()
		return nil, err
	}

	camouflaged, err := a.openResolveStream(ctx, raddr, relayID, warpID)
	if err != nil {
		scope.Done()
		return nil, err
	}

	cc, err := a.upgrader.Upgrade(ctx, t, camouflaged, network.DirOutbound, p, scope)
	if err != nil {
		_ = camouflaged.Close()
		scope.Done()
		return nil, err
	}
	return cc, nil
}

func (a *aliasMode) openResolveStream(ctx context.Context, raddr ma.Multiaddr, relayID peer.ID, warpID string) (manet.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, AliasDialTimeout)
	defer cancel()

	s, err := a.host.NewStream(dialCtx, relayID, aliasresolver.ResolveProtocol)
	if err != nil {
		return nil, fmt.Errorf("camouflage/alias: open resolve stream to relay %s: %w", relayID, err)
	}
	if deadline, ok := dialCtx.Deadline(); ok {
		_ = s.SetDeadline(deadline)
	}

	if err := aliasresolver.WriteResolveFrame(s, warpID); err != nil {
		_ = s.Reset()
		return nil, fmt.Errorf("camouflage/alias: write resolve request: %w", err)
	}
	ok, err := aliasresolver.ReadStatus(s)
	if err != nil {
		_ = s.Reset()
		return nil, fmt.Errorf("camouflage/alias: read resolve status: %w", err)
	}
	if !ok {
		_ = s.Reset()
		return nil, fmt.Errorf("camouflage/alias: relay refused resolve for %s", warpID)
	}

	_ = s.SetDeadline(time.Time{})

	// Dial-only nodes (no warpID) borrow the target's WarpID for the
	// local label so the resulting multiaddr is well-formed.
	localID := a.warpID
	if localID == "" {
		localID = warpID
	}
	local := buildAliasMultiaddr(relayID, localID)
	camouflaged, err := a.wrapStream(s, local, raddr, true)
	if err != nil {
		_ = s.Reset()
		return nil, fmt.Errorf("camouflage/alias: TLS camouflage: %w", err)
	}
	return camouflaged, nil
}

func (a *aliasMode) prepareListener(laddr ma.Multiaddr) (manet.Listener, error) {
	if a.warpID == "" {
		return nil, errors.New("camouflage/alias: cannot listen — no WarpID configured")
	}
	if a.privKey == nil {
		return nil, errors.New("camouflage/alias: cannot listen — host private key unavailable")
	}

	relayID, warpID, err := splitAliasListenAddr(laddr)
	if err != nil {
		return nil, err
	}
	if warpID != a.warpID {
		return nil, fmt.Errorf("camouflage/alias: listen warpID %s != configured %s", warpID, a.warpID)
	}

	listenAddr := buildAliasMultiaddr(relayID, warpID)

	// onClose must capture the listener instance to verify it's still
	// the active one — otherwise a stale Close could evict a successor
	// from prepareListener-after-restart.
	var sl *streamListener
	sl = newStreamListener(relayID, listenAddr, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if cur, ok := a.listeners[relayID]; ok && cur == sl {
			delete(a.listeners, relayID)
		}
	})

	a.mu.Lock()
	if _, exists := a.listeners[relayID]; exists {
		a.mu.Unlock()
		return nil, fmt.Errorf("camouflage/alias: already listening via relay %s", relayID)
	}
	a.listeners[relayID] = sl
	a.mu.Unlock()

	if err := a.registerOnRelay(context.Background(), relayID); err != nil {
		_ = sl.Close()
		return nil, err
	}
	return sl, nil
}

func (a *aliasMode) registerOnRelay(ctx context.Context, relayID peer.ID) error {
	dialCtx, cancel := context.WithTimeout(ctx, AliasDialTimeout)
	defer cancel()

	s, err := a.host.NewStream(dialCtx, relayID, aliasresolver.RegisterProtocol)
	if err != nil {
		return fmt.Errorf("camouflage/alias: open register stream to relay %s: %w", relayID, err)
	}
	defer s.Close()

	if deadline, ok := dialCtx.Deadline(); ok {
		_ = s.SetDeadline(deadline)
	}

	// Sign raw bytes so different hex encodings (case) never break verification.
	idBytes, err := hex.DecodeString(a.warpID)
	if err != nil {
		_ = s.Reset()
		return fmt.Errorf("camouflage/alias: warpID hex decode: %w", err)
	}
	sig, err := a.privKey.Sign(idBytes)
	if err != nil {
		_ = s.Reset()
		return fmt.Errorf("camouflage/alias: sign warpID: %w", err)
	}
	if err := aliasresolver.WriteRegisterFrame(s, a.warpID, sig); err != nil {
		_ = s.Reset()
		return fmt.Errorf("camouflage/alias: write register request: %w", err)
	}

	ok, err := aliasresolver.ReadStatus(s)
	if err != nil {
		return fmt.Errorf("camouflage/alias: read register status: %w", err)
	}
	if !ok {
		return errors.New("camouflage/alias: relay refused register")
	}
	return nil
}

func (a *aliasMode) handleStop(s network.Stream) {
	relay := s.Conn().RemotePeer()

	a.mu.Lock()
	l := a.listeners[relay]
	a.mu.Unlock()
	if l == nil {
		log.Printf("camouflage/alias: stop stream from unregistered relay %s", relay)
		_ = s.Reset()
		return
	}
	// Distinct remote per relay so ResourceManager scopes don't collapse.
	remote := s.Conn().RemoteMultiaddr()
	if remote == nil {
		remote = l.addr
	}
	if !l.deliver(a.spoofStream(s, l.addr, remote)) {
		_ = s.Reset()
	}
}

func (a *aliasMode) runRelayFinder() {
	sub, err := a.host.EventBus().Subscribe(new(event.EvtPeerIdentificationCompleted))
	if err != nil {
		log.Printf("camouflage/alias: cannot subscribe to identify events: %v", err)
		return
	}
	defer sub.Close()

	for {
		select {
		case <-a.finderCtx.Done():
			return
		case e, ok := <-sub.Out():
			if !ok {
				return
			}
			evt, ok := e.(event.EvtPeerIdentificationCompleted)
			if !ok {
				continue
			}
			log.Printf("camouflage/alias: identify from %s, %d protocols, register=%v",
				evt.Peer, len(evt.Protocols), supportsRegisterProtocol(evt.Protocols))
			if !supportsRegisterProtocol(evt.Protocols) {
				continue
			}
			// Limited (circuit-v2) reservations have data/duration
			// caps that would just time out a long-lived register stream.
			if evt.Conn != nil && evt.Conn.Stat().Limited {
				continue
			}
			if a.host.Network().Connectedness(evt.Peer) != network.Connected {
				continue
			}
			a.maybeAutoListen(evt.Peer)
		}
	}
}

func supportsRegisterProtocol(protos []protocol.ID) bool {
	for _, p := range protos {
		if p == aliasresolver.RegisterProtocol {
			return true
		}
	}
	return false
}

const (
	autoListenMaxAttempts    = 5
	autoListenInitialBackoff = 2 * time.Second
)

func (a *aliasMode) maybeAutoListen(relay peer.ID) {
	a.mu.Lock()
	_, exists := a.listeners[relay]
	a.mu.Unlock()
	if exists {
		return
	}
	listenAddr := buildAliasMultiaddr(relay, a.warpID)
	if err := a.host.Network().Listen(listenAddr); err != nil {
		log.Printf("camouflage/alias: auto-listen via %s: %v", relay, err)
		go a.retryAutoListen(relay, listenAddr, 1)
	}
}

func (a *aliasMode) retryAutoListen(relay peer.ID, addr ma.Multiaddr, attempt int) {
	if attempt > autoListenMaxAttempts {
		return
	}
	backoff := autoListenInitialBackoff << (attempt - 1)
	select {
	case <-a.finderCtx.Done():
		return
	case <-time.After(backoff):
	}
	if a.host.Network().Connectedness(relay) != network.Connected {
		return
	}
	a.mu.Lock()
	_, exists := a.listeners[relay]
	a.mu.Unlock()
	if exists {
		return
	}
	if err := a.host.Network().Listen(addr); err != nil {
		log.Printf("camouflage/alias: auto-listen retry %d via %s: %v", attempt, relay, err)
		go a.retryAutoListen(relay, addr, attempt+1)
	}
}

func (a *aliasMode) stop() {
	if a.finderCancel != nil {
		a.finderCancel()
	}
	a.host.Network().StopNotify(a)
}

var _ network.Notifiee = (*aliasMode)(nil)

func (a *aliasMode) Listen(_ network.Network, _ ma.Multiaddr)      {}
func (a *aliasMode) ListenClose(_ network.Network, _ ma.Multiaddr) {}
func (a *aliasMode) Connected(_ network.Network, _ network.Conn)   {}

func (a *aliasMode) Disconnected(n network.Network, c network.Conn) {
	p := c.RemotePeer()
	if len(n.ConnsToPeer(p)) > 0 {
		return
	}
	a.mu.Lock()
	l := a.listeners[p]
	a.mu.Unlock()
	if l != nil {
		_ = l.Close()
	}
}

// splitAliasDialAddr parses .../p2p/<relay>/warpid/<id>[/p2p/<target>].
func splitAliasDialAddr(a ma.Multiaddr) (peer.ID, string, peer.ID, error) {
	warpComp, tail, err := splitOnWarpID(a)
	if err != nil {
		return "", "", "", err
	}
	prefix, _ := ma.SplitFunc(a, func(c ma.Component) bool {
		return c.Protocol().Code == P_WARPID
	})
	if prefix == nil {
		return "", "", "", fmt.Errorf("camouflage/alias: missing relay before /warpid/ in %s", a)
	}
	relayIDStr, err := prefix.ValueForProtocol(ma.P_P2P)
	if err != nil {
		return "", "", "", fmt.Errorf("camouflage/alias: no /p2p/<relayID> in %s", a)
	}
	relayID, err := peer.Decode(relayIDStr)
	if err != nil {
		return "", "", "", fmt.Errorf("camouflage/alias: invalid relay peer id %q: %w", relayIDStr, err)
	}

	var target peer.ID
	if tail != nil {
		comps := ma.Split(tail)
		if len(comps) != 1 || comps[0].Protocols()[0].Code != ma.P_P2P {
			return "", "", "", fmt.Errorf("camouflage/alias: trailing components after /warpid/ must be a single /p2p/<id>, got %s", tail)
		}
		targetStr, err := comps[0].ValueForProtocol(ma.P_P2P)
		if err != nil {
			return "", "", "", fmt.Errorf("camouflage/alias: malformed /p2p/ tail in %s: %w", a, err)
		}
		target, err = peer.Decode(targetStr)
		if err != nil {
			return "", "", "", fmt.Errorf("camouflage/alias: invalid target peer id %q: %w", targetStr, err)
		}
	}
	return relayID, warpComp.Value(), target, nil
}

// splitAliasListenAddr accepts /p2p/<relay>/warpid/<id> only.
func splitAliasListenAddr(a ma.Multiaddr) (peer.ID, string, error) {
	warpComp, tail, err := splitOnWarpID(a)
	if err != nil {
		return "", "", err
	}
	if tail != nil {
		return "", "", fmt.Errorf("camouflage/alias: listen address must end at /warpid/, got %s", a)
	}
	prefix, _ := ma.SplitFunc(a, func(c ma.Component) bool {
		return c.Protocol().Code == P_WARPID
	})
	if prefix == nil {
		return "", "", fmt.Errorf("camouflage/alias: missing relay before /warpid/ in %s", a)
	}
	relayIDStr, err := prefix.ValueForProtocol(ma.P_P2P)
	if err != nil {
		return "", "", fmt.Errorf("camouflage/alias: no /p2p/<relayID> in %s", a)
	}
	relayID, err := peer.Decode(relayIDStr)
	if err != nil {
		return "", "", fmt.Errorf("camouflage/alias: invalid relay peer id %q: %w", relayIDStr, err)
	}
	return relayID, warpComp.Value(), nil
}

func splitOnWarpID(a ma.Multiaddr) (*ma.Component, ma.Multiaddr, error) {
	var warpComp *ma.Component
	var tail ma.Multiaddr
	var sawWarp bool
	ma.ForEach(a, func(c ma.Component) bool {
		if sawWarp {
			if tail == nil {
				tail = c.Multiaddr()
			} else {
				tail = ma.Join(tail, c.Multiaddr())
			}
			return true
		}
		if c.Protocol().Code == P_WARPID {
			cc := c
			warpComp = &cc
			sawWarp = true
		}
		return true
	})
	if warpComp == nil {
		return nil, nil, fmt.Errorf("camouflage/alias: address %s has no /warpid/ component", a)
	}
	return warpComp, tail, nil
}

// buildAliasMultiaddr panics on bad input — callers pre-validate.
func buildAliasMultiaddr(relayID peer.ID, warpID string) ma.Multiaddr {
	relay, err := ma.NewComponent("p2p", relayID.String())
	if err != nil {
		panic(fmt.Sprintf("camouflage/alias: bad relay peer id %q: %v", relayID, err))
	}
	wid, err := ma.NewComponent(WarpIDName, warpID)
	if err != nil {
		panic(fmt.Sprintf("camouflage/alias: bad warp id %q: %v", warpID, err))
	}
	return ma.Join(relay.Multiaddr(), wid.Multiaddr())
}

type streamListener struct {
	relayID peer.ID
	addr    ma.Multiaddr
	onClose func()

	incoming chan manet.Conn

	closeMu  sync.Mutex
	isClosed bool
	closed   chan struct{}
}

var _ manet.Listener = (*streamListener)(nil)

func newStreamListener(relayID peer.ID, addr ma.Multiaddr, onClose func()) *streamListener {
	return &streamListener{
		relayID:  relayID,
		addr:     addr,
		onClose:  onClose,
		incoming: make(chan manet.Conn, 16),
		closed:   make(chan struct{}),
	}
}

// deliver: closeMu guards the closed-check against the channel send so
// a concurrent Close cannot accept a value after draining.
func (l *streamListener) deliver(c manet.Conn) bool {
	l.closeMu.Lock()
	defer l.closeMu.Unlock()
	if l.isClosed {
		return false
	}
	select {
	case l.incoming <- c:
		return true
	default:
		return false
	}
}

func (l *streamListener) Accept() (manet.Conn, error) {
	select {
	case c := <-l.incoming:
		// Close may have flipped isClosed between deliver enqueuing
		// and us reading. Without this post-read check Go's select
		// could hand a caller a conn after the listener was closed.
		l.closeMu.Lock()
		closed := l.isClosed
		l.closeMu.Unlock()
		if closed {
			_ = c.Close()
			return nil, net.ErrClosed
		}
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *streamListener) Close() error {
	l.closeMu.Lock()
	if l.isClosed {
		l.closeMu.Unlock()
		return nil
	}
	l.isClosed = true
	close(l.closed)
	l.closeMu.Unlock()

	// onClose may take aliasMode.mu, so call it without closeMu held.
	if l.onClose != nil {
		l.onClose()
	}
	for {
		select {
		case c := <-l.incoming:
			_ = c.Close()
		default:
			return nil
		}
	}
}

func (l *streamListener) Multiaddr() ma.Multiaddr { return l.addr }
func (l *streamListener) Addr() net.Addr          { return spoofAddr(l.addr.String()) }

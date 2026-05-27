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

// alias.go: IP-hiding layer on top of CamouflageTransport. aliasMode owns
// every piece of alias state (host, key, mutex, listener); the transport
// only delegates Dial/Listen/CanDial for /warpid/ multiaddrs to it. The
// alias layer in turn never reaches into the transport's fields — they
// communicate via method arguments only.

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

// P_WARPID is the multiaddr protocol code for the /warpid/<hex> component.
// 0x0300 sits in the private-use range and does not collide with any
// upstream multicodec entry as of go-multiaddr v0.16.
const P_WARPID = 0x0300

// WarpIDName is the textual protocol name (`/warpid/...`).
const WarpIDName = "warpid"

// WarpIDByteLen is the fixed length of a decoded WarpID. The textual form
// is hex-encoded, so the string is always WarpIDByteLen*2 characters.
const WarpIDByteLen = 32

// AliasDialTimeout caps how long a single resolve handshake (open stream
// to relay, send id, read status) may take before being aborted.
var AliasDialTimeout = 30 * time.Second

var errInvalidWarpID = errors.New("warpid: invalid value")

// init registers the /warpid/ multiaddr protocol. If the code/name is
// already taken by something else we panic so the misconfiguration is
// caught at startup rather than producing mis-parsed multiaddrs later.
func init() {
	if existing := ma.ProtocolWithCode(P_WARPID); existing.Code != 0 && existing.Name != WarpIDName {
		panic(fmt.Sprintf("camouflage/alias: P_WARPID (%#x) already registered as %q", P_WARPID, existing.Name))
	}
	if existing := ma.ProtocolWithName(WarpIDName); existing.Code != 0 && existing.Code != P_WARPID {
		panic(fmt.Sprintf("camouflage/alias: protocol name %q already registered with code %#x", WarpIDName, existing.Code))
	}
	if existing := ma.ProtocolWithCode(P_WARPID); existing.Code == P_WARPID {
		// Same package linked twice (e.g. via plugins). Nothing to do.
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

// hasWarpID reports whether the multiaddr contains a /warpid/ component.
func hasWarpID(a ma.Multiaddr) bool {
	_, err := a.ValueForProtocol(P_WARPID)
	return err == nil
}

// ===========================================================================
// aliasMode: the IP-hiding layer. Owns its own host reference, key, and
// listener state. The host transport interacts with it through the four
// exported methods below (dial, listen, canDial, plus the constructor).
// ===========================================================================

type aliasMode struct {
	host     host.Host
	privKey  crypto.PrivKey // may be nil; dial-only nodes don't need it
	warpID   string         // empty => dial-only (cannot listen)
	upgrader transport.Upgrader

	mu        sync.Mutex
	listeners map[peer.ID]*aliasedListener // keyed by relay peer

	finderCtx    context.Context
	finderCancel context.CancelFunc
}

// newAliasMode wires the alias layer onto a host. It installs the stop
// stream handler immediately so even a dial-only node can later flip
// into listening on a /warpid/ multiaddr. When warpID is non-empty it
// also starts a background relay-finder that auto-Listens via every
// peer it discovers speaking aliasresolver.RegisterProtocol — same
// shape as libp2p's autorelay for circuit-v2.
func newAliasMode(h host.Host, upgrader transport.Upgrader, warpID string) *aliasMode {
	a := &aliasMode{
		host:      h,
		privKey:   h.Peerstore().PrivKey(h.ID()),
		warpID:    warpID,
		upgrader:  upgrader,
		listeners: make(map[peer.ID]*aliasedListener),
	}
	h.SetStreamHandler(aliasresolver.StopProtocol, a.handleStop)
	h.Network().Notify(a)

	if warpID != "" {
		a.finderCtx, a.finderCancel = context.WithCancel(context.Background())
		go a.runRelayFinder()
	}
	return a
}

// canDial returns true only for well-formed alias dial multiaddrs.
func (a *aliasMode) canDial(addr ma.Multiaddr) bool {
	_, _, _, err := splitAliasDialAddr(addr)
	return err == nil
}

// dial performs the relay-mediated resolve and returns an upgraded
// libp2p connection to the target peer. The transport argument is used
// only to satisfy the upgrader's transport.Transport parameter — the
// alias layer never reads from it.
func (a *aliasMode) dial(ctx context.Context, t transport.Transport, raddr ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	relayID, warpID, target, err := splitAliasDialAddr(raddr)
	if err != nil {
		return nil, err
	}
	// If the multiaddr embeds a /p2p/<target>, reject any mismatch with
	// the caller's `p` early — letting it slip through would surface as
	// an opaque upgrader/auth error several layers down.
	if target != "" && target != p {
		return nil, fmt.Errorf("camouflage/alias: dial multiaddr target %s != peer arg %s", target, p)
	}

	scope, err := a.host.Network().ResourceManager().OpenConnection(network.DirOutbound, false, raddr)
	if err != nil {
		return nil, err
	}
	if err := scope.SetPeer(p); err != nil {
		scope.Done()
		return nil, err
	}

	conn, err := a.openResolveStream(ctx, raddr, relayID, warpID)
	if err != nil {
		scope.Done()
		return nil, err
	}

	cc, err := a.upgrader.Upgrade(ctx, t, conn, network.DirOutbound, p, scope)
	if err != nil {
		_ = conn.Close()
		scope.Done()
		return nil, err
	}
	return cc, nil
}

func (a *aliasMode) openResolveStream(ctx context.Context, raddr ma.Multiaddr, relayID peer.ID, warpID string) (*aliasStreamConn, error) {
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

	// Clear the handshake deadline before handing the stream off to
	// the upgrader, which will run its own Noise handshake.
	_ = s.SetDeadline(time.Time{})

	// Pick a non-empty label for the local multiaddr. Dial-only nodes
	// (no WithWarpID) borrow the target's WarpID just so the resulting
	// multiaddr is well-formed; identify still publishes the empty set
	// because we never listen here.
	localID := a.warpID
	if localID == "" {
		localID = warpID
	}
	local := buildAliasMultiaddr(relayID, localID)
	return newAliasStreamConn(s, local, raddr), nil
}

// listen registers this peer's WarpID on the relay encoded in laddr and
// returns a listener whose advertised address is only the alias. Many
// listeners can be active concurrently (one per relay) — that is what
// auto-discovery uses to publish redundant alias paths.
func (a *aliasMode) listen(t transport.Transport, laddr ma.Multiaddr) (transport.Listener, error) {
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

	a.mu.Lock()
	if _, exists := a.listeners[relayID]; exists {
		a.mu.Unlock()
		return nil, fmt.Errorf("camouflage/alias: already listening via relay %s", relayID)
	}
	l := newAliasedListener(a, relayID, warpID)
	a.listeners[relayID] = l
	a.mu.Unlock()

	if err := a.registerOnRelay(context.Background(), relayID); err != nil {
		a.clear(l)
		return nil, err
	}

	return a.upgrader.UpgradeGatedMaListener(t, l), nil
}

// registerOnRelay signs the WarpID with the host's private key and sends
// it on RegisterProtocol. The relay verifies the signature against the
// connection's RemotePublicKey, so the signed material need only bind
// the alias to our identity.
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

	// Sign the raw 32-byte WarpID, not its hex rendering. The resolver
	// verifies the same raw bytes after re-decoding the frame.
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

// handleStop is invoked when a relay opens an inbound stream for a
// dialer it has resolved to us. We route the stream to the listener
// registered for that relay; streams from peers we never registered
// with are dropped.
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
	if !l.deliver(s) {
		_ = s.Reset()
	}
}

// clear removes a listener from the active set if it still occupies the
// slot for its relay. Called from the listener's Close path, the listen
// failure path, and the relay-disconnect notifiee.
func (a *aliasMode) clear(l *aliasedListener) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if cur, ok := a.listeners[l.relayID]; ok && cur == l {
		delete(a.listeners, l.relayID)
	}
}

// ===========================================================================
// Auto relay-finder. Mirrors the libp2p autorelay shape for circuit-v2:
// subscribe to identify completions, and for every peer that speaks
// /warpnet/alias-register/0.0.0, automatically Listen on
// /p2p/<peer>/warpid/<warpID>. The user doesn't have to call Listen
// anywhere.
// ===========================================================================

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
			if !supportsRegisterProtocol(evt.Protocols) {
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

// maybeAutoListen calls swarm.Listen for /p2p/<relay>/warpid/<warpID>
// if we are not yet registered with this relay. Idempotent: a second
// call for the same relay is a fast no-op (listen() rejects duplicates).
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
	}
}

// stop cancels the relay finder and unhooks the notifiee. Called from
// CamouflageTransport's close path (if/when added) — currently the
// goroutine also exits when the host's event bus closes, so explicit
// stop is optional.
func (a *aliasMode) stop() {
	if a.finderCancel != nil {
		a.finderCancel()
	}
	a.host.Network().StopNotify(a)
}

// ===========================================================================
// network.Notifiee — drop the listener when its relay disconnects so the
// listeners map stays in sync with reachable relays.
// ===========================================================================

var _ network.Notifiee = (*aliasMode)(nil)

func (a *aliasMode) Listen(_ network.Network, _ ma.Multiaddr)      {}
func (a *aliasMode) ListenClose(_ network.Network, _ ma.Multiaddr) {}
func (a *aliasMode) Connected(_ network.Network, _ network.Conn)   {}

func (a *aliasMode) Disconnected(n network.Network, c network.Conn) {
	p := c.RemotePeer()
	if len(n.ConnsToPeer(p)) > 0 {
		return // still other conns alive
	}
	a.mu.Lock()
	l := a.listeners[p]
	a.mu.Unlock()
	if l != nil {
		_ = l.Close() // also calls a.clear(l)
	}
}

// ===========================================================================
// multiaddr helpers
// ===========================================================================

// splitAliasDialAddr extracts the relay peer.ID, warpID, and optional
// target peer.ID from a dial multiaddr of the form
// .../p2p/<relay>/warpid/<id>[/p2p/<target>]. Anything past /warpid/
// other than a single /p2p/<id> is rejected so CanDial cannot lure the
// swarm into picking us for a malformed address.
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

// splitAliasListenAddr accepts /p2p/<relay>/warpid/<id> (no trailing /p2p/).
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

// splitOnWarpID returns (warpComponent, tailAfterWarp, error). tail is
// nil when there is nothing after /warpid/<id>.
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

// buildAliasMultiaddr returns /p2p/<relayID>/warpid/<warpID>. Both
// inputs are pre-validated by callers (relayID parsed from a multiaddr,
// warpID checked for hex/length), so an error here means a programming
// bug — surface it loudly instead of returning nil and causing a
// nil-pointer panic far downstream.
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

// ===========================================================================
// stream conn + listener
// ===========================================================================

// aliasStreamConn wraps a libp2p network.Stream as a manet.Conn so the
// libp2p upgrader can run Noise + a stream muxer over it.
type aliasStreamConn struct {
	stream network.Stream
	local  ma.Multiaddr
	remote ma.Multiaddr
}

var _ manet.Conn = (*aliasStreamConn)(nil)

func newAliasStreamConn(s network.Stream, local, remote ma.Multiaddr) *aliasStreamConn {
	return &aliasStreamConn{stream: s, local: local, remote: remote}
}

func (c *aliasStreamConn) Read(p []byte) (int, error)         { return c.stream.Read(p) }
func (c *aliasStreamConn) Write(p []byte) (int, error)        { return c.stream.Write(p) }
func (c *aliasStreamConn) Close() error                       { return c.stream.Close() }
func (c *aliasStreamConn) LocalAddr() net.Addr                { return aliasNetAddr{label: c.local.String()} }
func (c *aliasStreamConn) RemoteAddr() net.Addr               { return aliasNetAddr{label: c.remote.String()} }
func (c *aliasStreamConn) SetDeadline(t time.Time) error      { return c.stream.SetDeadline(t) }
func (c *aliasStreamConn) SetReadDeadline(t time.Time) error  { return c.stream.SetReadDeadline(t) }
func (c *aliasStreamConn) SetWriteDeadline(t time.Time) error { return c.stream.SetWriteDeadline(t) }
func (c *aliasStreamConn) LocalMultiaddr() ma.Multiaddr       { return c.local }
func (c *aliasStreamConn) RemoteMultiaddr() ma.Multiaddr      { return c.remote }

type aliasNetAddr struct{ label string }

func (a aliasNetAddr) Network() string { return "libp2p-warpid" }
func (a aliasNetAddr) String() string  { return a.label }

// aliasedListener implements transport.GatedMaListener. The advertised
// multiaddr is /p2p/<relayID>/warpid/<warpID>; no IP ever leaves this peer.
type aliasedListener struct {
	a       *aliasMode
	relayID peer.ID
	warpID  string
	addr    ma.Multiaddr

	incoming chan network.Stream

	closeOnce sync.Once
	closed    chan struct{}

	// closeMu pairs with isClosed to serialise the "already closed?"
	// check against an enqueue on l.incoming. Without it, a race in
	// deliver's select (closed-channel ready AND buffer space ready)
	// could let Go's randomised select enqueue a stream after Close.
	closeMu  sync.Mutex
	isClosed bool
}

var _ transport.GatedMaListener = (*aliasedListener)(nil)

func newAliasedListener(a *aliasMode, relayID peer.ID, warpID string) *aliasedListener {
	return &aliasedListener{
		a:        a,
		relayID:  relayID,
		warpID:   warpID,
		addr:     buildAliasMultiaddr(relayID, warpID),
		incoming: make(chan network.Stream, 16),
		closed:   make(chan struct{}),
	}
}

// deliver hands an inbound stream to a pending Accept. Returns false if
// the listener has been closed or its queue is saturated; in either
// case the caller resets the stream so the relay isn't kept on the
// hook waiting for bytes. Holding closeMu around the closed-check and
// the send avoids the select-randomisation race where deliver could
// enqueue after Close.
func (l *aliasedListener) deliver(s network.Stream) bool {
	l.closeMu.Lock()
	defer l.closeMu.Unlock()
	if l.isClosed {
		return false
	}
	select {
	case l.incoming <- s:
		return true
	default:
		return false
	}
}

func (l *aliasedListener) Accept() (manet.Conn, network.ConnManagementScope, error) {
	for {
		select {
		case s := <-l.incoming:
			scope, err := l.a.host.Network().ResourceManager().OpenConnection(network.DirInbound, false, l.addr)
			if err != nil {
				_ = s.Reset()
				continue
			}
			return newAliasStreamConn(s, l.addr, l.addr), scope, nil
		case <-l.closed:
			return nil, nil, transport.ErrListenerClosed
		}
	}
}

func (l *aliasedListener) Close() error {
	l.closeOnce.Do(func() {
		// Flip isClosed under closeMu so any concurrent deliver
		// reads true before its enqueue attempt, then drop the lock
		// before closing the channel / draining.
		l.closeMu.Lock()
		l.isClosed = true
		l.closeMu.Unlock()

		close(l.closed)
		l.a.clear(l)
		for {
			select {
			case s := <-l.incoming:
				_ = s.Reset()
			default:
				return
			}
		}
	})
	return nil
}

func (l *aliasedListener) Multiaddr() ma.Multiaddr { return l.addr }
func (l *aliasedListener) Addr() net.Addr          { return aliasNetAddr{label: l.addr.String()} }

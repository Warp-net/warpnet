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
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Warp-net/libp2p-camouflage-transport/aliasresolver"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/transport"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/libp2p/go-libp2p/p2p/transport/tcpreuse"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"log"
)

const (
	// DefaultFragmentSize is the number of bytes per TCP segment during
	// the handshake phase. Small values (1-3) are most effective at defeating
	// DPI signature matching on the first segment.
	DefaultFragmentSize = 2

	// DefaultHandshakeLen is the number of initial bytes subject to
	// fragmentation. This covers the TLS ClientHello (~500 bytes) with margin.
	DefaultHandshakeLen = 1024

	// DefaultMaxDelay is the upper bound for the random delay inserted
	// between handshake fragments. Keeping this small avoids noticeable
	// connection latency.
	DefaultMaxDelay = 5 * time.Millisecond

	defaultConnectTimeout = 60 * time.Second

	defaultSNI              = "www.googleapis.com"
	defaultHandshakeTimeout = 10 * time.Second
	defaultBrowserChrome    = "chrome"
)

type Option func(*CamouflageTransport) error

// WithFragmentSize sets the number of bytes per TCP segment during the
// handshake phase.
func WithFragmentSize(size int) Option {
	return func(t *CamouflageTransport) error {
		if size > 0 {
			t.fragmentSize = size
		}
		return nil
	}
}

// WithHandshakeLen sets the total number of bytes subject to fragmentation.
func WithHandshakeLen(n int) Option {
	return func(t *CamouflageTransport) error {
		if n > 0 {
			t.handshakeLen = n
		}
		return nil
	}
}

// WithMaxDelay sets the upper bound for random inter-fragment delays.
// Zero disables delays; negative values are ignored.
func WithMaxDelay(d time.Duration) Option {
	return func(t *CamouflageTransport) error {
		if d >= 0 {
			t.maxDelay = d
		}
		return nil
	}
}

// WithConnectTimeout sets the TCP connect timeout.
// Non-positive values are ignored.
func WithConnectTimeout(d time.Duration) Option {
	return func(t *CamouflageTransport) error {
		if d > 0 {
			t.connectTimeout = d
		}
		return nil
	}
}

// WithSNI sets the Server Name Indication value used in the TLS
// ClientHello. Defaults to "www.googleapis.com".
func WithSNI(sni string) Option {
	return func(t *CamouflageTransport) error {
		t.sni = sni
		return nil
	}
}

// WithBrowserFingerprint selects which browser's TLS fingerprint to
// mimic. Use the Browser* constants (e.g. BrowserChrome, BrowserFirefox).
// Defaults to Chrome if empty or unknown.
func WithBrowserFingerprint(browser string) Option {
	return func(t *CamouflageTransport) error {
		t.browserFingerprint = browser
		return nil
	}
}

// WithHandshakeTimeout sets the maximum duration for the TLS handshake.
// Connections that do not complete the handshake within this window are
// closed, defending against slow-handshake active probing. Non-positive
// values are ignored.
func WithHandshakeTimeout(d time.Duration) Option {
	return func(t *CamouflageTransport) error {
		if d > 0 {
			t.handshakeTimeout = d
		}
		return nil
	}
}

// CamouflageTransport is a libp2p transport that wraps TCP connections with
// real TLS camouflage (uTLS browser fingerprint) and handshake-phase
// traffic fragmentation to evade DPI. IP-hiding alias mode (/warpid/)
// is wired on separately via EnableAlias after the host exists; this
// keeps the constructor compatible with the autonat-service dialer's
// fx graph, which has no host.Host provider.
type CamouflageTransport struct {
	inner     *tcp.TcpTransport
	upgrader  transport.Upgrader
	rcmgr     network.ResourceManager
	sharedTCP *tcpreuse.ConnMgr

	fragmentSize   int
	handshakeLen   int
	maxDelay       time.Duration
	connectTimeout time.Duration

	// TLS camouflage settings.
	sni                string
	browserFingerprint string
	handshakeTimeout   time.Duration
	camoConfig         *CamouflageConfig // built once in constructor

	// alias is nil until EnableAlias is called on the host. It owns
	// every piece of alias state; this transport never reaches into it
	// directly.
	aliasMu sync.Mutex
	alias   *aliasMode
}

var _ transport.Transport = (*CamouflageTransport)(nil)

// NewCamouflageTransport creates a DPI-evasion transport. The constructor
// signature is compatible with libp2p.Transport() dependency injection;
// it takes only the values libp2p guarantees in every fx scope it builds
// (main node, autonat dialer, ...), and notably does NOT take host.Host.
// To enable IP-hiding alias mode, call EnableAlias on the host after it
// has been constructed.
func NewCamouflageTransport(
	upgrader transport.Upgrader,
	rcmgr network.ResourceManager,
	sharedTCP *tcpreuse.ConnMgr,
	opts ...Option,
) (*CamouflageTransport, error) {
	if rcmgr == nil {
		rcmgr = &network.NullResourceManager{}
	}
	t := &CamouflageTransport{
		upgrader:           upgrader,
		rcmgr:              rcmgr,
		sharedTCP:          sharedTCP,
		fragmentSize:       DefaultFragmentSize,
		handshakeLen:       DefaultHandshakeLen,
		maxDelay:           DefaultMaxDelay,
		connectTimeout:     defaultConnectTimeout,
		sni:                defaultSNI,
		browserFingerprint: defaultBrowserChrome,
		handshakeTimeout:   defaultHandshakeTimeout,
	}
	for _, o := range opts {
		if err := o(t); err != nil {
			return nil, err
		}
	}

	inner, err := tcp.NewTCPTransport(upgrader, rcmgr, sharedTCP)
	if err != nil {
		return nil, err
	}
	t.inner = inner

	// Build the TLS camouflage configuration once. The server-side TLS
	// config (including the generated certificate chain) is reused for
	// all accepted connections.
	cfg, err := BuildCamouflageConfig(t.sni, t.browserFingerprint, t.handshakeTimeout)
	if err != nil {
		log.Printf("dpi: camouflage config build failed: %v", err)
		return nil, err
	}
	t.camoConfig = cfg


	return t, nil
}

// Dial dials the remote peer, wrapping the raw TCP connection with
// SpoofConn + real TLS camouflage before the Noise handshake. When the
// multiaddr contains a /warpid/ component, the dial is delegated to the
// alias layer.
func (t *CamouflageTransport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	if hasWarpID(raddr) {
		a := t.currentAlias()
		if a == nil {
			return nil, errors.New("camouflage/alias: /warpid/ dial requested but alias mode is not enabled (call EnableAlias)")
		}
		return a.dial(ctx, t, raddr, p)
	}
	connScope, err := t.rcmgr.OpenConnection(network.DirOutbound, true, raddr)
	if err != nil {
		log.Printf("dpi: resource manager blocked outgoing connection to %s: %v", p, err)
		return nil, err
	}

	c, err := t.dialWithScope(ctx, raddr, p, connScope)
	if err != nil {
		connScope.Done()
		return nil, err
	}
	return c, nil
}

func (t *CamouflageTransport) dialWithScope(
	ctx context.Context,
	raddr ma.Multiaddr,
	p peer.ID,
	connScope network.ConnManagementScope,
) (transport.CapableConn, error) {
	if err := connScope.SetPeer(p); err != nil {
		log.Printf("dpi: resource manager blocked connection for peer %s: %v", p, err)
		return nil, err
	}

	rawConn, err := t.dialRaw(ctx, raddr)
	if err != nil {
		return nil, err
	}

	setLinger(rawConn, 0)
	tryKeepAlive(rawConn, true)

	// Layer 1: TCP fragmentation – fragments the TLS ClientHello into
	// small TCP segments to defeat first-segment DPI.
	wrapped := t.wrapConn(rawConn)

	// Layer 2: Real TLS tunnel – uTLS presents a genuine browser
	// ClientHello fingerprint; all subsequent traffic is encrypted TLS.
	camouflaged, err := NewCamouflageConn(wrapped, true, t.camoConfig)
	if err != nil {
		log.Printf("dpi: camouflage connection failed: %v", err)
		_ = rawConn.Close()
		return nil, err
	}

	direction := network.DirOutbound
	if ok, isClient, _ := network.GetSimultaneousConnect(ctx); ok && !isClient {
		direction = network.DirInbound
	}
	return t.upgrader.Upgrade(ctx, t, camouflaged, direction, p, connScope)
}

func (t *CamouflageTransport) dialRaw(ctx context.Context, raddr ma.Multiaddr) (manet.Conn, error) {
	if t.connectTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.connectTimeout)
		defer cancel()
	}
	// When sharedTCP (tcpreuse.ConnMgr) is available, it handles reuseport
	// dialing internally. When absent, fall back to standard dialing.
	if t.sharedTCP != nil {
		return t.sharedTCP.DialContext(ctx, raddr)
	}
	var d manet.Dialer
	return d.DialContext(ctx, raddr)
}

// Listen creates a TCP listener whose accepted connections are wrapped
// with SpoofConn + real TLS camouflage so that the TLS handshake
// completes before the Noise upgrade. When the multiaddr ends in
// /warpid/<id>, the listener registers the alias on the relay encoded in
// the address prefix and advertises only that alias.
//
// When sharedTCP is available, we register as DemultiplexedConnType_TLS
// so that the tcpreuse demultiplexer routes incoming TLS ClientHello
// connections (first byte 0x16) to this transport. Without this, the
// shared port cannot dispatch connections to us.
func (t *CamouflageTransport) Listen(laddr ma.Multiaddr) (transport.Listener, error) {
	if hasWarpID(laddr) {
		a := t.currentAlias()
		if a == nil {
			return nil, errors.New("camouflage/alias: /warpid/ listen requested but alias mode is not enabled (call EnableAlias)")
		}
		return a.listen(t, laddr)
	}
	var gated transport.GatedMaListener
	if t.sharedTCP != nil {
		var err error
		gated, err = t.sharedTCP.DemultiplexedListen(laddr, tcpreuse.DemultiplexedConnType_TLS)
		if err != nil {
			return nil, err
		}
	} else {
		mal, err := manet.Listen(laddr)
		if err != nil {
			return nil, err
		}
		gated = t.upgrader.GateMaListener(mal)
	}

	camouflageList := &camouflageGatedMaListener{
		GatedMaListener: gated,
		fragmentSize:    t.fragmentSize,
		handshakeLen:    t.handshakeLen,
		maxDelay:        t.maxDelay,
		camoConfig:      t.camoConfig,
	}

	return t.upgrader.UpgradeGatedMaListener(t, camouflageList), nil
}

// currentAlias returns the active alias layer (or nil). Read under the
// mutex so EnableAlias's store is visible to concurrent Dial/Listen.
func (t *CamouflageTransport) currentAlias() *aliasMode {
	t.aliasMu.Lock()
	defer t.aliasMu.Unlock()
	return t.alias
}

// CanDial returns true if the transport can dial the given multiaddr.
// Alias addresses are delegated to the alias layer for structural
// validation; CanDial returns false until EnableAlias has been called.
func (t *CamouflageTransport) CanDial(addr ma.Multiaddr) bool {
	if hasWarpID(addr) {
		a := t.currentAlias()
		return a != nil && a.canDial(addr)
	}
	return t.inner.CanDial(addr)
}

// Protocols returns the inner TCP transport's protocols plus /warpid/.
// We claim /warpid/ unconditionally so the swarm — which calls this
// once at AddTransport time — routes alias multiaddrs to us even when
// EnableAlias has not been called yet. CanDial / Dial / Listen guard
// the runtime behavior.
func (t *CamouflageTransport) Protocols() []int {
	return append(t.inner.Protocols(), P_WARPID)
}

// Proxy returns true so the swarm prefers this transport for multiaddrs
// containing /warpid/. The TCP-only dialing/listening paths are
// unaffected: TransportForListening picks the last proxy transport along
// the multiaddr, and a plain /tcp/ addr still selects us as the only
// transport registered for the TCP protocol.
func (t *CamouflageTransport) Proxy() bool {
	return true
}

// EnableAlias wires the IP-hiding alias layer onto the CamouflageTransport
// already registered on h's swarm. After this call the transport will
// dial and listen on /p2p/<relay>/warpid/<id> multiaddrs. warpID may be
// empty for dial-only nodes (they can resolve other peers' aliases but
// cannot themselves register one). The warpID is canonicalized to
// lower-case hex so signatures, table keys and the multiaddr transcoder
// all agree.
func EnableAlias(h host.Host, warpID string) error {
	if h == nil {
		return errors.New("camouflage/alias: host is nil")
	}
	// TransportForDialing is a method on *swarm.Swarm but not part of
	// the public transport.TransportNetwork interface; assert against
	// the concrete shape we expect.
	finder, ok := h.Network().(interface {
		TransportForDialing(ma.Multiaddr) transport.Transport
	})
	if !ok {
		return fmt.Errorf("camouflage/alias: host network %T does not expose TransportForDialing", h.Network())
	}
	probe, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		return err
	}
	tr := finder.TransportForDialing(probe)
	ct, ok := tr.(*CamouflageTransport)
	if !ok {
		return fmt.Errorf("camouflage/alias: no CamouflageTransport registered on host (got %T)", tr)
	}
	return ct.enableAlias(h, warpID)
}

func (t *CamouflageTransport) enableAlias(h host.Host, warpID string) error {
	warpID = strings.ToLower(warpID)
	if warpID != "" {
		if len(warpID) != WarpIDByteLen*2 {
			return fmt.Errorf("camouflage/alias: warpID must be %d hex chars (got %d)", WarpIDByteLen*2, len(warpID))
		}
		if _, err := hex.DecodeString(warpID); err != nil {
			return fmt.Errorf("camouflage/alias: warpID must be valid hex: %w", err)
		}
	}

	t.aliasMu.Lock()
	defer t.aliasMu.Unlock()
	if t.alias != nil {
		return errors.New("camouflage/alias: already enabled")
	}
	t.alias = newAliasMode(h, t.upgrader, warpID)
	return nil
}

// EnableAliasService turns this host into an alias-resolver relay: it
// installs handlers for /warpnet/alias-register/0.0.0 and
// /warpnet/alias-resolve/0.0.0, accepts signed registrations, and
// proxies dialer streams onto registered listeners. Client peers
// running EnableAlias discover us automatically through identify and
// start Listening through us.
//
// This is the alias counterpart to libp2p.EnableRelayService for
// circuit-v2: opt in only on the nodes that should actually serve as
// alias relays (typically your bootstraps). Thin clients must not
// call this.
//
// The returned *aliasresolver.Resolver lets callers inspect the table
// or call Stop explicitly. Most setups can ignore it; the resolver's
// stream handlers go inert once the host closes.
func EnableAliasService(h host.Host) (*aliasresolver.Resolver, error) {
	if h == nil {
		return nil, errors.New("camouflage/alias: host is nil")
	}
	r := aliasresolver.New(h)
	r.Start()
	return r, nil
}

func (t *CamouflageTransport) String() string {
	return "CamouflageTCP"
}

func (t *CamouflageTransport) wrapConn(c manet.Conn) *SpoofConn {
	return NewSpoofConn(c, t.fragmentSize, t.handshakeLen, t.maxDelay)
}

type camouflageGatedMaListener struct {
	transport.GatedMaListener

	fragmentSize int
	handshakeLen int
	maxDelay     time.Duration
	camoConfig   *CamouflageConfig
}

func (l *camouflageGatedMaListener) Accept() (manet.Conn, network.ConnManagementScope, error) {
	for {
		conn, scope, err := l.GatedMaListener.Accept()
		if err != nil {
			if scope != nil {
				scope.Done()
			}

			if err != nil && strings.HasSuffix(err.Error(), "use of closed network connection") {
				return nil, nil, err
			}
			log.Printf("dpi: transient accept: %v", err)
			continue
		}

		setLinger(conn, 0)
		tryKeepAlive(conn, true)

		// Layer 1: TCP fragmentation for server-side responses.
		spoofed := NewSpoofConn(conn, l.fragmentSize, l.handshakeLen, l.maxDelay)

		// Layer 2: Real TLS tunnel – server side accepts TLS with a plausible
		// certificate chain and validates the client's ALPN.
		camouflaged, err := NewCamouflageConn(spoofed, false, l.camoConfig)
		if err != nil {
			log.Printf("dpi: camouflage handshake failed from %s: %v", conn.RemoteAddr(), err)
			if scope != nil {
				scope.Done()
			}
			_ = conn.Close()
			continue
		}

		return camouflaged, scope, nil
	}
}

func setLinger(conn net.Conn, sec int) {
	type canLinger interface {
		SetLinger(int) error
	}
	if c, ok := conn.(canLinger); ok {
		_ = c.SetLinger(sec)
	}
}

// Prefer the full TCP keepalive interface (including period) but fall
// back to just enabling keepalive if SetKeepAlivePeriod is unavailable.
type (
	fullKeepAlive interface {
		SetKeepAlive(bool) error
		SetKeepAlivePeriod(time.Duration) error
	}
	basicKeepAlive interface {
		SetKeepAlive(bool) error
	}
)

func tryKeepAlive(conn net.Conn, enabled bool) {
	if c, ok := conn.(fullKeepAlive); ok {
		if err := c.SetKeepAlive(enabled); err != nil {
			log.Printf("dpi: enabling TCP keepalive: %v", err)
			return
		}
		if !enabled {
			return
		}
		if err := c.SetKeepAlivePeriod(30 * time.Second); err != nil {
			log.Printf("dpi: setting TCP keepalive period: %v", err)
		}
		return
	}
	if c, ok := conn.(basicKeepAlive); ok {
		if err := c.SetKeepAlive(enabled); err != nil {
			log.Printf("dpi: enabling TCP keepalive (no period support): %v", err)
		}
	}
}

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
	"net"
	"strings"
	"time"

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

// WithWarpID enables IP-hiding alias mode. The transport then also
// handles /p2p/<relayID>/warpid/<id>[/p2p/<peerID>] multiaddrs: Listen
// registers the alias on the relay and publishes only that multiaddr,
// Dial resolves an alias by talking to the relay. A zero-length warpID
// leaves the transport dial-only for alias addresses (it can still reach
// other aliased peers but cannot itself listen as one).
func WithWarpID(warpID string) Option {
	return func(t *CamouflageTransport) error {
		// Lower-case so that signatures, table keys and multiaddr
		// transcoding (hex.EncodeToString is always lower-case) all
		// agree on the canonical form.
		t.warpID = strings.ToLower(warpID)
		return nil
	}
}

// CamouflageTransport is a libp2p transport that wraps TCP connections with
// real TLS camouflage (uTLS browser fingerprint) and handshake-phase
// traffic fragmentation to evade DPI. When constructed with a host (DI)
// it also owns an *aliasMode that handles /warpid/ multiaddrs on top.
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

	// warpID is a transient holder for the value supplied by
	// WithWarpID; it is consumed when alias is constructed and never
	// read again from here.
	warpID string

	// alias is non-nil whenever the DI graph supplied a host. It owns
	// every piece of alias state; this transport never reaches into it
	// directly.
	alias *aliasMode
}

var _ transport.Transport = (*CamouflageTransport)(nil)

// NewCamouflageTransport creates a DPI-evasion transport. The constructor
// signature is compatible with libp2p.Transport() dependency injection:
// the framework injects the upgrader, resource manager, shared TCP
// manager and host automatically. host is only required when alias mode
// (WithWarpID) is enabled, but accepting it here lets a single
// construction site cover both use cases.
func NewCamouflageTransport(
	upgrader transport.Upgrader,
	rcmgr network.ResourceManager,
	sharedTCP *tcpreuse.ConnMgr,
	h host.Host,
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

	// Hand the alias-relevant inputs off to the alias layer and forget
	// about them. From now on this transport only delegates to t.alias
	// when it sees a /warpid/ multiaddr.
	if h != nil {
		t.alias = newAliasMode(h, upgrader, t.warpID)
	}
	t.warpID = ""

	return t, nil
}

// Dial dials the remote peer, wrapping the raw TCP connection with
// SpoofConn + real TLS camouflage before the Noise handshake. When the
// multiaddr contains a /warpid/ component, the dial is delegated to the
// alias layer.
func (t *CamouflageTransport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	if t.alias != nil && hasWarpID(raddr) {
		return t.alias.dial(ctx, t, raddr, p)
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
	if t.alias != nil && hasWarpID(laddr) {
		return t.alias.listen(t, laddr)
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

// CanDial returns true if the transport can dial the given multiaddr.
// Alias addresses are delegated to the alias layer for structural
// validation (it only accepts well-formed /p2p/<relay>/warpid/<id>
// addresses).
func (t *CamouflageTransport) CanDial(addr ma.Multiaddr) bool {
	if hasWarpID(addr) {
		return t.alias != nil && t.alias.canDial(addr)
	}
	return t.inner.CanDial(addr)
}

// Protocols returns the set of protocols handled by this transport: the
// inner TCP transport's protocols, plus /warpid/ whenever the alias
// layer is wired up.
func (t *CamouflageTransport) Protocols() []int {
	p := t.inner.Protocols()
	if t.alias != nil {
		p = append(p, P_WARPID)
	}
	return p
}

// Proxy returns true so the swarm prefers this transport for multiaddrs
// containing /warpid/. The TCP-only dialing/listening paths are
// unaffected: TransportForListening picks the last proxy transport along
// the multiaddr, and a plain /tcp/ addr still selects us as the only
// transport registered for the TCP protocol.
func (t *CamouflageTransport) Proxy() bool {
	return true
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

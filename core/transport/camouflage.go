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

package transport

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/transport"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/libp2p/go-libp2p/p2p/transport/tcpreuse"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	log "github.com/sirupsen/logrus"
)

// Package provides a libp2p transport wrapper that defeats Deep Packet
// Inspection through two complementary techniques:
//
//  1. TLS camouflage – a real TLS tunnel is established using uTLS with a
//     genuine browser fingerprint (Chrome, Firefox, etc.) on the client side
//     and standard crypto/tls with a plausible certificate chain on the server
//     side. The Noise protocol handshake and all application data travel inside
//     this TLS tunnel, making the connection indistinguishable from normal
//     HTTPS browser traffic to DPI middleboxes.
//
//  2. TCP fragmentation – the initial bytes of the TLS ClientHello are split
//     into small TCP segments with random inter-segment delays so that
//     stateful DPI that only inspects the first segment cannot match known
//     signatures.
//
// Active probing defenses include SNI/ALPN consistency validation, a plausible
// two-certificate chain (fake CA + leaf), and configurable handshake timeouts.

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
// traffic fragmentation to evade DPI.
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
	camoConfig         *security.CamouflageConfig // built once in constructor
}

var _ transport.Transport = (*CamouflageTransport)(nil)

// NewCamouflageTransport creates a DPI-evasion transport. The constructor
// signature is compatible with libp2p.Transport() dependency injection:
// the framework injects the upgrader, resource manager, and shared TCP
// manager automatically.
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
	cfg, err := security.BuildCamouflageConfig(t.sni, t.browserFingerprint, t.handshakeTimeout)
	if err != nil {
		log.Errorf("dpi: camouflage config build failed: %v", err)
		return nil, err
	}
	t.camoConfig = cfg

	return t, nil
}

// Dial dials the remote peer, wrapping the raw TCP connection with
// SpoofConn + real TLS camouflage before the Noise handshake.
func (t *CamouflageTransport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	connScope, err := t.rcmgr.OpenConnection(network.DirOutbound, true, raddr)
	if err != nil {
		log.Errorf("dpi: resource manager blocked outgoing connection to %s: %v", p, err)
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
		log.Debugf("dpi: resource manager blocked connection for peer %s: %v", p, err)
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
	camouflaged, err := security.NewCamouflageConn(wrapped, true, t.camoConfig)
	if err != nil {
		log.Errorf("dpi: camouflage connection failed: %v", err)
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
// completes before the Noise upgrade.
//
// When sharedTCP is available, we register as DemultiplexedConnType_TLS
// so that the tcpreuse demultiplexer routes incoming TLS ClientHello
// connections (first byte 0x16) to this transport. Without this, the
// shared port cannot dispatch connections to us.
func (t *CamouflageTransport) Listen(laddr ma.Multiaddr) (transport.Listener, error) {
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
func (t *CamouflageTransport) CanDial(addr ma.Multiaddr) bool {
	return t.inner.CanDial(addr)
}

// Protocols returns the set of protocols handled by this transport.
func (t *CamouflageTransport) Protocols() []int {
	return t.inner.Protocols()
}

// Proxy always returns false.
func (t *CamouflageTransport) Proxy() bool {
	return false
}

func (t *CamouflageTransport) String() string {
	return "CamouflageTCP"
}

func (t *CamouflageTransport) wrapConn(c manet.Conn) *security.SpoofConn {
	return security.NewSpoofConn(c, t.fragmentSize, t.handshakeLen, t.maxDelay)
}

type camouflageGatedMaListener struct {
	transport.GatedMaListener

	fragmentSize int
	handshakeLen int
	maxDelay     time.Duration
	camoConfig   *security.CamouflageConfig
}

func (l *camouflageGatedMaListener) Accept() (manet.Conn, network.ConnManagementScope, error) {
	for {
		conn, scope, err := l.GatedMaListener.Accept()
		if err != nil {
			if scope != nil {
				scope.Done()
			}
			if isTransientAcceptError(err) {
				log.Debugf("dpi: transient accept error ignored: %v", err)
				continue
			}
			return nil, nil, err
		}

		setLinger(conn, 0)
		tryKeepAlive(conn, true)

		// Layer 1: TCP fragmentation for server-side responses.
		spoofed := security.NewSpoofConn(conn, l.fragmentSize, l.handshakeLen, l.maxDelay)

		// Layer 2: Real TLS tunnel – server side accepts TLS with a plausible
		// certificate chain and validates the client's ALPN.
		camouflaged, err := security.NewCamouflageConn(spoofed, false, l.camoConfig)
		if err != nil {
			log.Errorf("dpi: camouflage handshake failed from %s: %v", conn.RemoteAddr(), err)
			if scope != nil {
				scope.Done()
			}
			_ = conn.Close()
			continue
		}

		return camouflaged, scope, nil
	}
}

func isTransientAcceptError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "dpi: TLS handshake failed") ||
		strings.Contains(msg, "first record does not look like a TLS handshake")
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
			log.Errorf("error enabling TCP keepalive: %v", err)
			return
		}
		if !enabled {
			return
		}
		if err := c.SetKeepAlivePeriod(30 * time.Second); err != nil {
			log.Errorf("error setting TCP keepalive period: %v", err)
		}
		return
	}
	if c, ok := conn.(basicKeepAlive); ok {
		if err := c.SetKeepAlive(enabled); err != nil {
			log.Errorf("error enabling TCP keepalive (no period support): %v", err)
		}
	}
}

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

package dpi

import (
	"context"
	"crypto/rand"
	"io"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/transport"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/libp2p/go-libp2p/p2p/transport/tcpreuse"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	log "github.com/sirupsen/logrus"
)

// Package dpi provides a libp2p transport wrapper that defeats Deep Packet
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
)

// SpoofConn wraps a manet.Conn and transparently splits Write calls into
// small TCP segments while the connection is in the handshake phase (the
// first handshakeLen bytes). After the handshake, writes pass through
// without modification.
type SpoofConn struct {
	manet.Conn

	mu           sync.Mutex
	bytesWritten int
	fragmentSize int
	handshakeLen int
	maxDelay     time.Duration
}

// Write fragments b into small segments if the handshake phase is still
// active; otherwise it delegates directly to the underlying connection.
func (c *SpoofConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	pastHandshake := c.bytesWritten >= c.handshakeLen
	c.mu.Unlock()

	if pastHandshake {
		return c.Conn.Write(b)
	}

	return c.fragmentedWrite(b)
}

func (c *SpoofConn) fragmentedWrite(b []byte) (int, error) {
	fragSize := c.fragmentSize
	if fragSize <= 0 {
		fragSize = DefaultFragmentSize
	}

	total := 0
	for len(b) > 0 {
		c.mu.Lock()
		pastHandshake := c.bytesWritten >= c.handshakeLen
		c.mu.Unlock()

		if pastHandshake {
			// Past handshake: write-all loop for the remainder.
			for len(b) > 0 {
				n, err := c.Conn.Write(b)
				c.mu.Lock()
				c.bytesWritten += n
				c.mu.Unlock()
				total += n
				if n == 0 && err == nil {
					return total, io.ErrShortWrite
				}
				if err != nil {
					return total, err
				}
				b = b[n:]
			}
			return total, nil
		}

		size := min(fragSize, len(b))
		n, err := c.Conn.Write(b[:size])
		c.mu.Lock()
		c.bytesWritten += n
		c.mu.Unlock()
		total += n
		if n == 0 && err == nil {
			return total, io.ErrShortWrite
		}
		if err != nil {
			return total, err
		}
		b = b[n:]

		c.mu.Lock()
		stillHandshake := c.bytesWritten < c.handshakeLen
		c.mu.Unlock()
		if len(b) > 0 && stillHandshake {
			delay := randDuration(c.maxDelay)
			if delay > 0 {
				time.Sleep(delay)
			}
		}
	}
	return total, nil
}

// CloseRead forwards to the underlying connection if supported.
func (c *SpoofConn) CloseRead() error {
	if cr, ok := c.Conn.(interface{ CloseRead() error }); ok {
		return cr.CloseRead()
	}
	return nil
}

// CloseWrite forwards to the underlying connection if supported.
func (c *SpoofConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

type Option func(*SpoofTransport) error

// WithFragmentSize sets the number of bytes per TCP segment during the
// handshake phase.
func WithFragmentSize(size int) Option {
	return func(t *SpoofTransport) error {
		if size > 0 {
			t.fragmentSize = size
		}
		return nil
	}
}

// WithHandshakeLen sets the total number of bytes subject to fragmentation.
func WithHandshakeLen(n int) Option {
	return func(t *SpoofTransport) error {
		if n > 0 {
			t.handshakeLen = n
		}
		return nil
	}
}

// WithMaxDelay sets the upper bound for random inter-fragment delays.
// Zero disables delays; negative values are ignored.
func WithMaxDelay(d time.Duration) Option {
	return func(t *SpoofTransport) error {
		if d >= 0 {
			t.maxDelay = d
		}
		return nil
	}
}

// WithConnectTimeout sets the TCP connect timeout.
// Non-positive values are ignored.
func WithConnectTimeout(d time.Duration) Option {
	return func(t *SpoofTransport) error {
		if d > 0 {
			t.connectTimeout = d
		}
		return nil
	}
}

// WithSNI sets the Server Name Indication value used in the TLS
// ClientHello. Defaults to "www.googleapis.com".
func WithSNI(sni string) Option {
	return func(t *SpoofTransport) error {
		t.sni = sni
		return nil
	}
}

// WithBrowserFingerprint selects which browser's TLS fingerprint to
// mimic. Use the Browser* constants (e.g. BrowserChrome, BrowserFirefox).
// Defaults to Chrome if empty or unknown.
func WithBrowserFingerprint(browser string) Option {
	return func(t *SpoofTransport) error {
		t.browserFingerprint = browser
		return nil
	}
}

// WithHandshakeTimeout sets the maximum duration for the TLS handshake.
// Connections that do not complete the handshake within this window are
// closed, defending against slow-handshake active probing. Non-positive
// values are ignored.
func WithHandshakeTimeout(d time.Duration) Option {
	return func(t *SpoofTransport) error {
		if d > 0 {
			t.handshakeTimeout = d
		}
		return nil
	}
}

// SpoofTransport is a libp2p transport that wraps TCP connections with
// real TLS camouflage (uTLS browser fingerprint) and handshake-phase
// traffic fragmentation to evade DPI.
type SpoofTransport struct {
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
	camoConfig         *camouflageConfig // built once in constructor
}

var _ transport.Transport = (*SpoofTransport)(nil)

// NewSpoofTransport creates a DPI-evasion transport. The constructor
// signature is compatible with libp2p.Transport() dependency injection:
// the framework injects the upgrader, resource manager, and shared TCP
// manager automatically.
func NewSpoofTransport(
	upgrader transport.Upgrader,
	rcmgr network.ResourceManager,
	sharedTCP *tcpreuse.ConnMgr,
	opts ...Option,
) (*SpoofTransport, error) {
	if rcmgr == nil {
		rcmgr = &network.NullResourceManager{}
	}
	t := &SpoofTransport{
		upgrader:           upgrader,
		rcmgr:             rcmgr,
		sharedTCP:          sharedTCP,
		fragmentSize:       DefaultFragmentSize,
		handshakeLen:       DefaultHandshakeLen,
		maxDelay:           DefaultMaxDelay,
		connectTimeout:     defaultConnectTimeout,
		sni:                defaultSNI,
		browserFingerprint: BrowserChrome,
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
	cfg, err := t.buildCamouflageConfig()
	if err != nil {
		return nil, err
	}
	t.camoConfig = cfg

	return t, nil
}

// buildCamouflageConfig constructs the camouflageConfig from transport
// settings. Called once during construction.
func (t *SpoofTransport) buildCamouflageConfig() (*camouflageConfig, error) {
	cache := &certCache{}
	serverCfg, err := buildServerTLSConfig(t.sni, defaultALPNProtos, cache)
	if err != nil {
		return nil, err
	}

	return &camouflageConfig{
		sni:              t.sni,
		alpnProtos:       defaultALPNProtos,
		clientHelloID:    browserToHelloID(t.browserFingerprint),
		handshakeTimeout: t.handshakeTimeout,
		serverTLSConfig:  serverCfg,
	}, nil
}

// Dial dials the remote peer, wrapping the raw TCP connection with
// SpoofConn + real TLS camouflage before the Noise handshake.
func (t *SpoofTransport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	connScope, err := t.rcmgr.OpenConnection(network.DirOutbound, true, raddr)
	if err != nil {
		log.Debugf("dpi: resource manager blocked outgoing connection to %s: %v", p, err)
		return nil, err
	}

	c, err := t.dialWithScope(ctx, raddr, p, connScope)
	if err != nil {
		connScope.Done()
		return nil, err
	}
	return c, nil
}

func (t *SpoofTransport) dialWithScope(
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
	spoofed := t.wrapConn(rawConn)

	// Layer 2: Real TLS tunnel – uTLS presents a genuine browser
	// ClientHello fingerprint; all subsequent traffic is encrypted TLS.
	camouflaged, err := newCamouflageConn(spoofed, true, t.camoConfig)
	if err != nil {
		_ = rawConn.Close()
		return nil, err
	}

	direction := network.DirOutbound
	if ok, isClient, _ := network.GetSimultaneousConnect(ctx); ok && !isClient {
		direction = network.DirInbound
	}
	return t.upgrader.Upgrade(ctx, t, camouflaged, direction, p, connScope)
}

func (t *SpoofTransport) dialRaw(ctx context.Context, raddr ma.Multiaddr) (manet.Conn, error) {
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
// Note: we intentionally bypass sharedTCP demultiplexed listening because
// our connections start with a real TLS ClientHello (0x16...), not
// multistream select.
// Using demultiplexed conn type multistream select would
// cause incoming connections to be misclassified.
func (t *SpoofTransport) Listen(laddr ma.Multiaddr) (transport.Listener, error) {
	mal, err := manet.Listen(laddr)
	if err != nil {
		return nil, err
	}
	gated := t.upgrader.GateMaListener(mal)

	spoofList := &spoofGatedMaListener{
		GatedMaListener: gated,
		fragmentSize:    t.fragmentSize,
		handshakeLen:    t.handshakeLen,
		maxDelay:        t.maxDelay,
		camoConfig:      t.camoConfig,
	}

	return t.upgrader.UpgradeGatedMaListener(t, spoofList), nil
}

// CanDial returns true if the transport can dial the given multiaddr.
func (t *SpoofTransport) CanDial(addr ma.Multiaddr) bool {
	return t.inner.CanDial(addr)
}

// Protocols returns the set of protocols handled by this transport.
func (t *SpoofTransport) Protocols() []int {
	return t.inner.Protocols()
}

// Proxy always returns false.
func (t *SpoofTransport) Proxy() bool {
	return false
}

func (t *SpoofTransport) String() string {
	return "SpoofTCP"
}

func (t *SpoofTransport) wrapConn(c manet.Conn) *SpoofConn {
	return &SpoofConn{
		Conn:         c,
		fragmentSize: t.fragmentSize,
		handshakeLen: t.handshakeLen,
		maxDelay:     t.maxDelay,
	}
}

type spoofGatedMaListener struct {
	transport.GatedMaListener

	fragmentSize int
	handshakeLen int
	maxDelay     time.Duration
	camoConfig   *camouflageConfig
}

func (l *spoofGatedMaListener) Accept() (manet.Conn, network.ConnManagementScope, error) {
	conn, scope, err := l.GatedMaListener.Accept()
	if err != nil {
		if scope != nil {
			scope.Done()
		}
		return nil, nil, err
	}

	setLinger(conn, 0)
	tryKeepAlive(conn, true)

	// Layer 1: TCP fragmentation for server-side responses.
	spoofed := &SpoofConn{
		Conn:         conn,
		fragmentSize: l.fragmentSize,
		handshakeLen: l.handshakeLen,
		maxDelay:     l.maxDelay,
	}

	// Layer 2: Real TLS tunnel – server side accepts TLS with a plausible
	// certificate chain and validates the client's ALPN.
	camouflaged, err := newCamouflageConn(spoofed, false, l.camoConfig)
	if err != nil {
		if scope != nil {
			scope.Done()
		}
		_ = conn.Close()
		return nil, nil, err
	}

	return camouflaged, scope, nil
}

func setLinger(conn net.Conn, sec int) {
	type canLinger interface {
		SetLinger(int) error
	}
	if c, ok := conn.(canLinger); ok {
		_ = c.SetLinger(sec)
	}
}

func tryKeepAlive(conn net.Conn, enabled bool) {
	// Prefer the full TCP keepalive interface (including period) but fall
	// back to just enabling keepalive if SetKeepAlivePeriod is unavailable.
	type fullKeepAlive interface {
		SetKeepAlive(bool) error
		SetKeepAlivePeriod(time.Duration) error
	}
	if c, ok := conn.(fullKeepAlive); ok {
		if err := c.SetKeepAlive(enabled); err != nil {
			log.Debugf("error enabling TCP keepalive: %v", err)
			return
		}
		if !enabled {
			return
		}
		if err := c.SetKeepAlivePeriod(30 * time.Second); err != nil {
			log.Debugf("error setting TCP keepalive period: %v", err)
		}
		return
	}

	type basicKeepAlive interface {
		SetKeepAlive(bool) error
	}
	if c, ok := conn.(basicKeepAlive); ok {
		if err := c.SetKeepAlive(enabled); err != nil {
			log.Debugf("error enabling TCP keepalive (no period support): %v", err)
		}
	}
}

func randDuration(maximum time.Duration) time.Duration {
	if maximum <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(maximum)))
	if err != nil {
		return maximum / 2
	}
	return time.Duration(n.Int64())
}

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
	DefaultFragmentSize = 2
	DefaultHandshakeLen = 1024
	DefaultMaxDelay     = 5 * time.Millisecond

	defaultConnectTimeout   = 60 * time.Second
	defaultSNI              = "www.googleapis.com"
	defaultHandshakeTimeout = 10 * time.Second
	defaultBrowserChrome    = "chrome"
)

type Option func(*CamouflageTransport) error

func WithFragmentSize(size int) Option {
	return func(t *CamouflageTransport) error {
		if size > 0 {
			t.fragmentSize = size
		}
		return nil
	}
}

func WithHandshakeLen(n int) Option {
	return func(t *CamouflageTransport) error {
		if n > 0 {
			t.handshakeLen = n
		}
		return nil
	}
}

func WithMaxDelay(d time.Duration) Option {
	return func(t *CamouflageTransport) error {
		if d >= 0 {
			t.maxDelay = d
		}
		return nil
	}
}

func WithConnectTimeout(d time.Duration) Option {
	return func(t *CamouflageTransport) error {
		if d > 0 {
			t.connectTimeout = d
		}
		return nil
	}
}

func WithSNI(sni string) Option {
	return func(t *CamouflageTransport) error {
		t.sni = sni
		return nil
	}
}

func WithBrowserFingerprint(browser string) Option {
	return func(t *CamouflageTransport) error {
		t.browserFingerprint = browser
		return nil
	}
}

func WithHandshakeTimeout(d time.Duration) Option {
	return func(t *CamouflageTransport) error {
		if d > 0 {
			t.handshakeTimeout = d
		}
		return nil
	}
}

type CamouflageTransport struct {
	inner     *tcp.TcpTransport
	upgrader  transport.Upgrader
	rcmgr     network.ResourceManager
	sharedTCP *tcpreuse.ConnMgr

	fragmentSize   int
	handshakeLen   int
	maxDelay       time.Duration
	connectTimeout time.Duration

	sni                string
	browserFingerprint string
	handshakeTimeout   time.Duration
	camoConfig         *CamouflageConfig

	aliasMu sync.Mutex
	alias   *aliasMode
}

var _ transport.Transport = (*CamouflageTransport)(nil)

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

	cfg, err := BuildCamouflageConfig(t.sni, t.browserFingerprint, t.handshakeTimeout)
	if err != nil {
		log.Printf("dpi: camouflage config build failed: %v", err)
		return nil, err
	}
	t.camoConfig = cfg
	return t, nil
}

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

	wrapped := t.wrapConn(rawConn)
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
	if t.sharedTCP != nil {
		return t.sharedTCP.DialContext(ctx, raddr)
	}
	var d manet.Dialer
	return d.DialContext(ctx, raddr)
}

func (t *CamouflageTransport) Listen(laddr ma.Multiaddr) (transport.Listener, error) {
	gated, err := t.gateListenerFor(laddr)
	if err != nil {
		return nil, err
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

func (t *CamouflageTransport) currentAlias() *aliasMode {
	t.aliasMu.Lock()
	defer t.aliasMu.Unlock()
	return t.alias
}

func (t *CamouflageTransport) gateListenerFor(laddr ma.Multiaddr) (transport.GatedMaListener, error) {
	if hasWarpID(laddr) {
		a := t.currentAlias()
		if a == nil {
			return nil, errors.New("camouflage/alias: /warpid/ listen requested but alias mode is not enabled (call EnableAlias)")
		}
		sl, err := a.prepareListener(laddr)
		if err != nil {
			return nil, err
		}
		return t.upgrader.GateMaListener(sl), nil
	}
	if t.sharedTCP != nil {
		return t.sharedTCP.DemultiplexedListen(laddr, tcpreuse.DemultiplexedConnType_TLS)
	}
	mal, err := manet.Listen(laddr)
	if err != nil {
		return nil, err
	}
	return t.upgrader.GateMaListener(mal), nil
}

func (t *CamouflageTransport) CanDial(addr ma.Multiaddr) bool {
	if hasWarpID(addr) {
		a := t.currentAlias()
		return a != nil && a.canDial(addr)
	}
	return t.inner.CanDial(addr)
}

// Protocols claims /warpid/ unconditionally; the swarm reads this once
// at AddTransport time. CanDial/Dial/Listen guard until EnableAlias.
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

// EnableAlias wires alias mode onto the CamouflageTransport registered
// on h's swarm. warpID="" makes the host dial-only. Lower-cases input.
func EnableAlias(h host.Host, warpID string) error {
	if h == nil {
		return errors.New("camouflage/alias: host is nil")
	}
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

	spoof := func(s network.Stream, local, remote ma.Multiaddr) *SpoofConn {
		return NewSpoofConnFromStream(s, local, remote, t.fragmentSize, t.handshakeLen, t.maxDelay)
	}
	wrap := func(s network.Stream, local, remote ma.Multiaddr, isClient bool) (manet.Conn, error) {
		return NewCamouflageConn(spoof(s, local, remote), isClient, t.camoConfig)
	}

	t.alias = newAliasMode(h, t.upgrader, warpID, spoof, wrap)
	return nil
}

// EnableAliasService runs the alias resolver on h — counterpart of
// libp2p.EnableRelayService. Thin clients must NOT call this.
func EnableAliasService(h host.Host) (*aliasresolver.Resolver, error) {
	if h == nil {
		return nil, errors.New("camouflage/alias: host is nil")
	}
	for _, p := range h.Mux().Protocols() {
		if p == aliasresolver.RegisterProtocol {
			return nil, errors.New("camouflage/alias: alias service already enabled")
		}
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

		spoofed := NewSpoofConn(conn, l.fragmentSize, l.handshakeLen, l.maxDelay)
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

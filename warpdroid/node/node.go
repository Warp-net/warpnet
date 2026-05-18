//go:build mobile

package node

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	camouflage "github.com/Warp-net/libp2p-camouflage-transport"
	"github.com/libp2p/go-libp2p"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/multiformats/go-multiaddr"
	"io"
	"strings"
	"sync"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
)

type clientNode struct {
	host          host.Host
	ctx           context.Context
	cancel        context.CancelFunc
	desktopPeerID peer.ID
	mu            sync.RWMutex
	dht           *dht.IpfsDHT
	privKey       crypto.PrivKey

	// Event ring buffer for libp2p connectedness changes. Notifiee
	// callbacks push here cheaply; the buffer is drained to log only
	// when the Kotlin side actually calls a method on the node (so we
	// don't have a polling goroutine eating the battery).
	eventsMu sync.Mutex
	events   []string
}

// nodeNotifiee is a network.Notifiee that records connection events into
// the parent clientNode's lazy event buffer. No I/O happens in the
// callbacks — just an in-memory append.
type nodeNotifiee struct{ c *clientNode }

func (n *nodeNotifiee) Listen(_ network.Network, _ multiaddr.Multiaddr)      {}
func (n *nodeNotifiee) ListenClose(_ network.Network, _ multiaddr.Multiaddr) {}

func (n *nodeNotifiee) Connected(_ network.Network, conn network.Conn) {
	n.c.recordEvent("Connected", conn)
}

func (n *nodeNotifiee) Disconnected(_ network.Network, conn network.Conn) {
	n.c.recordEvent("Disconnected", conn)
}

func (c *clientNode) recordEvent(kind string, conn network.Conn) {
	dir := "?"
	switch conn.Stat().Direction {
	case network.DirInbound:
		dir = "in"
	case network.DirOutbound:
		dir = "out"
	}
	transient := ""
	if conn.Stat().Limited {
		transient = " (limited)"
	}
	line := fmt.Sprintf(
		"libp2p %s %s peer=%s remote=%s%s",
		kind, dir, conn.RemotePeer(), conn.RemoteMultiaddr(), transient,
	)
	c.eventsMu.Lock()
	c.events = append(c.events, line)
	// Cap buffer so a long idle period can't grow it unboundedly.
	if len(c.events) > 256 {
		c.events = c.events[len(c.events)-256:]
	}
	c.eventsMu.Unlock()
}

// flushEvents prints any buffered libp2p events through fmt.Printf (which
// gomobile redirects to adb logcat under the GoLog tag). Called at the top
// of every public clientNode method so events surface only on real
// interaction with the binding — no background poller.
func (c *clientNode) flushEvents() {
	c.eventsMu.Lock()
	if len(c.events) == 0 {
		c.eventsMu.Unlock()
		return
	}
	events := c.events
	c.events = nil
	c.eventsMu.Unlock()
	for _, line := range events {
		fmt.Println(line)
	}
}

// newClient creates a new WarpNet thin client configured as per requirements
func newClient(
	privKey []byte,
	psk []byte,
	warpNetwork string,
	bootstrapNodes []string,
) (*clientNode, error) {
	if len(psk) == 0 {
		return nil, errors.New("psk is required")
	}
	if warpNetwork == "" {
		warpNetwork = "testnet"
	}
	if len(bootstrapNodes) == 0 {
		return nil, errors.New("bootstrap nodes required")
	}
	ctx, cancel := context.WithCancel(context.Background())

	// Generate a new private key for this client instance
	// Ed25519 has a fixed key size, so -1 is used when the parameter is not applicable
	privateKey, err := crypto.UnmarshalEd25519PrivateKey(privKey)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}
	connManager, err := connmgr.NewConnManager(
		2, // low water
		5, // high water
		connmgr.WithGracePeriod(20*time.Second),
		connmgr.WithSilencePeriod(10*time.Second),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create conn manager: %w", err)
	}

	limits := rcmgr.DefaultLimits
	libp2p.SetDefaultServiceLimits(&limits)
	limiter := rcmgr.NewFixedLimiter(limits.Scale(
		16<<20, // 16 MB
		128,
	))
	rm, _ := rcmgr.NewResourceManager(limiter)

	ya := yamux.DefaultTransport
	ya.KeepAliveInterval = 15 * time.Second
	ya.ConnectionWriteTimeout = 30 * time.Second

	// Build libp2p options matching thin client requirements
	opts := []libp2p.Option{
		libp2p.DisableMetrics(), // Lightweight
		libp2p.EnableRelay(),    // Circuit-v2 client so /p2p-circuit bootstrap addrs are dialable
		libp2p.DisableIdentifyAddressDiscovery(),
		libp2p.NoTransports,
		libp2p.NoListenAddrs, // Client-only mode - no listening
		libp2p.PrivateNetwork(psk),
		libp2p.Identity(privateKey),                         // Client identity
		libp2p.Security(noise.ID, noise.New),                // Noise protocol for encryption
		libp2p.Transport(camouflage.NewCamouflageTransport), // TCP transport
		libp2p.UserAgent("warpdroid"),                       // Custom user agent
		libp2p.ForceReachabilityPrivate(),
		libp2p.Muxer(yamux.ID, ya),
		libp2p.ConnectionManager(connManager),
		libp2p.ResourceManager(rm),
	}

	// Create the libp2p host
	h, err := libp2p.New(opts...)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	var infos []peer.AddrInfo
	for _, addr := range bootstrapNodes {
		maddr, _ := multiaddr.NewMultiaddr(addr)
		addrInfo, _ := peer.AddrInfoFromP2pAddr(maddr)
		if addrInfo == nil {
			continue
		}
		infos = append(infos, *addrInfo)
	}

	hashTable, err := dht.New(
		ctx, h,
		dht.Mode(dht.ModeClient),
		dht.ProtocolPrefix(protocol.ID("/"+warpNetwork)),
		dht.Concurrency(3),
		dht.RoutingTableRefreshPeriod(6*time.Hour),
		dht.MaxRecordAge(time.Hour),
		dht.DisableAutoRefresh(),
		dht.QueryFilter(dht.PublicQueryFilter),
		dht.RoutingTableRefreshQueryTimeout(time.Second*30), //nolint:mnd
		dht.BootstrapPeers(infos...),
		dht.RoutingTableLatencyTolerance(time.Second*20),
		dht.BucketSize(10), //nolint:mnd
	)
	if err != nil {
		cancel()
		return nil, err
	}

	cn := &clientNode{
		host:    h,
		ctx:     ctx,
		cancel:  cancel,
		dht:     hashTable,
		privKey: privateKey,
	}
	h.Network().Notify(&nodeNotifiee{c: cn})

	for _, addr := range bootstrapNodes {
		if err := cn.connect(addr); err != nil {
			fmt.Printf("failed to connect to bootstrap node %s: %v\n", addr, err)
			continue
		}
	}
	return cn, nil
}

// connect accepts one or more newline-separated multiaddrs for a single
// peer and hands them all to host.Connect in one call so libp2p's
// swarm.DefaultDialRanker can rank and dial them in parallel.
func (c *clientNode) connect(peerInfo string) error {
	c.flushEvents()
	go func() {
		_ = c.dht.Bootstrap(c.ctx)
		c.dht.RefreshRoutingTable()
	}()

	if peerInfo == "" {
		return fmt.Errorf("not connected to desktop node")
	}

	var (
		peerID peer.ID
		addrs  []multiaddr.Multiaddr
	)
	for _, line := range strings.Split(peerInfo, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		maddr, err := multiaddr.NewMultiaddr(line)
		if err != nil {
			// Skip unparseable entries silently so a single typo in
			// the QR can't kill the whole dial. The caller already
			// validated structurally before getting here.
			continue
		}
		info, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil || info == nil {
			continue
		}
		if peerID == "" {
			peerID = info.ID
		} else if peerID != info.ID {
			// Mixed peer IDs in one batch — bug on the caller side.
			// Drop the mismatched entries and keep going.
			continue
		}
		addrs = append(addrs, info.Addrs...)
	}
	if peerID == "" || len(addrs) == 0 {
		return fmt.Errorf("invalid peer info: %s", peerInfo)
	}
	if len(peerID) > 52 {
		return fmt.Errorf("stream: node id is too long: %s", peerID)
	}
	if err := peerID.Validate(); err != nil {
		return err
	}

	// Peerstore is internally thread-safe and host.Connect can take 30s —
	// don't hold c.mu across either, or pause/resume/disconnect would
	// block on the dial.
	c.host.Peerstore().AddAddrs(peerID, addrs, peerstore.PermanentAddrTTL)

	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	if err := c.host.Connect(ctx, peer.AddrInfo{ID: peerID, Addrs: addrs}); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	c.mu.Lock()
	c.desktopPeerID = peerID
	c.mu.Unlock()
	return nil
}

func (c *clientNode) stream(protocolID string, data []byte) ([]byte, error) {
	c.flushEvents()
	c.mu.RLock()
	desktopPeerID := c.desktopPeerID
	c.mu.RUnlock()
	if desktopPeerID == "" {
		return nil, fmt.Errorf("not connected to desktop node")
	}

	if protocolID == "" {
		return nil, fmt.Errorf("empty protocol ID")
	}

	ctx, cancel := context.WithTimeout(c.ctx, 15*time.Second)
	defer cancel()

	connectedness := c.host.Network().Connectedness(desktopPeerID)
	switch connectedness {
	case network.Limited:
		ctx = network.WithAllowLimitedConn(ctx, "warpnet")
	default:
	}

	stream, err := c.host.NewStream(ctx, desktopPeerID, protocol.ID(protocolID))
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}

	var rw = bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
	if data != nil {
		_, err = rw.Write(data)
	}
	flush(rw)
	closeWrite(stream)
	if err != nil {
		return nil, fmt.Errorf("stream: writing: %w", err)
	}

	buf := bytes.NewBuffer(nil)
	_, err = buf.ReadFrom(rw)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf(
			"stream: reading response from %s: %w", desktopPeerID.String(), err,
		)
	}

	return buf.Bytes(), nil
}

func flush(rw *bufio.ReadWriter) {
	if err := rw.Flush(); err != nil {
		fmt.Printf("stream: close write: %s", err)
	}
}

func closeWrite(s network.Stream) {
	if err := s.CloseWrite(); err != nil {
		fmt.Printf("stream: close write: %s", err)
	}
}

// sign produces a base64-encoded Ed25519 signature over body using the libp2p
// identity key. The format matches security.Sign on the desktop side, so the
// node's auth middleware (warpnet/core/middleware/auth.go) verifies it
// against the peer ID extracted from the libp2p connection.
func (c *clientNode) sign(body []byte) (string, error) {
	c.flushEvents()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.privKey == nil {
		return "", errors.New("private key not set")
	}
	sig, err := c.privKey.Sign(body)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func (c *clientNode) getPeerID() string {
	c.flushEvents()
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.host.ID().String()
}

// IsConnected checks if connected to the desktop node
func (c *clientNode) isConnected() bool {
	c.flushEvents()
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.desktopPeerID == "" {
		return false
	}

	connectedness := c.host.Network().Connectedness(c.desktopPeerID)
	return connectedness == network.Connected || connectedness == network.Limited
}

// connectedness returns the libp2p Connectedness#String for the paired
// desktop peer. Surface for the Kotlin ConnectionMonitor, which owns the
// reconnect loop; Go only reports the snapshot. Returns "NotConnected"
// when no peer is paired yet.
func (c *clientNode) connectedness() string {
	c.flushEvents()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.desktopPeerID == "" {
		return network.NotConnected.String()
	}
	return c.host.Network().Connectedness(c.desktopPeerID).String()
}

func (c *clientNode) disconnect() error {
	c.flushEvents()
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.desktopPeerID != "" {
		if err := c.host.Network().ClosePeer(c.desktopPeerID); err != nil {
			return fmt.Errorf("failed to close peer connection: %w", err)
		}
		c.host.Peerstore().RemovePeer(c.desktopPeerID)
		c.desktopPeerID = ""
	}

	return nil
}

// pause drops all live connections on the libp2p host so the radio can
// go to sleep while the app is backgrounded. The host itself stays
// initialised; resume() re-dials the paired peer.
func (c *clientNode) pause() {
	c.flushEvents()
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, conn := range c.host.Network().Conns() {
		_ = conn.Close()
	}
}

// resume re-dials the paired desktop peer in the background. Kicked from
// the Android onStart lifecycle callback, which runs on the main thread,
// so the actual host.Connect runs in a goroutine bounded by a 10s
// ceiling — a dead peer can't wedge subsequent lifecycle callbacks.
// Only the paired peer is dialled; iterating every peerstore entry
// would burn dial budget on stale DHT routing-table fillers.
func (c *clientNode) resume() {
	c.flushEvents()
	c.mu.RLock()
	desktopPeerID := c.desktopPeerID
	c.mu.RUnlock()
	if desktopPeerID == "" {
		return
	}
	info := c.host.Peerstore().PeerInfo(desktopPeerID)
	go func() {
		// Parent off c.ctx so close() / cancel() reliably tears down
		// in-flight resume dials instead of letting them outlive the
		// node.
		ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		defer cancel()
		_ = c.host.Connect(ctx, info)
	}()
}

func (c *clientNode) close() error {
	defer func() { recover() }()
	c.cancel()
	_ = c.dht.Close()
	return c.host.Close()
}

// refreshPeerAddrs adds the supplied newline-separated multiaddrs to the
// paired desktop peer's peerstore entry with a permanent TTL. The pair
// handler on the fat node returns its current public addresses on every
// call, so periodically re-pairing keeps the thin client's peerstore in
// sync with the fat node's IP / port changes — handy when the desktop
// moves between networks.
func (c *clientNode) refreshPeerAddrs(addrs string) error {
	c.flushEvents()
	c.mu.RLock()
	peerID := c.desktopPeerID
	c.mu.RUnlock()
	if peerID == "" {
		return fmt.Errorf("no paired peer")
	}
	var maddrs []multiaddr.Multiaddr
	hadInput := false
	for _, line := range strings.Split(addrs, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		hadInput = true
		m, err := multiaddr.NewMultiaddr(line)
		if err != nil {
			continue
		}
		maddrs = append(maddrs, m)
	}
	if hadInput && len(maddrs) == 0 {
		// Non-empty input but every line failed to parse — surface the
		// contract violation instead of silently leaving the peerstore
		// stale.
		return fmt.Errorf("no valid multiaddrs in refresh payload")
	}
	if len(maddrs) == 0 {
		return nil
	}
	c.host.Peerstore().AddAddrs(peerID, maddrs, peerstore.PermanentAddrTTL)
	return nil
}

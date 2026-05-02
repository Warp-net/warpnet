package node

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	camouflage "github.com/Warp-net/libp2p-camouflage-transport"
	"github.com/libp2p/go-libp2p"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/multiformats/go-multiaddr"
	"io"
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
		libp2p.Muxer(yamux.ID, yamux.DefaultTransport),
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
		host:   h,
		ctx:    ctx,
		cancel: cancel,
		dht:    hashTable,
	}

	for _, addr := range bootstrapNodes {
		if err := cn.connect(addr); err != nil {
			fmt.Printf("failed to connect to bootstrap node %s: %v\n", addr, err)
			continue
		}
	}
	return cn, nil
}

func (c *clientNode) connect(peerInfo string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	go func() {
		_ = c.dht.Bootstrap(c.ctx)
		c.dht.RefreshRoutingTable()
	}()

	if peerInfo == "" {
		return fmt.Errorf("not connected to desktop node")
	}
	addrInfo, err := peer.AddrInfoFromString(peerInfo)
	if err != nil {
		return err
	}
	if addrInfo == nil {
		return fmt.Errorf("invalid peer info: %s", peerInfo)
	}
	if len(addrInfo.ID) > 52 {
		return fmt.Errorf("stream: node id is too long: %s", peerInfo)
	}
	if err := addrInfo.ID.Validate(); err != nil {
		return err
	}

	c.host.Peerstore().AddAddrs(addrInfo.ID, addrInfo.Addrs, peerstore.PermanentAddrTTL)

	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	if err := c.host.Connect(ctx, *addrInfo); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	c.desktopPeerID = addrInfo.ID
	return nil
}

func (c *clientNode) stream(protocolID string, data []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.desktopPeerID == "" {
		return nil, fmt.Errorf("not connected to desktop node")
	}
	desktopPeerID := c.desktopPeerID

	if protocolID == "" {
		return nil, fmt.Errorf("empty protocol ID")
	}

	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
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

func (c *clientNode) getPeerID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.host.ID().String()
}

// IsConnected checks if connected to the desktop node
func (c *clientNode) isConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.desktopPeerID == "" {
		return false
	}

	connectedness := c.host.Network().Connectedness(c.desktopPeerID)
	return connectedness == network.Connected || connectedness == network.Limited
}

func (c *clientNode) disconnect() error {
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

func (c *clientNode) close() error {
	defer func() { recover() }()
	c.cancel()
	_ = c.dht.Close()
	return c.host.Close()
}

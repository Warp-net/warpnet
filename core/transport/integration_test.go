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

package transport

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testProtocol = protocol.ID("/dpi-test/echo/1.0.0")

// TestIntegration_TwoNodes verifies that two libp2p hosts using the
// SpoofTransport (TLS camouflage + TCP fragmentation) can successfully:
//   - complete the Noise handshake through the camouflage layer
//   - open a stream and exchange data bidirectionally
func TestIntegration_TwoNodes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create two hosts with SpoofTransport + Noise security.
	hostA, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Transport(NewCamouflageTransport,
			WithFragmentSize(3),
			WithMaxDelay(0),
		),
		libp2p.Security(noise.ID, noise.New),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	defer hostA.Close()

	hostB, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Transport(NewCamouflageTransport,
			WithFragmentSize(3),
			WithMaxDelay(0),
		),
		libp2p.Security(noise.ID, noise.New),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	defer hostB.Close()

	// Set up echo handler on host B.
	done := make(chan struct{})
	hostB.SetStreamHandler(testProtocol, func(s network.Stream) {
		defer s.Close()
		defer close(done)

		buf, err := io.ReadAll(s)
		if err != nil {
			t.Errorf("hostB: read error: %v", err)
			return
		}
		// Echo back.
		_, _ = s.Write(buf)
	})

	// Connect A → B.
	hostA.Peerstore().AddAddrs(hostB.ID(), hostB.Addrs(), time.Hour)
	err = hostA.Connect(ctx, peer.AddrInfo{
		ID:    hostB.ID(),
		Addrs: hostB.Addrs(),
	})
	require.NoError(t, err, "hosts should connect through SpoofTransport")

	// Open stream A → B and send data.
	stream, err := hostA.NewStream(ctx, hostB.ID(), testProtocol)
	require.NoError(t, err, "should open stream through camouflaged connection")

	testPayload := []byte("Hello through TLS-camouflaged DPI-evasion transport!")
	_, err = stream.Write(testPayload)
	require.NoError(t, err)
	_ = stream.CloseWrite()

	// Read echo from B.
	reply, err := io.ReadAll(stream)
	require.NoError(t, err)
	assert.Equal(t, testPayload, reply, "echoed data should match")

	// Wait for handler to complete.
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timeout waiting for echo handler")
	}
}

// TestIntegration_LargePayload verifies that large data transfers work
// correctly through the camouflage layer (multiple TLS records).
func TestIntegration_LargePayload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hostA, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Transport(NewCamouflageTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	defer hostA.Close()

	hostB, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Transport(NewCamouflageTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	defer hostB.Close()

	receivedCh := make(chan []byte, 1)
	hostB.SetStreamHandler(testProtocol, func(s network.Stream) {
		defer s.Close()
		buf, err := io.ReadAll(s)
		if err != nil {
			t.Errorf("hostB: read error: %v", err)
			return
		}
		receivedCh <- buf
	})

	hostA.Peerstore().AddAddrs(hostB.ID(), hostB.Addrs(), time.Hour)
	err = hostA.Connect(ctx, peer.AddrInfo{ID: hostB.ID(), Addrs: hostB.Addrs()})
	require.NoError(t, err)

	stream, err := hostA.NewStream(ctx, hostB.ID(), testProtocol)
	require.NoError(t, err)

	// Send 128KB — spans multiple TLS Application Data records.
	bigPayload := make([]byte, 128*1024)
	for i := range bigPayload {
		bigPayload[i] = byte(i % 251) // deterministic pattern
	}
	_, err = stream.Write(bigPayload)
	require.NoError(t, err)
	_ = stream.Close()

	select {
	case received := <-receivedCh:
		assert.Equal(t, bigPayload, received, "large payload should arrive intact")
	case <-ctx.Done():
		t.Fatal("timeout waiting for large payload")
	}
}

// TestIntegration_BidirectionalStream verifies full-duplex communication
// where both sides write and read concurrently.
func TestIntegration_BidirectionalStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hostA, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Transport(NewCamouflageTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	defer hostA.Close()

	hostB, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Transport(NewCamouflageTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	defer hostB.Close()

	serverMsg := []byte("server-says-hello")
	clientMsg := []byte("client-says-hello")

	serverRecv := make(chan []byte, 1)
	hostB.SetStreamHandler(testProtocol, func(s network.Stream) {
		defer s.Close()

		// Write server message immediately.
		_, err := s.Write(serverMsg)
		if err != nil {
			t.Errorf("hostB write: %v", err)
			return
		}
		_ = s.CloseWrite()

		// Read client message.
		buf, err := io.ReadAll(s)
		if err != nil {
			t.Errorf("hostB read: %v", err)
			return
		}
		serverRecv <- buf
	})

	hostA.Peerstore().AddAddrs(hostB.ID(), hostB.Addrs(), time.Hour)
	err = hostA.Connect(ctx, peer.AddrInfo{ID: hostB.ID(), Addrs: hostB.Addrs()})
	require.NoError(t, err)

	stream, err := hostA.NewStream(ctx, hostB.ID(), testProtocol)
	require.NoError(t, err)

	// Write client message.
	_, err = stream.Write(clientMsg)
	require.NoError(t, err)
	_ = stream.CloseWrite()

	// Read server message.
	reply, err := io.ReadAll(stream)
	require.NoError(t, err)
	assert.Equal(t, serverMsg, reply)

	select {
	case got := <-serverRecv:
		assert.Equal(t, clientMsg, got)
	case <-ctx.Done():
		t.Fatal("timeout")
	}
}

// ---------------------------------------------------------------------------
// DPI emulation test
// ---------------------------------------------------------------------------

// tlsContentType describes TLS record content types for DPI analysis.
const (
	tlsContentTypeChangeCipherSpec = 0x14
	tlsContentTypeAlert            = 0x15
	tlsContentTypeHandshake        = 0x16
	tlsContentTypeApplicationData  = 0x17
)

// tlsRecord represents a parsed TLS record header seen on the wire.
type tlsRecord struct {
	ContentType byte
	Major       byte
	Minor       byte
	Length      uint16
}

// parseTLSRecords reassembles captured TCP segments and parses out TLS
// record headers. Returns all records found in the stream and a boolean
// indicating whether the entire stream consisted of valid TLS records.
func parseTLSRecords(raw []byte) ([]tlsRecord, bool) {
	var records []tlsRecord
	offset := 0
	for offset+5 <= len(raw) {
		ct := raw[offset]
		if ct < 0x14 || ct > 0x17 {
			return records, false // not a valid TLS content type
		}
		major := raw[offset+1]
		minor := raw[offset+2]
		// TLS record version must be 3.x (SSL 3.0 / TLS 1.0-1.3).
		if major != 0x03 {
			return records, false
		}
		// Minor version should be 0x00-0x04.
		if minor > 0x04 {
			return records, false
		}
		length := binary.BigEndian.Uint16(raw[offset+3 : offset+5])
		// Sanity: TLS records should not exceed 16KB + overhead.
		if length > 16384+2048 {
			return records, false
		}
		records = append(records, tlsRecord{
			ContentType: ct,
			Major:       major,
			Minor:       minor,
			Length:      length,
		})
		offset += 5 + int(length)
	}
	// If there are leftover bytes that don't form a record header, the
	// stream may have been partially captured. Still valid if we got at
	// least some records.
	return records, len(records) > 0
}

// tcpProxy is a simple TCP proxy that captures all traffic flowing from
// the client to the server, allowing DPI-like inspection of raw bytes.
type tcpProxy struct {
	listener   net.Listener
	targetAddr string

	mu       sync.Mutex
	captured []byte // all bytes from client → server
}

func newTCPProxy(t *testing.T, targetAddr string) *tcpProxy {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0") //nolint:noctx
	require.NoError(t, err)
	return &tcpProxy{
		listener:   lis,
		targetAddr: targetAddr,
	}
}

func (p *tcpProxy) addr() string {
	return p.listener.Addr().String()
}

func (p *tcpProxy) close() {
	_ = p.listener.Close()
}

func (p *tcpProxy) getCaptured() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]byte, len(p.captured))
	copy(out, p.captured)
	return out
}

// run accepts one connection, proxies traffic bidirectionally, and
// records all client→server bytes. Blocks until both directions finish.
func (p *tcpProxy) run(t *testing.T) {
	t.Helper()
	clientConn, err := p.listener.Accept()
	if err != nil {
		return
	}
	defer clientConn.Close()

	serverConn, err := net.Dial("tcp", p.targetAddr) //nolint:noctx
	require.NoError(t, err)
	defer serverConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	// Client → Server (captured).
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := clientConn.Read(buf)
			if n > 0 {
				p.mu.Lock()
				p.captured = append(p.captured, buf[:n]...)
				p.mu.Unlock()
				_, _ = serverConn.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// Server → Client (forwarded, not captured).
	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, serverConn)
	}()

	wg.Wait()
}

// TestIntegration_DPIEmulation sets up a TCP proxy between two libp2p
// nodes using SpoofTransport and inspects the raw traffic to verify it
// looks like a standard TLS session.
//
// The DPI checks:
//  1. First record must be a TLS Handshake (ClientHello, type 0x16)
//  2. All records must have valid TLS content types (0x14-0x17)
//  3. All records must have TLS version 3.x
//  4. After the handshake, application data records (0x17) must be present
//  5. No plaintext Noise or multistream-select bytes visible on the wire
func TestIntegration_DPIEmulation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start host B (server) first to get its address.
	hostB, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Transport(NewCamouflageTransport,
			WithFragmentSize(3),
			WithMaxDelay(0),
		),
		libp2p.Security(noise.ID, noise.New),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	defer hostB.Close()

	// Extract the TCP address from host B's multiaddr.
	bAddrs := hostB.Addrs()
	require.NotEmpty(t, bAddrs)
	// Parse the TCP host:port from the multiaddr.
	var bTCPAddr string
	for _, addr := range bAddrs {
		parts := addr.Protocols()
		if len(parts) >= 2 && parts[0].Name == "ip4" && parts[1].Name == "tcp" {
			ip, _ := addr.ValueForProtocol(4)   // /ip4
			port, _ := addr.ValueForProtocol(6) // /tcp
			bTCPAddr = ip + ":" + port
			break
		}
	}
	require.NotEmpty(t, bTCPAddr, "could not extract TCP address from host B")

	// Set up the TCP proxy pointing to host B.
	proxy := newTCPProxy(t, bTCPAddr)
	defer proxy.close()

	// Run the proxy in a goroutine.
	proxyDone := make(chan struct{})
	go func() {
		defer close(proxyDone)
		proxy.run(t)
	}()

	// Start host A (client) that connects through the proxy.
	// We use the proxy address as the dial target by constructing
	// a multiaddr pointing to the proxy.
	proxyMA := "/ip4/127.0.0.1/tcp/" + portFromAddr(proxy.addr())

	hostA, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Transport(NewCamouflageTransport,
			WithFragmentSize(3),
			WithMaxDelay(0),
		),
		libp2p.Security(noise.ID, noise.New),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	defer hostA.Close()

	// Set up echo handler on host B.
	done := make(chan struct{})
	hostB.SetStreamHandler(testProtocol, func(s network.Stream) {
		defer s.Close()
		defer close(done)
		buf, readErr := io.ReadAll(s)
		if readErr != nil {
			return
		}
		_, _ = s.Write(buf)
	})

	// Tell host A to connect to host B via the proxy address.
	proxyMultiaddr, err := peer.AddrInfoFromString(proxyMA + "/p2p/" + hostB.ID().String())
	require.NoError(t, err)
	hostA.Peerstore().AddAddrs(proxyMultiaddr.ID, proxyMultiaddr.Addrs, time.Hour)
	err = hostA.Connect(ctx, *proxyMultiaddr)
	require.NoError(t, err, "should connect through TCP proxy")

	// Exchange data to generate both handshake and application traffic.
	stream, err := hostA.NewStream(ctx, hostB.ID(), testProtocol)
	require.NoError(t, err)

	payload := []byte("DPI-test: this payload must NOT be visible in raw traffic")
	_, err = stream.Write(payload)
	require.NoError(t, err)
	_ = stream.CloseWrite()

	reply, err := io.ReadAll(stream)
	require.NoError(t, err)
	assert.Equal(t, payload, reply)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timeout waiting for echo handler")
	}

	// Close connections to flush the proxy buffers.
	_ = stream.Close()
	hostA.Close()
	hostB.Close()

	// Wait for proxy goroutine to finish.
	select {
	case <-proxyDone:
	case <-time.After(5 * time.Second):
		// Proxy may not finish cleanly; proceed with what we have.
	}

	// --- DPI analysis of captured raw traffic ---
	captured := proxy.getCaptured()
	require.True(t, len(captured) > 5,
		"proxy should have captured some traffic, got %d bytes", len(captured))

	// 1. First byte must be 0x16 (TLS Handshake = ClientHello).
	assert.Equal(t, byte(tlsContentTypeHandshake), captured[0],
		"first byte on wire must be TLS Handshake (0x16), got 0x%02x", captured[0])

	// 2. Parse all TLS records from the captured stream.
	records, validTLS := parseTLSRecords(captured)
	assert.True(t, validTLS,
		"entire captured stream should consist of valid TLS records")
	require.True(t, len(records) >= 2,
		"should see at least 2 TLS records (handshake + app data), got %d", len(records))

	// 3. First record should be a Handshake.
	assert.Equal(t, byte(tlsContentTypeHandshake), records[0].ContentType,
		"first TLS record should be Handshake")

	// 4. There should be Application Data records (encrypted payload).
	hasAppData := false
	for _, r := range records {
		if r.ContentType == tlsContentTypeApplicationData {
			hasAppData = true
			break
		}
	}
	assert.True(t, hasAppData,
		"should see TLS Application Data records in the traffic")

	// 5. All records must have valid TLS version (3.x).
	for i, r := range records {
		assert.Equal(t, byte(0x03), r.Major,
			"record %d: TLS major version must be 3", i)
		assert.True(t, r.Minor <= 0x04,
			"record %d: TLS minor version must be <= 4, got %d", i, r.Minor)
	}

	// 6. Plaintext protocol strings must NOT be visible in raw traffic.
	// If they were, it means the TLS encryption is not working.
	assert.NotContains(t, string(captured), "/multistream/",
		"raw traffic must not contain plaintext multistream-select")
	assert.NotContains(t, string(captured), "/noise",
		"raw traffic must not contain plaintext Noise protocol ID")
	assert.NotContains(t, string(captured), string(payload),
		"raw traffic must not contain the plaintext payload")

	// Log summary for debugging.
	t.Logf("DPI analysis: captured %d bytes, %d TLS records", len(captured), len(records))
	for i, r := range records {
		typeName := "Unknown"
		switch r.ContentType {
		case tlsContentTypeHandshake:
			typeName = "Handshake"
		case tlsContentTypeChangeCipherSpec:
			typeName = "ChangeCipherSpec"
		case tlsContentTypeApplicationData:
			typeName = "ApplicationData"
		case tlsContentTypeAlert:
			typeName = "Alert"
		}
		t.Logf("  record[%d]: type=0x%02x (%s) version=%d.%d len=%d",
			i, r.ContentType, typeName, r.Major, r.Minor, r.Length)
	}
}

// ---------------------------------------------------------------------------
// mDNS discovery test
// ---------------------------------------------------------------------------

// mdnsNotifee connects to discovered peers and signals via a channel.
type mdnsNotifee struct {
	h         host.Host
	found     chan peer.AddrInfo
	connected chan peer.ID
}

func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	if pi.ID == n.h.ID() {
		return // ignore self
	}
	// Signal discovery.
	select {
	case n.found <- pi:
	default:
	}

	// Attempt to connect.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n.h.Peerstore().AddAddrs(pi.ID, pi.Addrs, time.Hour)
	if err := n.h.Connect(ctx, pi); err != nil {
		return
	}
	select {
	case n.connected <- pi.ID:
	default:
	}
}

// TestIntegration_MDNSDiscovery verifies that two libp2p hosts using
// CamouflageTransport can discover each other via mDNS and successfully
// exchange data through the camouflaged connection.
//
// NOTE: This test requires UDP multicast to work (not available in all
// environments, e.g. containers). It will be skipped if multicast is
// unavailable.
func TestIntegration_MDNSDiscovery(t *testing.T) {
	checkMDNSAvailable(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Host A.
	hostA, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Transport(NewCamouflageTransport,
			WithMaxDelay(0),
		),
		libp2p.Security(noise.ID, noise.New),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	defer hostA.Close()

	// Host B.
	hostB, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Transport(NewCamouflageTransport,
			WithMaxDelay(0),
		),
		libp2p.Security(noise.ID, noise.New),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	defer hostB.Close()

	t.Logf("Host A: %s addrs=%v", hostA.ID(), hostA.Addrs())
	t.Logf("Host B: %s addrs=%v", hostB.ID(), hostB.Addrs())

	// Set up echo handler on host B.
	echoDone := make(chan struct{})
	hostB.SetStreamHandler(testProtocol, func(s network.Stream) {
		defer s.Close()
		defer close(echoDone)
		buf, err := io.ReadAll(s)
		if err != nil {
			t.Errorf("hostB: read error: %v", err)
			return
		}
		_, _ = s.Write(buf)
	})

	// Start mDNS on both hosts.
	const mdnsServiceName = "_camouflage-test._udp"

	notifeeA := &mdnsNotifee{
		h:         hostA,
		found:     make(chan peer.AddrInfo, 5),
		connected: make(chan peer.ID, 5),
	}
	mdnsA := mdns.NewMdnsService(hostA, mdnsServiceName, notifeeA)
	require.NoError(t, mdnsA.Start())
	defer mdnsA.Close()

	notifeeB := &mdnsNotifee{
		h:         hostB,
		found:     make(chan peer.AddrInfo, 5),
		connected: make(chan peer.ID, 5),
	}
	mdnsB := mdns.NewMdnsService(hostB, mdnsServiceName, notifeeB)
	require.NoError(t, mdnsB.Start())
	defer mdnsB.Close()

	// Wait for at least one side to discover and connect to the other.
	t.Log("Waiting for mDNS discovery...")
	var connectedPeer peer.ID
	select {
	case connectedPeer = <-notifeeA.connected:
		t.Logf("Host A connected to discovered peer %s", connectedPeer)
		assert.Equal(t, hostB.ID(), connectedPeer)
	case connectedPeer = <-notifeeB.connected:
		t.Logf("Host B connected to discovered peer %s", connectedPeer)
		assert.Equal(t, hostA.ID(), connectedPeer)
	case <-ctx.Done():
		// Dump discovery state for debugging.
		t.Logf("Host A network peers: %v", hostA.Network().Peers())
		t.Logf("Host B network peers: %v", hostB.Network().Peers())
		t.Fatal("timeout waiting for mDNS discovery and connection")
	}

	// Verify data exchange works over the mDNS-discovered connection.
	// Ensure A is connected to B (might have been B→A above).
	if hostA.Network().Connectedness(hostB.ID()) != network.Connected {
		// If B discovered A, A might not have a connection to B yet.
		err := hostA.Connect(ctx, peer.AddrInfo{ID: hostB.ID(), Addrs: hostB.Addrs()})
		require.NoError(t, err)
	}

	stream, err := hostA.NewStream(ctx, hostB.ID(), testProtocol)
	require.NoError(t, err, "should open stream over mDNS-discovered connection")

	testPayload := []byte("Hello via mDNS discovery through CamouflageTransport!")
	_, err = stream.Write(testPayload)
	require.NoError(t, err)
	_ = stream.CloseWrite()

	reply, err := io.ReadAll(stream)
	require.NoError(t, err)
	assert.Equal(t, testPayload, reply, "echoed data should match")

	select {
	case <-echoDone:
	case <-ctx.Done():
		t.Fatal("timeout waiting for echo handler")
	}
}

// TestIntegration_MDNSDiscoveryWithPSKAndRelay closely matches the real
// app setup: CamouflageTransport + Noise + PSK + Relay + HolePunching.
func TestIntegration_MDNSDiscoveryWithPSKAndRelay(t *testing.T) {
	checkMDNSAvailable(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Generate a 32-byte PSK for private network.
	testPSK := make([]byte, 32)
	for i := range testPSK {
		testPSK[i] = byte(i + 1)
	}

	commonOpts := func() []libp2p.Option {
		return []libp2p.Option{
			libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
			libp2p.Transport(NewCamouflageTransport,
				WithMaxDelay(0),
			),
			libp2p.Security(noise.ID, noise.New),
			libp2p.PrivateNetwork(testPSK),
			libp2p.EnableRelay(),
			libp2p.EnableHolePunching(),
			libp2p.NATPortMap(),
		}
	}

	hostA, err := libp2p.New(commonOpts()...)
	require.NoError(t, err)
	defer hostA.Close()

	hostB, err := libp2p.New(commonOpts()...)
	require.NoError(t, err)
	defer hostB.Close()

	t.Logf("Host A: %s addrs=%v", hostA.ID(), hostA.Addrs())
	t.Logf("Host B: %s addrs=%v", hostB.ID(), hostB.Addrs())

	// Set up echo handler.
	echoDone := make(chan struct{})
	hostB.SetStreamHandler(testProtocol, func(s network.Stream) {
		defer s.Close()
		defer close(echoDone)
		buf, err := io.ReadAll(s)
		if err != nil {
			t.Errorf("hostB: read error: %v", err)
			return
		}
		_, _ = s.Write(buf)
	})

	// Start mDNS.
	const mdnsServiceName = "_camouflage-psk-test._udp"

	notifeeA := &mdnsNotifee{
		h:         hostA,
		found:     make(chan peer.AddrInfo, 5),
		connected: make(chan peer.ID, 5),
	}
	mdnsA := mdns.NewMdnsService(hostA, mdnsServiceName, notifeeA)
	require.NoError(t, mdnsA.Start())
	defer mdnsA.Close()

	notifeeB := &mdnsNotifee{
		h:         hostB,
		found:     make(chan peer.AddrInfo, 5),
		connected: make(chan peer.ID, 5),
	}
	mdnsB := mdns.NewMdnsService(hostB, mdnsServiceName, notifeeB)
	require.NoError(t, mdnsB.Start())
	defer mdnsB.Close()

	t.Log("Waiting for mDNS discovery with PSK+Relay...")
	select {
	case pid := <-notifeeA.connected:
		t.Logf("Host A connected to discovered peer %s", pid)
		assert.Equal(t, hostB.ID(), pid)
	case pid := <-notifeeB.connected:
		t.Logf("Host B connected to discovered peer %s", pid)
		assert.Equal(t, hostA.ID(), pid)
	case <-ctx.Done():
		t.Logf("Host A network peers: %v", hostA.Network().Peers())
		t.Logf("Host B network peers: %v", hostB.Network().Peers())
		t.Fatal("timeout waiting for mDNS discovery and connection (PSK+Relay)")
	}

	// Verify data exchange.
	if hostA.Network().Connectedness(hostB.ID()) != network.Connected {
		err := hostA.Connect(ctx, peer.AddrInfo{ID: hostB.ID(), Addrs: hostB.Addrs()})
		require.NoError(t, err)
	}

	stream, err := hostA.NewStream(ctx, hostB.ID(), testProtocol)
	require.NoError(t, err)

	testPayload := []byte("Hello via mDNS+PSK+Relay through CamouflageTransport!")
	_, err = stream.Write(testPayload)
	require.NoError(t, err)
	_ = stream.CloseWrite()

	reply, err := io.ReadAll(stream)
	require.NoError(t, err)
	assert.Equal(t, testPayload, reply)

	select {
	case <-echoDone:
	case <-ctx.Done():
		t.Fatal("timeout waiting for echo handler (PSK+Relay)")
	}
}

// TestIntegration_DirectConnectWithPSK verifies basic manual connectivity
// with CamouflageTransport + PSK (no discovery, just Dial).
func TestIntegration_DirectConnectWithPSK(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testPSK := make([]byte, 32)
	for i := range testPSK {
		testPSK[i] = byte(i + 1)
	}

	hostA, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Transport(NewCamouflageTransport, WithMaxDelay(0)),
		libp2p.Security(noise.ID, noise.New),
		libp2p.PrivateNetwork(testPSK),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	defer hostA.Close()

	hostB, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Transport(NewCamouflageTransport, WithMaxDelay(0)),
		libp2p.Security(noise.ID, noise.New),
		libp2p.PrivateNetwork(testPSK),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	defer hostB.Close()

	t.Logf("Host A: %s addrs=%v", hostA.ID(), hostA.Addrs())
	t.Logf("Host B: %s addrs=%v", hostB.ID(), hostB.Addrs())

	done := make(chan struct{})
	hostB.SetStreamHandler(testProtocol, func(s network.Stream) {
		defer s.Close()
		defer close(done)
		buf, err := io.ReadAll(s)
		if err != nil {
			t.Errorf("hostB: read error: %v", err)
			return
		}
		_, _ = s.Write(buf)
	})

	hostA.Peerstore().AddAddrs(hostB.ID(), hostB.Addrs(), time.Hour)
	err = hostA.Connect(ctx, peer.AddrInfo{ID: hostB.ID(), Addrs: hostB.Addrs()})
	require.NoError(t, err, "should connect with CamouflageTransport+PSK")

	stream, err := hostA.NewStream(ctx, hostB.ID(), testProtocol)
	require.NoError(t, err)

	testPayload := []byte("Hello through CamouflageTransport+PSK!")
	_, err = stream.Write(testPayload)
	require.NoError(t, err)
	_ = stream.CloseWrite()

	reply, err := io.ReadAll(stream)
	require.NoError(t, err)
	assert.Equal(t, testPayload, reply)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timeout")
	}
}

// checkMDNSAvailable performs a real mDNS discovery probe using plain
// TCP hosts. If two hosts cannot discover each other via mDNS within a
// short timeout, the calling test is skipped. This catches CI
// environments where multicast sockets can be opened but mDNS packets
// are not delivered (e.g. macOS GitHub Actions runners).
func checkMDNSAvailable(t *testing.T) {
	t.Helper()

	// Quick socket-level check first.
	conn, err := net.ListenPacket("udp4", "224.0.0.251:0") //nolint:noctx
	if err != nil {
		t.Skipf("multicast not available: %v", err)
	}
	_ = conn.Close()

	// Real mDNS probe with plain TCP (no camouflage) to isolate mDNS
	// availability from transport behavior.
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	h1, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Security(noise.ID, noise.New),
		libp2p.DisableRelay(),
	)
	if err != nil {
		t.Skipf("mDNS probe: cannot create host: %v", err)
	}
	defer h1.Close()

	h2, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Security(noise.ID, noise.New),
		libp2p.DisableRelay(),
	)
	if err != nil {
		t.Skipf("mDNS probe: cannot create host: %v", err)
	}
	defer h2.Close()

	found := make(chan struct{}, 1)
	notifee := &mdnsNotifee{
		h:         h2,
		found:     make(chan peer.AddrInfo, 1),
		connected: make(chan peer.ID, 1),
	}

	const probeSvc = "_mdns-probe._udp"
	s1 := mdns.NewMdnsService(h1, probeSvc, &mdnsNotifee{
		h:         h1,
		found:     make(chan peer.AddrInfo, 1),
		connected: make(chan peer.ID, 1),
	})
	if err := s1.Start(); err != nil {
		t.Skipf("mDNS probe: cannot start service: %v", err)
	}
	defer s1.Close()

	s2 := mdns.NewMdnsService(h2, probeSvc, notifee)
	if err := s2.Start(); err != nil {
		t.Skipf("mDNS probe: cannot start service: %v", err)
	}
	defer s2.Close()

	go func() {
		select {
		case <-notifee.found:
			found <- struct{}{}
		case <-ctx.Done():
		}
	}()

	select {
	case <-found:
		// mDNS works in this environment.
	case <-ctx.Done():
		t.Skipf("mDNS not functional in this environment (probe timed out)")
	}
}

// portFromAddr extracts the port from a "host:port" string.
func portFromAddr(addr string) string {
	_, port, _ := net.SplitHostPort(addr)
	return port
}

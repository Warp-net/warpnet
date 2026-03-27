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

package dpi

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
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
		libp2p.Transport(NewSpoofTransport,
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
		libp2p.Transport(NewSpoofTransport,
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
		libp2p.Transport(NewSpoofTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	defer hostA.Close()

	hostB, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Transport(NewSpoofTransport),
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
		libp2p.Transport(NewSpoofTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.DisableRelay(),
	)
	require.NoError(t, err)
	defer hostA.Close()

	hostB, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Transport(NewSpoofTransport),
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

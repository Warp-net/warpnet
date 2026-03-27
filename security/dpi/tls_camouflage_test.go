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
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"

	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// pipeConn – a pair of connected manet.Conn backed by net.Pipe
// ---------------------------------------------------------------------------

type pipeConn struct {
	net.Conn
}

func (p *pipeConn) LocalMultiaddr() ma.Multiaddr  { return nil }
func (p *pipeConn) RemoteMultiaddr() ma.Multiaddr { return nil }

func newPipePair() (*pipeConn, *pipeConn) {
	a, b := net.Pipe()
	return &pipeConn{a}, &pipeConn{b}
}

// ---------------------------------------------------------------------------
// ClientHello structure tests
// ---------------------------------------------------------------------------

func TestBuildClientHello_HasValidTLSStructure(t *testing.T) {
	hello := buildClientHello("example.com")

	require.True(t, len(hello) > tlsRecordHeaderLen, "ClientHello too short")

	// TLS record header.
	assert.Equal(t, byte(tlsRecordHandshake), hello[0], "content type should be Handshake (0x16)")
	assert.Equal(t, byte(0x03), hello[1])
	assert.Equal(t, byte(0x03), hello[2], "record version should be TLS 1.2")
	recordLen := int(binary.BigEndian.Uint16(hello[3:5]))
	assert.Equal(t, len(hello)-tlsRecordHeaderLen, recordLen)

	// Handshake header.
	payload := hello[tlsRecordHeaderLen:]
	assert.Equal(t, byte(tlsHandshakeClientHello), payload[0], "handshake type should be ClientHello")

	// Handshake length (3 bytes).
	hsLen := int(payload[1])<<16 | int(payload[2])<<8 | int(payload[3])
	assert.Equal(t, len(payload)-4, hsLen, "handshake length mismatch")

	// Client version.
	assert.Equal(t, byte(0x03), payload[4])
	assert.Equal(t, byte(0x03), payload[5], "client version should be TLS 1.2")

	// Random (32 bytes) at offset 6.
	// Session ID length at offset 38.
	require.True(t, len(payload) > 38)
	sidLen := int(payload[38])
	assert.Equal(t, 32, sidLen, "session ID should be 32 bytes")
}

func TestBuildClientHello_ContainsSNI(t *testing.T) {
	sni := "test.example.org"
	hello := buildClientHello(sni)

	// The SNI should appear in the raw bytes.
	assert.True(t, bytes.Contains(hello, []byte(sni)),
		"ClientHello should contain the SNI hostname")
}

func TestBuildClientHello_ContainsALPN(t *testing.T) {
	hello := buildClientHello("example.com")
	assert.True(t, bytes.Contains(hello, []byte("h2")), "should contain h2 ALPN")
	assert.True(t, bytes.Contains(hello, []byte("http/1.1")), "should contain http/1.1 ALPN")
}

func TestBuildClientHello_RealisticSize(t *testing.T) {
	hello := buildClientHello("www.google.com")
	// Real ClientHello is typically 300-600 bytes.
	assert.True(t, len(hello) >= 250, "ClientHello too small: %d", len(hello))
	assert.True(t, len(hello) <= 800, "ClientHello too large: %d", len(hello))
}

// ---------------------------------------------------------------------------
// ServerHello structure tests
// ---------------------------------------------------------------------------

func TestBuildServerHello_HasValidTLSStructure(t *testing.T) {
	sessionID := make([]byte, 32)
	sh := buildServerHello(sessionID)

	require.True(t, len(sh) > tlsRecordHeaderLen)
	assert.Equal(t, byte(tlsRecordHandshake), sh[0])
	recordLen := int(binary.BigEndian.Uint16(sh[3:5]))
	assert.Equal(t, len(sh)-tlsRecordHeaderLen, recordLen)

	payload := sh[tlsRecordHeaderLen:]
	assert.Equal(t, byte(tlsHandshakeServerHello), payload[0])
}

func TestBuildServerHello_EchoesSessionID(t *testing.T) {
	sessionID := bytes.Repeat([]byte{0xAB}, 32)
	sh := buildServerHello(sessionID)
	assert.True(t, bytes.Contains(sh, sessionID), "ServerHello should echo the session ID")
}

// ---------------------------------------------------------------------------
// TLS record framing tests
// ---------------------------------------------------------------------------

func TestMakeTLSRecord_Structure(t *testing.T) {
	payload := []byte("hello")
	rec := makeTLSRecord(tlsRecordApplicationData, payload)

	assert.Equal(t, byte(tlsRecordApplicationData), rec[0])
	assert.Equal(t, byte(0x03), rec[1])
	assert.Equal(t, byte(0x03), rec[2])
	recLen := binary.BigEndian.Uint16(rec[3:5])
	assert.Equal(t, uint16(5), recLen)
	assert.Equal(t, payload, rec[tlsRecordHeaderLen:])
}

func TestReadTLSRecord_RoundTrip(t *testing.T) {
	original := []byte("test payload data")
	record := makeTLSRecord(tlsRecordApplicationData, original)

	payload, err := readTLSRecord(bytes.NewReader(record), tlsRecordApplicationData)
	require.NoError(t, err)
	assert.Equal(t, original, payload)
}

func TestReadTLSRecord_WrongType(t *testing.T) {
	record := makeTLSRecord(tlsRecordHandshake, []byte("data"))

	_, err := readTLSRecord(bytes.NewReader(record), tlsRecordApplicationData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected TLS record type")
}

// ---------------------------------------------------------------------------
// Camouflage handshake + data round-trip
// ---------------------------------------------------------------------------

func TestCamouflageConn_HandshakeAndDataRoundTrip(t *testing.T) {
	clientRaw, serverRaw := newPipePair()

	var (
		wg         sync.WaitGroup
		clientConn *CamouflageConn
		serverConn *CamouflageConn
		clientErr  error
		serverErr  error
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		clientConn, clientErr = newCamouflageConn(clientRaw, true, "test.example.com")
	}()
	go func() {
		defer wg.Done()
		serverConn, serverErr = newCamouflageConn(serverRaw, false, "")
	}()
	wg.Wait()

	require.NoError(t, clientErr, "client handshake failed")
	require.NoError(t, serverErr, "server handshake failed")

	// Send data from client to server.
	testData := []byte("Hello from client via TLS camouflage!")
	go func() {
		_, err := clientConn.Write(testData)
		assert.NoError(t, err)
	}()

	buf := make([]byte, 1024)
	n, err := serverConn.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, testData, buf[:n])

	// Send data from server to client.
	replyData := []byte("Server reply over camouflaged connection")
	go func() {
		_, err := serverConn.Write(replyData)
		assert.NoError(t, err)
	}()

	n, err = clientConn.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, replyData, buf[:n])

	_ = clientConn.Close()
	_ = serverConn.Close()
}

func TestCamouflageConn_LargeDataTransfer(t *testing.T) {
	clientRaw, serverRaw := newPipePair()

	var wg sync.WaitGroup
	var clientConn, serverConn *CamouflageConn
	var clientErr, serverErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		clientConn, clientErr = newCamouflageConn(clientRaw, true, "cdn.example.com")
	}()
	go func() {
		defer wg.Done()
		serverConn, serverErr = newCamouflageConn(serverRaw, false, "")
	}()
	wg.Wait()

	require.NoError(t, clientErr)
	require.NoError(t, serverErr)

	// Send data larger than one TLS record (>16KB).
	bigData := bytes.Repeat([]byte("X"), 32*1024)

	go func() {
		_, err := clientConn.Write(bigData)
		assert.NoError(t, err)
	}()

	received := make([]byte, 0, len(bigData))
	buf := make([]byte, 4096)
	for len(received) < len(bigData) {
		n, err := serverConn.Read(buf)
		require.NoError(t, err)
		received = append(received, buf[:n]...)
	}

	assert.Equal(t, bigData, received)

	_ = clientConn.Close()
	_ = serverConn.Close()
}

func TestCamouflageConn_WireTrafficLooksTLS(t *testing.T) {
	// Use a recording conn to inspect raw bytes on the wire.
	clientRaw, serverRaw := newPipePair()
	recorder := &recordingConn{pipeConn: clientRaw}

	var wg sync.WaitGroup
	var clientConn *CamouflageConn
	var clientErr, serverErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		clientConn, clientErr = newCamouflageConn(recorder, true, "www.google.com")
	}()
	go func() {
		defer wg.Done()
		_, serverErr = newCamouflageConn(serverRaw, false, "")
	}()
	wg.Wait()

	require.NoError(t, clientErr)
	require.NoError(t, serverErr)

	// Verify the first bytes written were a TLS Handshake record (ClientHello).
	firstWrite := recorder.getWrite(0)
	require.True(t, len(firstWrite) >= tlsRecordHeaderLen,
		"first write too short: %d", len(firstWrite))
	assert.Equal(t, byte(tlsRecordHandshake), firstWrite[0],
		"first byte on wire should be TLS Handshake content type (0x16)")

	// Send application data and verify it's wrapped in TLS Application Data.
	recorder.resetWrites()
	go func() {
		_, _ = clientConn.Write([]byte("application payload"))
	}()

	// Read from server side to unblock the pipe.
	buf := make([]byte, 1024)
	_, _ = io.ReadAtLeast(serverRaw.Conn, buf, 1)

	writes := recorder.allWriteBytes()
	require.True(t, len(writes) >= tlsRecordHeaderLen)
	assert.Equal(t, byte(tlsRecordApplicationData), writes[0],
		"application data should be wrapped in TLS Application Data record (0x17)")

	_ = clientConn.Close()
}

func TestExtractSessionID(t *testing.T) {
	hello := buildClientHello("example.com")
	payload := hello[tlsRecordHeaderLen:]

	sid := extractSessionID(payload)
	assert.Equal(t, 32, len(sid), "session ID should be 32 bytes")
	assert.False(t, bytes.Equal(sid, make([]byte, 32)),
		"session ID should not be all zeros (it's random)")
}

func TestCamouflageConn_EmptyWrite(t *testing.T) {
	clientRaw, serverRaw := newPipePair()

	var wg sync.WaitGroup
	var clientConn *CamouflageConn
	var clientErr, serverErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		clientConn, clientErr = newCamouflageConn(clientRaw, true, "example.com")
	}()
	go func() {
		defer wg.Done()
		_, serverErr = newCamouflageConn(serverRaw, false, "")
	}()
	wg.Wait()

	require.NoError(t, clientErr)
	require.NoError(t, serverErr)

	n, err := clientConn.Write(nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, n)

	n, err = clientConn.Write([]byte{})
	assert.NoError(t, err)
	assert.Equal(t, 0, n)

	_ = clientConn.Close()
}

// ---------------------------------------------------------------------------
// recordingConn – records writes for inspection
// ---------------------------------------------------------------------------

type recordingConn struct {
	*pipeConn
	mu     sync.Mutex
	writes [][]byte
}

func (r *recordingConn) Write(b []byte) (int, error) {
	r.mu.Lock()
	cp := make([]byte, len(b))
	copy(cp, b)
	r.writes = append(r.writes, cp)
	r.mu.Unlock()
	return r.pipeConn.Write(b)
}

func (r *recordingConn) getWrite(i int) []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i >= len(r.writes) {
		return nil
	}
	return r.writes[i]
}

func (r *recordingConn) resetWrites() {
	r.mu.Lock()
	r.writes = nil
	r.mu.Unlock()
}

func (r *recordingConn) allWriteBytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []byte
	for _, w := range r.writes {
		out = append(out, w...)
	}
	return out
}

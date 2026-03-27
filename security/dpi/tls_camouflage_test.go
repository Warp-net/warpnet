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
	"crypto/x509"
	"net"
	"sync"
	"testing"
	"time"

	ma "github.com/multiformats/go-multiaddr"
	utls "github.com/refraction-networking/utls"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pipeConn struct {
	net.Conn

	local  ma.Multiaddr
	remote ma.Multiaddr
}

func (p *pipeConn) LocalMultiaddr() ma.Multiaddr  { return p.local }
func (p *pipeConn) RemoteMultiaddr() ma.Multiaddr { return p.remote }

func newPipePair() (*pipeConn, *pipeConn) {
	a, b := net.Pipe()

	// Use distinct, non-nil multiaddrs so tests can verify delegation.
	ma1, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1")
	ma2, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/2")

	return &pipeConn{Conn: a, local: ma1, remote: ma2},
		&pipeConn{Conn: b, local: ma2, remote: ma1}
}

// testCamoConfig builds a camouflageConfig suitable for tests.
func testCamoConfig(sni string) *camouflageConfig {
	if sni == "" {
		sni = "test.example.com"
	}
	cache := &certCache{}
	serverCfg, err := buildServerTLSConfig(sni, defaultALPNProtos, cache)
	if err != nil {
		panic(err)
	}
	return &camouflageConfig{
		sni:              sni,
		alpnProtos:       defaultALPNProtos,
		clientHelloID:    utls.HelloChrome_Auto,
		handshakeTimeout: 10 * time.Second,
		serverTLSConfig:  serverCfg,
	}
}

func TestGenerateCertChain_ValidStructure(t *testing.T) {
	cert, err := generateCertChain("test.example.com")
	require.NoError(t, err)

	// Should have two certificates: leaf + CA.
	require.Len(t, cert.Certificate, 2, "cert chain should have leaf + CA")

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(cert.Certificate[1])
	require.NoError(t, err)

	// CA cert checks.
	assert.True(t, ca.IsCA, "CA cert should have IsCA=true")
	assert.Contains(t, ca.Subject.Organization, "Cloudflare, Inc.")

	// Leaf cert checks.
	assert.False(t, leaf.IsCA, "leaf cert should not be CA")
	assert.Equal(t, "test.example.com", leaf.Subject.CommonName)
	assert.Contains(t, leaf.DNSNames, "test.example.com")
	assert.Contains(t, leaf.ExtKeyUsage, x509.ExtKeyUsageServerAuth)

	// Leaf should be signed by CA.
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	_, err = leaf.Verify(x509.VerifyOptions{Roots: roots})
	require.NoError(t, err, "leaf cert should be verifiable against CA")
}

func TestGenerateCertChain_PlausibleDates(t *testing.T) {
	cert, err := generateCertChain("example.com")
	require.NoError(t, err)

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)

	now := time.Now()
	assert.True(t, leaf.NotBefore.Before(now), "leaf NotBefore should be in the past")
	assert.True(t, leaf.NotAfter.After(now), "leaf NotAfter should be in the future")

	// Validity period should be roughly 1 year.
	validity := leaf.NotAfter.Sub(leaf.NotBefore)
	assert.True(t, validity > 300*24*time.Hour, "cert validity too short")
	assert.True(t, validity < 400*24*time.Hour, "cert validity too long")
}

func TestGenerateCertChain_DifferentSerials(t *testing.T) {
	cert1, err := generateCertChain("a.example.com")
	require.NoError(t, err)
	cert2, err := generateCertChain("b.example.com")
	require.NoError(t, err)

	leaf1, _ := x509.ParseCertificate(cert1.Certificate[0])
	leaf2, _ := x509.ParseCertificate(cert2.Certificate[0])
	assert.NotEqual(t, leaf1.SerialNumber, leaf2.SerialNumber,
		"different certs should have different serial numbers")
}

func TestBrowserToHelloID(t *testing.T) {
	assert.Equal(t, utls.HelloChrome_Auto, browserToHelloID(BrowserChrome))
	assert.Equal(t, utls.HelloFirefox_Auto, browserToHelloID(BrowserFirefox))
	assert.Equal(t, utls.HelloSafari_Auto, browserToHelloID(BrowserSafari))
	assert.Equal(t, utls.HelloEdge_Auto, browserToHelloID(BrowserEdge))
	// Unknown defaults to Chrome.
	assert.Equal(t, utls.HelloChrome_Auto, browserToHelloID("unknown"))
	assert.Equal(t, utls.HelloChrome_Auto, browserToHelloID(""))
}

func TestCamouflageConn_HandshakeAndDataRoundTrip(t *testing.T) {
	clientRaw, serverRaw := newPipePair()
	cfg := testCamoConfig("test.example.com")

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
		clientConn, clientErr = newCamouflageConn(clientRaw, true, cfg)
	}()
	go func() {
		defer wg.Done()
		serverConn, serverErr = newCamouflageConn(serverRaw, false, cfg)
	}()
	wg.Wait()

	require.NoError(t, clientErr, "client handshake failed")
	require.NoError(t, serverErr, "server handshake failed")

	// Send data from client to server.
	testData := []byte("Hello from client via real TLS camouflage!")
	go func() {
		_, err := clientConn.Write(testData)
		assert.NoError(t, err)
	}()

	buf := make([]byte, 1024)
	n, err := serverConn.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, testData, buf[:n])

	// Send data from server to client.
	replyData := []byte("Server reply over TLS tunnel")
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
	cfg := testCamoConfig("cdn.example.com")

	var wg sync.WaitGroup
	var clientConn, serverConn *CamouflageConn
	var clientErr, serverErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		clientConn, clientErr = newCamouflageConn(clientRaw, true, cfg)
	}()
	go func() {
		defer wg.Done()
		serverConn, serverErr = newCamouflageConn(serverRaw, false, cfg)
	}()
	wg.Wait()

	require.NoError(t, clientErr)
	require.NoError(t, serverErr)

	// Send 64KB of data through the TLS tunnel.
	bigData := bytes.Repeat([]byte("X"), 64*1024)

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

func TestCamouflageConn_WireTrafficIsRealTLS(t *testing.T) {
	clientRaw, serverRaw := newPipePair()
	cfg := testCamoConfig("www.google.com")
	recorder := &recordingConn{pipeConn: clientRaw}

	var wg sync.WaitGroup
	var serverErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = newCamouflageConn(recorder, true, cfg)
	}()
	go func() {
		defer wg.Done()
		_, serverErr = newCamouflageConn(serverRaw, false, cfg)
	}()
	wg.Wait()

	require.NoError(t, serverErr)

	// Verify the first bytes written were a TLS ClientHello (0x16).
	firstWrite := recorder.getWrite(0)
	require.True(t, len(firstWrite) >= 5,
		"first write too short: %d", len(firstWrite))
	assert.Equal(t, byte(0x16), firstWrite[0],
		"first byte on wire should be TLS Handshake content type (0x16)")
	// TLS record version should be 0x0301 (TLS 1.0) or 0x0303 (TLS 1.2)
	// in the record header – uTLS mimics this from the browser.
	assert.Equal(t, byte(0x03), firstWrite[1],
		"TLS major version should be 3")
}

func TestValidateALPN(t *testing.T) {
	allowed := []string{"h2", "http/1.1"}

	assert.True(t, validateALPN("h2", allowed))
	assert.True(t, validateALPN("http/1.1", allowed))
	assert.True(t, validateALPN("", allowed)) // empty is always OK
	assert.False(t, validateALPN("mqtt", allowed))
	assert.False(t, validateALPN("grpc", allowed))
}

func TestCertCache_ReusesForSameSNI(t *testing.T) {
	cache := &certCache{}

	cert1, err := cache.getOrGenerate("test.example.com")
	require.NoError(t, err)
	cert2, err := cache.getOrGenerate("test.example.com")
	require.NoError(t, err)

	// Same SNI should return the same certificate.
	assert.Equal(t, cert1.Certificate, cert2.Certificate)
}

func TestCertCache_RegeneratesForDifferentSNI(t *testing.T) {
	cache := &certCache{}

	cert1, err := cache.getOrGenerate("a.example.com")
	require.NoError(t, err)
	cert2, err := cache.getOrGenerate("b.example.com")
	require.NoError(t, err)

	// Different SNI should produce different certificates.
	assert.NotEqual(t, cert1.Certificate, cert2.Certificate)
}

func TestCamouflageConn_PreservesMultiaddr(t *testing.T) {
	clientRaw, serverRaw := newPipePair()
	cfg := testCamoConfig("example.com")

	var wg sync.WaitGroup
	var clientConn *CamouflageConn
	var clientErr, serverErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		clientConn, clientErr = newCamouflageConn(clientRaw, true, cfg)
	}()
	go func() {
		defer wg.Done()
		_, serverErr = newCamouflageConn(serverRaw, false, cfg)
	}()
	wg.Wait()

	require.NoError(t, clientErr)
	require.NoError(t, serverErr)

	// CamouflageConn should delegate multiaddr to the raw connection.
	require.NotNil(t, clientRaw.LocalMultiaddr(), "test setup: raw local multiaddr must be non-nil")
	require.NotNil(t, clientRaw.RemoteMultiaddr(), "test setup: raw remote multiaddr must be non-nil")
	assert.Equal(t, clientRaw.LocalMultiaddr(), clientConn.LocalMultiaddr())
	assert.Equal(t, clientRaw.RemoteMultiaddr(), clientConn.RemoteMultiaddr())

	_ = clientConn.Close()
}

func TestCamouflageConn_EmptyWrite(t *testing.T) {
	clientRaw, serverRaw := newPipePair()
	cfg := testCamoConfig("example.com")

	var wg sync.WaitGroup
	var clientConn *CamouflageConn
	var clientErr, serverErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		clientConn, clientErr = newCamouflageConn(clientRaw, true, cfg)
	}()
	go func() {
		defer wg.Done()
		_, serverErr = newCamouflageConn(serverRaw, false, cfg)
	}()
	wg.Wait()

	require.NoError(t, clientErr)
	require.NoError(t, serverErr)

	// TLS handles empty writes gracefully.
	n, err := clientConn.Write(nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, n)

	n, err = clientConn.Write([]byte{})
	assert.NoError(t, err)
	assert.Equal(t, 0, n)

	_ = clientConn.Close()
}

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

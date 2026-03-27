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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"slices"
	"sync"
	"time"

	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	utls "github.com/refraction-networking/utls"
	log "github.com/sirupsen/logrus"
)

const (
	defaultSNI              = "www.googleapis.com"
	defaultHandshakeTimeout = 10 * time.Second
)

// Well-known browser fingerprint identifiers for WithBrowserFingerprint.
const (
	BrowserChrome  = "chrome"
	BrowserFirefox = "firefox"
	BrowserSafari  = "safari"
	BrowserEdge    = "edge"
	BrowserIOS     = "ios"
	BrowserAndroid = "android"
)

var defaultALPNProtos = []string{"h2", "http/1.1"}

// Sentinel errors for TLS camouflage.
var (
	errHandshakeFailed = errors.New("dpi: TLS handshake failed")
	errALPNMismatch    = errors.New("dpi: ALPN protocol not in allowed set")
)

// camouflageConfig holds the TLS camouflage settings shared by all
// connections created by a single SpoofTransport instance.
type camouflageConfig struct {
	sni              string
	alpnProtos       []string
	clientHelloID    utls.ClientHelloID
	handshakeTimeout time.Duration
	serverTLSConfig  *tls.Config // generated once per transport
}

// CamouflageConn wraps a TCP connection with a real TLS tunnel.
// The client side uses uTLS to present a genuine browser TLS fingerprint
// (Chrome, Firefox, etc.), while the server side uses standard crypto/tls
// with a plausible certificate chain. All traffic inside the tunnel is
// indistinguishable from normal HTTPS browsing to DPI middleboxes.
type CamouflageConn struct {
	tlsConn net.Conn   // *utls.UConn (client) or *tls.Conn (server)
	rawConn manet.Conn // underlying manet.Conn for multiaddr info
}

var _ manet.Conn = (*CamouflageConn)(nil)

func (cc *CamouflageConn) Read(b []byte) (int, error)         { return cc.tlsConn.Read(b) }
func (cc *CamouflageConn) Write(b []byte) (int, error)        { return cc.tlsConn.Write(b) }
func (cc *CamouflageConn) Close() error                       { return cc.tlsConn.Close() }
func (cc *CamouflageConn) LocalAddr() net.Addr                { return cc.tlsConn.LocalAddr() }
func (cc *CamouflageConn) RemoteAddr() net.Addr               { return cc.tlsConn.RemoteAddr() }
func (cc *CamouflageConn) SetDeadline(t time.Time) error      { return cc.tlsConn.SetDeadline(t) }
func (cc *CamouflageConn) SetReadDeadline(t time.Time) error  { return cc.tlsConn.SetReadDeadline(t) }
func (cc *CamouflageConn) SetWriteDeadline(t time.Time) error { return cc.tlsConn.SetWriteDeadline(t) }
func (cc *CamouflageConn) LocalMultiaddr() ma.Multiaddr       { return cc.rawConn.LocalMultiaddr() }
func (cc *CamouflageConn) RemoteMultiaddr() ma.Multiaddr      { return cc.rawConn.RemoteMultiaddr() }

// CloseRead forwards to the underlying connection if supported.
func (cc *CamouflageConn) CloseRead() error {
	if cr, ok := cc.tlsConn.(interface{ CloseRead() error }); ok {
		return cr.CloseRead()
	}
	return nil
}

// CloseWrite sends a TLS close_notify alert if the underlying TLS
// connection supports half-close.
func (cc *CamouflageConn) CloseWrite() error {
	if cw, ok := cc.tlsConn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

// newCamouflageConn wraps conn with a real TLS tunnel and performs the
// TLS handshake. isClient determines whether this side initiates the
// handshake (uTLS with browser fingerprint) or accepts (crypto/tls server).
func newCamouflageConn(conn manet.Conn, isClient bool, cfg *camouflageConfig) (*CamouflageConn, error) {
	// Set a deadline for the handshake to defend against slow-handshake
	// active probing attacks.
	if err := conn.SetDeadline(time.Now().Add(cfg.handshakeTimeout)); err != nil {
		log.Debugf("dpi: set handshake deadline: %v", err)
	}

	var tlsConn net.Conn
	var err error
	if isClient {
		tlsConn, err = clientTLSHandshake(conn, cfg)
	} else {
		tlsConn, err = serverTLSHandshake(conn, cfg)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errHandshakeFailed, err.Error())
	}

	// Clear the handshake deadline so subsequent I/O is not constrained.
	if err := conn.SetDeadline(time.Time{}); err != nil {
		log.Debugf("dpi: clear handshake deadline: %v", err)
	}

	return &CamouflageConn{tlsConn: tlsConn, rawConn: conn}, nil
}

// clientTLSHandshake performs a TLS handshake using uTLS with a browser
// ClientHello fingerprint. The resulting connection is indistinguishable
// from a real browser HTTPS session to passive DPI observers.
func clientTLSHandshake(conn net.Conn, cfg *camouflageConfig) (net.Conn, error) {
	utlsConfig := &utls.Config{
		ServerName: cfg.sni,
		NextProtos: cfg.alpnProtos,
		// We verify peer identity via the Noise handshake inside TLS
		// so certificate verification is intentionally skipped here.
		InsecureSkipVerify: true, //nolint:gosec // identity verified by Noise
	}

	uconn := utls.UClient(conn, utlsConfig, cfg.clientHelloID)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.handshakeTimeout)
	defer cancel()
	if err := uconn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("uTLS handshake: %w", err)
	}

	return uconn, nil
}

var errConfigNotInit = errors.New("dpi: server TLS  config not initialized") //nolint:all

// serverTLSHandshake accepts a TLS connection using standard crypto/tls
// with a plausible certificate chain.
func serverTLSHandshake(conn net.Conn, cfg *camouflageConfig) (net.Conn, error) {
	if cfg.serverTLSConfig == nil {
		return nil, errConfigNotInit //nolint:all
	}

	tlsConn := tls.Server(conn, cfg.serverTLSConfig)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.handshakeTimeout)
	defer cancel()
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("server TLS handshake: %w", err)
	}

	// Active probing defense: validate that the negotiated ALPN protocol
	// is in the expected set. A non-browser probe may negotiate an
	// unexpected protocol or none at all.
	state := tlsConn.ConnectionState()
	if !validateALPN(state.NegotiatedProtocol, cfg.alpnProtos) {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("%w: got %q", errALPNMismatch, state.NegotiatedProtocol)
	}

	return tlsConn, nil
}

// validateALPN checks whether the negotiated ALPN protocol is in the
// allowed set. An empty negotiated protocol is always accepted (some
// legitimate clients omit ALPN).
func validateALPN(negotiated string, allowed []string) bool {
	if negotiated == "" {
		return true
	}
	if slices.Contains(allowed, negotiated) {
		return true
	}
	return false
}

// browserToHelloID maps a human-readable browser name to a uTLS
// ClientHelloID preset. Unknown names fall back to Chrome.
func browserToHelloID(browser string) utls.ClientHelloID {
	switch browser {
	case BrowserChrome:
		return utls.HelloChrome_Auto
	case BrowserFirefox:
		return utls.HelloFirefox_Auto
	case BrowserSafari:
		return utls.HelloSafari_Auto
	case BrowserEdge:
		return utls.HelloEdge_Auto
	case BrowserIOS:
		return utls.HelloIOS_Auto
	case BrowserAndroid:
		return utls.HelloAndroid_11_OkHttp
	default:
		return utls.HelloChrome_Auto
	}
}

// certCache caches a generated TLS certificate to avoid expensive key
// generation on every connection. One certificate per SpoofTransport.
type certCache struct {
	mu   sync.Mutex
	cert *tls.Certificate
	sni  string
}

func (c *certCache) getOrGenerate(sni string) (tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cert != nil && c.sni == sni {
		return *c.cert, nil
	}

	cert, err := generateCertChain(sni)
	if err != nil {
		return tls.Certificate{}, err
	}
	c.cert = &cert
	c.sni = sni
	return cert, nil
}

// generateCertChain creates a two-certificate chain (fake CA + leaf) that
// structurally resembles certificates issued by a major CA. The chain
// makes the TLS session look more realistic to DPI systems that inspect
// certificate metadata.
func generateCertChain(sni string) (tls.Certificate, error) {
	// --- CA certificate ---
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate CA key: %w", err)
	}

	caSerial, err := randomSerial()
	if err != nil {
		return tls.Certificate{}, err
	}

	caTemplate := &x509.Certificate{
		SerialNumber: caSerial,
		Subject: pkix.Name{
			CommonName:   "Cloudflare Inc ECC CA-3",
			Organization: []string{"Cloudflare, Inc."},
			Country:      []string{"US"},
		},
		NotBefore:             time.Now().Add(-365 * 24 * time.Hour),
		NotAfter:              time.Now().Add(5 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create CA certificate: %w", err)
	}

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse CA certificate: %w", err)
	}

	// --- Leaf certificate ---
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate leaf key: %w", err)
	}

	leafSerial, err := randomSerial()
	if err != nil {
		return tls.Certificate{}, err
	}

	leafTemplate := &x509.Certificate{
		SerialNumber: leafSerial,
		Subject: pkix.Name{
			CommonName:   sni,
			Organization: []string{"Cloudflare, Inc."},
			Country:      []string{"US"},
			Province:     []string{"California"},
			Locality:     []string{"San Francisco"},
		},
		DNSNames:              []string{sni},
		NotBefore:             time.Now().Add(-30 * 24 * time.Hour),
		NotAfter:              time.Now().Add(335 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	leafCertDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create leaf certificate: %w", err)
	}

	return tls.Certificate{
		Certificate: [][]byte{leafCertDER, caCertDER},
		PrivateKey:  leafKey,
	}, nil
}

// randomSerial generates a 128-bit random serial number for X.509
// certificates.
func randomSerial() (*big.Int, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %w", err)
	}
	return serial, nil
}

// buildServerTLSConfig creates the crypto/tls configuration for the
// server side, including the generated certificate chain and ALPN
// protocol list.
func buildServerTLSConfig(sni string, alpn []string, cache *certCache) (*tls.Config, error) {
	cert, err := cache.getOrGenerate(sni)
	if err != nil {
		return nil, fmt.Errorf("generate server certificate: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   alpn,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

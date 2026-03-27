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
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	manet "github.com/multiformats/go-multiaddr/net"
	log "github.com/sirupsen/logrus"
)

const (
	tlsRecordHandshake        = 0x16
	tlsRecordChangeCipherSpec = 0x14
	tlsRecordApplicationData  = 0x17

	tlsHandshakeClientHello = 0x01
	tlsHandshakeServerHello = 0x02

	tlsVersionTLS12 uint16 = 0x0303
	tlsVersionTLS13 uint16 = 0x0304

	tlsRecordHeaderLen  = 5
	maxTLSRecordPayload = 16384 // 2^14

	defaultSNI = "www.googleapis.com"
)

// Sentinel errors for TLS record parsing.
var (
	errUnexpectedRecordType = errors.New("unexpected TLS record type")
	errRecordTooLarge       = errors.New("TLS record too large")
)

// CamouflageConn wraps a connection with TLS record framing so that
// traffic appears to be a standard TLS 1.3 session to DPI middleboxes.
// The actual encryption is handled by the Noise protocol running inside
// TLS Application Data records.
type CamouflageConn struct {
	manet.Conn

	mu      sync.Mutex
	readBuf bytes.Buffer
}

// newCamouflageConn wraps conn with TLS camouflage and performs the
// fake TLS handshake. isClient determines whether this side initiates
// the handshake (ClientHello) or responds (ServerHello).
func newCamouflageConn(conn manet.Conn, isClient bool, sni string) (*CamouflageConn, error) {
	if sni == "" {
		sni = defaultSNI
	}
	cc := &CamouflageConn{Conn: conn}

	var err error
	if isClient {
		err = cc.clientHandshake(sni)
	} else {
		err = cc.serverHandshake()
	}
	if err != nil {
		return nil, fmt.Errorf("dpi: TLS camouflage handshake: %w", err)
	}
	return cc, nil
}

// Write wraps payload in TLS Application Data records.
func (cc *CamouflageConn) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}

	total := 0
	for len(b) > 0 {
		chunk := b
		if len(chunk) > maxTLSRecordPayload {
			chunk = b[:maxTLSRecordPayload]
		}

		record := makeTLSRecord(tlsRecordApplicationData, chunk)
		if err := writeAll(cc.Conn, record); err != nil {
			return total, err
		}
		total += len(chunk)
		b = b[len(chunk):]
	}
	return total, nil
}

// Read unwraps TLS Application Data records, returning the inner payload.
func (cc *CamouflageConn) Read(b []byte) (int, error) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.readBuf.Len() > 0 {
		return cc.readBuf.Read(b)
	}

	payload, err := readTLSRecord(cc.Conn, tlsRecordApplicationData)
	if err != nil {
		return 0, err
	}

	n := copy(b, payload)
	if n < len(payload) {
		_, _ = cc.readBuf.Write(payload[n:])
	}
	return n, nil
}

// CloseRead forwards to the underlying connection if supported.
func (cc *CamouflageConn) CloseRead() error {
	if cr, ok := cc.Conn.(interface{ CloseRead() error }); ok {
		return cr.CloseRead()
	}
	return nil
}

// CloseWrite forwards to the underlying connection if supported.
func (cc *CamouflageConn) CloseWrite() error {
	if cw, ok := cc.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Fake TLS handshake
// ---------------------------------------------------------------------------

func (cc *CamouflageConn) clientHandshake(sni string) error {
	hello := buildClientHello(sni)
	ccs := makeTLSRecord(tlsRecordChangeCipherSpec, []byte{0x01})

	if err := writeAll(cc.Conn, hello); err != nil {
		return err
	}
	if err := writeAll(cc.Conn, ccs); err != nil {
		return err
	}
	if _, err := readTLSRecord(cc.Conn, tlsRecordHandshake); err != nil {
		return err
	}
	if _, err := readTLSRecord(cc.Conn, tlsRecordChangeCipherSpec); err != nil {
		return err
	}
	return nil
}

func (cc *CamouflageConn) serverHandshake() error {
	chPayload, err := readTLSRecord(cc.Conn, tlsRecordHandshake)
	if err != nil {
		return err
	}
	if _, err = readTLSRecord(cc.Conn, tlsRecordChangeCipherSpec); err != nil {
		return err
	}

	sessionID := extractSessionID(chPayload)
	sh := buildServerHello(sessionID)
	ccs := makeTLSRecord(tlsRecordChangeCipherSpec, []byte{0x01})

	if err = writeAll(cc.Conn, sh); err != nil {
		return err
	}
	if err = writeAll(cc.Conn, ccs); err != nil {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// TLS record helpers
// ---------------------------------------------------------------------------

// readTLSRecord reads a single TLS record from r and validates the content type.
func readTLSRecord(r io.Reader, expectedType byte) ([]byte, error) {
	var hdr [tlsRecordHeaderLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, fmt.Errorf("read TLS record header: %w", err)
	}

	if hdr[0] != expectedType {
		return nil, fmt.Errorf("%w: got 0x%02x, want 0x%02x", errUnexpectedRecordType, hdr[0], expectedType)
	}

	length := int(binary.BigEndian.Uint16(hdr[3:5]))
	if length > maxTLSRecordPayload+2048 {
		return nil, fmt.Errorf("%w: %d", errRecordTooLarge, length)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("read TLS record payload: %w", err)
	}
	return payload, nil
}

// makeTLSRecord constructs a TLS record with the given content type.
func makeTLSRecord(contentType byte, payload []byte) []byte {
	record := make([]byte, tlsRecordHeaderLen+len(payload))
	record[0] = contentType
	binary.BigEndian.PutUint16(record[1:3], tlsVersionTLS12) // record-layer version
	binary.BigEndian.PutUint16(record[3:5], uint16(len(payload)))
	copy(record[tlsRecordHeaderLen:], payload)
	return record
}

// ---------------------------------------------------------------------------
// ClientHello / ServerHello construction
//
// All bytes.Buffer write methods are infallible (only OOM panics), so
// errors are explicitly discarded with _, _ = to satisfy errcheck.
// ---------------------------------------------------------------------------

// buildClientHello constructs a realistic TLS 1.3 ClientHello wrapped
// in a TLS Handshake record.
func buildClientHello(sni string) []byte {
	var msg bytes.Buffer

	// Handshake header: type(1) + length(3).
	_ = msg.WriteByte(tlsHandshakeClientHello)
	_, _ = msg.Write([]byte{0, 0, 0}) // length placeholder

	bodyStart := msg.Len()

	// Client version: TLS 1.2 (TLS 1.3 negotiated via extension).
	putUint16(&msg, tlsVersionTLS12)

	// Random (32 bytes).
	writeRandom(&msg)

	// Session ID (32 bytes – TLS 1.3 compatibility mode).
	sessionID := makeRandom(32)
	_ = msg.WriteByte(32)
	_, _ = msg.Write(sessionID)

	// Cipher suites.
	suites := []uint16{
		0x1301, 0x1302, 0x1303, // TLS 1.3
		0xc02b, 0xc02f, 0xc02c, 0xc030, // ECDHE-GCM
		0xcca9, 0xcca8, // CHACHA20
	}
	putUint16(&msg, uint16(len(suites)*2))
	for _, s := range suites {
		putUint16(&msg, s)
	}

	// Compression methods.
	_ = msg.WriteByte(1)
	_ = msg.WriteByte(0x00)

	// Extensions.
	var exts bytes.Buffer
	writeSNIExt(&exts, sni)
	writeSupportedVersionsExt(&exts)
	writeSignatureAlgorithmsExt(&exts)
	writeSupportedGroupsExt(&exts)
	writeKeyShareClientExt(&exts)
	writeALPNExt(&exts)
	writeECPointFormatsExt(&exts)
	writeEmptyExt(&exts, 0x0023) // session ticket
	writeEmptyExt(&exts, 0x0017) // extended master secret
	writeRenegotiationInfoExt(&exts)

	putUint16(&msg, uint16(exts.Len()))
	_, _ = msg.Write(exts.Bytes())

	// Patch handshake length.
	raw := msg.Bytes()
	patchHandshakeLen(raw, bodyStart)

	return makeTLSRecord(tlsRecordHandshake, raw)
}

// buildServerHello constructs a TLS 1.3 ServerHello wrapped in a TLS
// Handshake record. sessionID is echoed from the ClientHello.
func buildServerHello(sessionID []byte) []byte {
	var msg bytes.Buffer

	_ = msg.WriteByte(tlsHandshakeServerHello)
	_, _ = msg.Write([]byte{0, 0, 0})

	bodyStart := msg.Len()

	putUint16(&msg, tlsVersionTLS12) // legacy version

	writeRandom(&msg)

	if len(sessionID) == 0 {
		sessionID = makeRandom(32)
	}
	_ = msg.WriteByte(byte(len(sessionID)))
	_, _ = msg.Write(sessionID)

	putUint16(&msg, 0x1301) // TLS_AES_128_GCM_SHA256
	_ = msg.WriteByte(0x00) // null compression

	// Extensions.
	var exts bytes.Buffer
	// supported_versions → TLS 1.3
	putUint16(&exts, 0x002b)
	putUint16(&exts, 2)
	putUint16(&exts, tlsVersionTLS13)
	// key_share → x25519 with random key
	keyData := makeRandom(32)
	putUint16(&exts, 0x0033)
	putUint16(&exts, 2+2+32) // group(2)+keyLen(2)+key(32)
	putUint16(&exts, 0x001d) // x25519
	putUint16(&exts, 32)
	_, _ = exts.Write(keyData)

	putUint16(&msg, uint16(exts.Len()))
	_, _ = msg.Write(exts.Bytes())

	raw := msg.Bytes()
	patchHandshakeLen(raw, bodyStart)

	return makeTLSRecord(tlsRecordHandshake, raw)
}

// extractSessionID returns the session ID from a raw ClientHello
// handshake payload (without the TLS record header).
func extractSessionID(payload []byte) []byte {
	// type(1) + len(3) + version(2) + random(32) = offset 38
	const sidOffset = 1 + 3 + 2 + 32
	if len(payload) < sidOffset+1 {
		return makeRandom(32) // random fallback avoids zero-fingerprint
	}
	sidLen := int(payload[sidOffset])
	if len(payload) < sidOffset+1+sidLen {
		return makeRandom(32)
	}
	out := make([]byte, sidLen)
	copy(out, payload[sidOffset+1:])
	return out
}

// ---------------------------------------------------------------------------
// Extension writers
// ---------------------------------------------------------------------------

func writeSNIExt(buf *bytes.Buffer, sni string) {
	name := []byte(sni)
	listLen := 1 + 2 + len(name) // type(1) + nameLen(2) + name
	putUint16(buf, 0x0000)
	putUint16(buf, uint16(2+listLen)) // ext data len
	putUint16(buf, uint16(listLen))   // server name list len
	_ = buf.WriteByte(0x00)           // host_name type
	putUint16(buf, uint16(len(name)))
	_, _ = buf.Write(name)
}

func writeSupportedVersionsExt(buf *bytes.Buffer) {
	putUint16(buf, 0x002b)
	putUint16(buf, 5)     // data len
	_ = buf.WriteByte(4)  // versions list len
	putUint16(buf, tlsVersionTLS13)
	putUint16(buf, tlsVersionTLS12)
}

func writeSignatureAlgorithmsExt(buf *bytes.Buffer) {
	algos := []uint16{
		0x0403, 0x0503, 0x0603, // ECDSA
		0x0807, 0x0808, // EdDSA
		0x0804, 0x0805, 0x0806, // RSA-PSS
		0x0401, 0x0501, 0x0601, // RSA-PKCS1
	}
	putUint16(buf, 0x000d)
	putUint16(buf, uint16(2+len(algos)*2))
	putUint16(buf, uint16(len(algos)*2))
	for _, a := range algos {
		putUint16(buf, a)
	}
}

func writeSupportedGroupsExt(buf *bytes.Buffer) {
	groups := []uint16{0x001d, 0x0017, 0x0018, 0x0019} // x25519, P-256, P-384, P-521
	putUint16(buf, 0x000a)
	putUint16(buf, uint16(2+len(groups)*2))
	putUint16(buf, uint16(len(groups)*2))
	for _, g := range groups {
		putUint16(buf, g)
	}
}

func writeKeyShareClientExt(buf *bytes.Buffer) {
	key := makeRandom(32)
	entryLen := 2 + 2 + 32 // group(2) + keyLen(2) + key(32)
	putUint16(buf, 0x0033)
	putUint16(buf, uint16(2+entryLen)) // ext data len
	putUint16(buf, uint16(entryLen))   // client_shares len
	putUint16(buf, 0x001d)             // x25519
	putUint16(buf, 32)
	_, _ = buf.Write(key)
}

func writeALPNExt(buf *bytes.Buffer) {
	var list bytes.Buffer
	for _, p := range []string{"h2", "http/1.1"} {
		_ = list.WriteByte(byte(len(p)))
		_, _ = list.WriteString(p)
	}
	putUint16(buf, 0x0010)
	putUint16(buf, uint16(2+list.Len()))
	putUint16(buf, uint16(list.Len()))
	_, _ = buf.Write(list.Bytes())
}

func writeECPointFormatsExt(buf *bytes.Buffer) {
	putUint16(buf, 0x000b)
	putUint16(buf, 2)     // ext data len
	_ = buf.WriteByte(1)  // formats length
	_ = buf.WriteByte(0)  // uncompressed
}

func writeEmptyExt(buf *bytes.Buffer, extType uint16) {
	putUint16(buf, extType)
	putUint16(buf, 0)
}

func writeRenegotiationInfoExt(buf *bytes.Buffer) {
	putUint16(buf, 0xff01)
	putUint16(buf, 1)
	_ = buf.WriteByte(0)
}

// ---------------------------------------------------------------------------
// Tiny helpers
// ---------------------------------------------------------------------------

func putUint16(buf *bytes.Buffer, v uint16) {
	_, _ = buf.Write([]byte{byte(v >> 8), byte(v)})
}

func makeRandom(n int) []byte {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		log.Debugf("dpi: crypto/rand short read, falling back: %v", err)
		for i := range b {
			b[i] = byte(i * 37)
		}
	}
	return b
}

func writeRandom(buf *bytes.Buffer) {
	_, _ = buf.Write(makeRandom(32))
}

func patchHandshakeLen(raw []byte, bodyStart int) {
	bodyLen := len(raw) - bodyStart
	raw[1] = byte(bodyLen >> 16)
	raw[2] = byte(bodyLen >> 8)
	raw[3] = byte(bodyLen)
}

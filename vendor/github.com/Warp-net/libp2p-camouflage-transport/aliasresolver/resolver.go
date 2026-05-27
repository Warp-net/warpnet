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

// Package aliasresolver runs on relay peers. It maintains an in-memory
// table of WarpID -> owning peer, accepts signed registrations from
// listeners, and proxies inbound resolve requests onto the registered
// listener over a side-channel stream. The relay sees ciphertext only:
// dialer and listener run their own libp2p security handshake inside the
// forwarded stream.
//
// Wire format (deliberately tight, fixed-layout, JSON-free):
//
//	RegisterProtocol frame: [32-byte raw WarpID][2-byte BE sigLen][sig...]
//	ResolveProtocol  frame: [32-byte raw WarpID]
//	Status response:        [1 byte] 0x01=ok, anything else=denied
//
// The fixed prefix size makes framing trivial: readers consume exactly
// the bytes for one message and never over-read into the byte-piping
// phase that follows on the resolve channel.
package aliasresolver

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	ma "github.com/multiformats/go-multiaddr"
)

// Wire protocols spoken by the resolver. Versions are bumped if the
// register/resolve framing or fields change incompatibly.
const (
	RegisterProtocol protocol.ID = "/warpnet/alias-register/0.0.0"
	ResolveProtocol  protocol.ID = "/warpnet/alias-resolve/0.0.0"
	// StopProtocol is the stream the relay opens *back* to a registered
	// listener once a dialer asks to resolve its WarpID. Bytes are then
	// piped between dialer and listener; both ends run their own Noise
	// handshake inside.
	StopProtocol protocol.ID = "/warpnet/alias-stop/0.0.0"
)

// Wire format constants.
const (
	WarpIDByteLen = 32
	// MaxSigLen caps the signature length the resolver will accept on
	// the register frame. Generous enough for RSA-4096 (~512 bytes); a
	// hard ceiling stops a peer from making us allocate megabytes.
	MaxSigLen = 1024

	statusOK     = 0x01
	statusDenied = 0x00
)

const (
	handshakeTimeout = 10 * time.Second
)

// DefaultMaxEntries caps the WarpID table size. On a public relay this
// keeps a botnet from filling memory with bogus registrations. One
// owning peer may still occupy at most one slot at a time.
const DefaultMaxEntries = 10000

// Entry is a single row of the resolver table.
type Entry struct {
	Peer  peer.ID
	Owner crypto.PubKey
}

// Resolver keeps an in-memory WarpID -> peer mapping for a single relay
// host. A WarpID is owned by the public key that first registered it; a
// subsequent registration with a different key is rejected. Entries are
// evicted automatically when the owning peer fully disconnects.
type Resolver struct {
	host       host.Host
	maxEntries int

	mu     sync.RWMutex
	table  map[string]Entry // key: hex WarpID
	byPeer map[peer.ID]string
}

// New returns an unstarted Resolver. Call Start to wire up stream
// handlers and disconnect notifications on the host.
func New(h host.Host) *Resolver {
	return &Resolver{
		host:       h,
		maxEntries: DefaultMaxEntries,
		table:      make(map[string]Entry),
		byPeer:     make(map[peer.ID]string),
	}
}

// Start registers the resolver's stream handlers on the host and hooks
// into network notifications so disconnected peers' WarpIDs are evicted
// automatically. Safe to call once per Resolver.
func (r *Resolver) Start() {
	r.host.SetStreamHandler(RegisterProtocol, r.HandleRegister)
	r.host.SetStreamHandler(ResolveProtocol, r.HandleResolve)
	r.host.Network().Notify(r)
}

// Stop removes the resolver's stream handlers and clears the table.
func (r *Resolver) Stop() {
	r.host.RemoveStreamHandler(RegisterProtocol)
	r.host.RemoveStreamHandler(ResolveProtocol)
	r.host.Network().StopNotify(r)
	r.mu.Lock()
	r.table = make(map[string]Entry)
	r.byPeer = make(map[peer.ID]string)
	r.mu.Unlock()
}

// Lookup returns the registered entry for id, if any. Intended for tests
// and diagnostics.
func (r *Resolver) Lookup(id string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.table[id]
	return e, ok
}

// HandleRegister processes one register frame from a listener. The
// stream's RemotePublicKey is the source of truth for ownership;
// because the connection is libp2p-secure, the relay can trust it. The
// signature ties the WarpID to the same key, ruling out third parties
// who might capture and replay the payload over their own connection.
func (r *Resolver) HandleRegister(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(handshakeTimeout))

	pub := s.Conn().RemotePublicKey()
	if pub == nil {
		// Unsecured connections cannot prove ownership.
		_ = s.Reset()
		return
	}

	warpHex, sig, err := ReadRegisterFrame(s)
	if err != nil {
		log.Printf("aliasresolver: register decode from %s: %v", s.Conn().RemotePeer(), err)
		_ = s.Reset()
		return
	}

	// Verify the signature against the raw 32-byte WarpID, not its hex
	// rendering. The wire frame carried the raw bytes; we just
	// re-decoded what ReadRegisterFrame encoded, so the DecodeString
	// call here is infallible.
	idBytes, _ := hex.DecodeString(warpHex)
	ok, err := pub.Verify(idBytes, sig)
	if err != nil || !ok {
		log.Printf("aliasresolver: register sig invalid from %s", s.Conn().RemotePeer())
		_ = s.Reset()
		return
	}

	remote := s.Conn().RemotePeer()

	r.mu.Lock()
	if e, exists := r.table[warpHex]; exists && !e.Owner.Equals(pub) {
		r.mu.Unlock()
		log.Printf("aliasresolver: register conflict for id from %s", remote)
		_ = WriteStatus(s, false)
		return
	}
	// Reject net-new registrations once the table is full. Existing
	// entries (same WarpID, same owner) still update in place.
	if _, exists := r.table[warpHex]; !exists && len(r.table) >= r.maxEntries {
		r.mu.Unlock()
		log.Printf("aliasresolver: table full (%d), rejecting %s", r.maxEntries, remote)
		_ = WriteStatus(s, false)
		return
	}
	// One WarpID per owning peer: if this peer previously claimed a
	// different alias, retire the old one.
	if prev, ok := r.byPeer[remote]; ok && prev != warpHex {
		delete(r.table, prev)
	}
	r.table[warpHex] = Entry{Peer: remote, Owner: pub}
	r.byPeer[remote] = warpHex
	r.mu.Unlock()

	_ = WriteStatus(s, true)
}

// HandleResolve looks up the listener for the requested WarpID, opens
// an upstream StopProtocol stream to it, acknowledges the dialer, then
// pipes bytes both ways until either side closes.
func (r *Resolver) HandleResolve(s network.Stream) {
	_ = s.SetDeadline(time.Now().Add(handshakeTimeout))

	warpHex, err := ReadResolveFrame(s)
	if err != nil {
		log.Printf("aliasresolver: resolve decode from %s: %v", s.Conn().RemotePeer(), err)
		_ = s.Reset()
		return
	}

	r.mu.RLock()
	e, ok := r.table[warpHex]
	r.mu.RUnlock()
	if !ok {
		_ = WriteStatus(s, false)
		_ = s.Close()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
	upstream, err := r.host.NewStream(ctx, e.Peer, StopProtocol)
	cancel()
	if err != nil {
		log.Printf("aliasresolver: open stop stream to %s: %v", e.Peer, err)
		_ = WriteStatus(s, false)
		_ = s.Close()
		return
	}

	if err := WriteStatus(s, true); err != nil {
		_ = s.Reset()
		_ = upstream.Reset()
		return
	}

	// Clear deadlines so the long-lived data flow is not bounded by
	// the handshake budget.
	_ = s.SetDeadline(time.Time{})
	_ = upstream.SetDeadline(time.Time{})

	pipe(s, upstream)
}

// pipe shuffles bytes between the dialer (down) and the listener (up).
func pipe(down, up network.Stream) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(up, down)
		_ = up.CloseWrite()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(down, up)
		_ = down.CloseWrite()
	}()
	wg.Wait()
	_ = down.Close()
	_ = up.Close()
}

// ---------- wire helpers (exported for the transport client) ----------

// ErrInvalidWarpIDHex is returned when a WarpID string is not 64 hex chars.
var ErrInvalidWarpIDHex = errors.New("aliasresolver: warp id must be 64 hex chars")

// WriteRegisterFrame serializes [warpID, sig] onto w. warpIDHex must
// decode to exactly 32 bytes.
func WriteRegisterFrame(w io.Writer, warpIDHex string, sig []byte) error {
	idBytes, err := decodeWarpID(warpIDHex)
	if err != nil {
		return err
	}
	if len(sig) > MaxSigLen {
		return fmt.Errorf("aliasresolver: signature too long (%d > %d)", len(sig), MaxSigLen)
	}
	buf := make([]byte, 0, WarpIDByteLen+2+len(sig))
	buf = append(buf, idBytes...)
	var sigLen [2]byte
	binary.BigEndian.PutUint16(sigLen[:], uint16(len(sig)))
	buf = append(buf, sigLen[:]...)
	buf = append(buf, sig...)
	_, err = w.Write(buf)
	return err
}

// ReadRegisterFrame parses one register frame and returns the WarpID
// (hex) and signature.
func ReadRegisterFrame(r io.Reader) (string, []byte, error) {
	var idBytes [WarpIDByteLen]byte
	if _, err := io.ReadFull(r, idBytes[:]); err != nil {
		return "", nil, err
	}
	var sigLenBytes [2]byte
	if _, err := io.ReadFull(r, sigLenBytes[:]); err != nil {
		return "", nil, err
	}
	sigLen := int(binary.BigEndian.Uint16(sigLenBytes[:]))
	if sigLen == 0 || sigLen > MaxSigLen {
		return "", nil, fmt.Errorf("aliasresolver: sig length out of range: %d", sigLen)
	}
	sig := make([]byte, sigLen)
	if _, err := io.ReadFull(r, sig); err != nil {
		return "", nil, err
	}
	return hex.EncodeToString(idBytes[:]), sig, nil
}

// WriteResolveFrame serializes [warpID] onto w.
func WriteResolveFrame(w io.Writer, warpIDHex string) error {
	idBytes, err := decodeWarpID(warpIDHex)
	if err != nil {
		return err
	}
	_, err = w.Write(idBytes)
	return err
}

// ReadResolveFrame parses one resolve frame and returns the WarpID (hex).
func ReadResolveFrame(r io.Reader) (string, error) {
	var idBytes [WarpIDByteLen]byte
	if _, err := io.ReadFull(r, idBytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(idBytes[:]), nil
}

// WriteStatus writes a single status byte: ok=true → 0x01, else 0x00.
func WriteStatus(w io.Writer, ok bool) error {
	b := byte(statusDenied)
	if ok {
		b = statusOK
	}
	_, err := w.Write([]byte{b})
	return err
}

// ReadStatus reads one status byte; returns true iff it is 0x01.
func ReadStatus(r io.Reader) (bool, error) {
	var b [1]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return false, err
	}
	return b[0] == statusOK, nil
}

// ===========================================================================
// network.Notifiee — evict registrations when the owning peer fully
// disconnects. This bounds memory on a public relay even when peers
// churn without explicit unregister.
// ===========================================================================

var _ network.Notifiee = (*Resolver)(nil)

func (r *Resolver) Listen(_ network.Network, _ ma.Multiaddr)      {}
func (r *Resolver) ListenClose(_ network.Network, _ ma.Multiaddr) {}
func (r *Resolver) Connected(_ network.Network, _ network.Conn)   {}

func (r *Resolver) Disconnected(n network.Network, c network.Conn) {
	p := c.RemotePeer()
	// Only evict when the LAST connection to this peer goes away;
	// otherwise a transient connection close (with another conn still
	// up) would purge a still-reachable listener.
	if len(n.ConnsToPeer(p)) > 0 {
		return
	}
	r.mu.Lock()
	if id, ok := r.byPeer[p]; ok {
		delete(r.table, id)
		delete(r.byPeer, p)
	}
	r.mu.Unlock()
}

func decodeWarpID(s string) ([]byte, error) {
	if len(s) != WarpIDByteLen*2 {
		return nil, ErrInvalidWarpIDHex
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWarpIDHex, err)
	}
	if len(b) != WarpIDByteLen {
		return nil, ErrInvalidWarpIDHex
	}
	return b, nil
}

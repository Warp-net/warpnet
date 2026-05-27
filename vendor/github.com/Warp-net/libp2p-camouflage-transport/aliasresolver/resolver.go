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

// Package aliasresolver — relay-side WarpID table and resolve-stream
// pipe. Wire frames:
//
//	Register: [32-byte WarpID][2-byte BE sigLen][sig...]
//	Resolve:  [32-byte WarpID]
//	Status:   [1 byte] 0x01=ok
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

const (
	RegisterProtocol protocol.ID = "/warpnet/alias-register/0.0.0"
	ResolveProtocol  protocol.ID = "/warpnet/alias-resolve/0.0.0"
	StopProtocol     protocol.ID = "/warpnet/alias-stop/0.0.0"
)

const (
	WarpIDByteLen = 32
	MaxSigLen     = 1024

	statusOK     = 0x01
	statusDenied = 0x00
)

// HandshakeTimeout caps how long the relay waits for a single
// register or resolve handshake to complete. Overridable from outside
// for slow networks; not per-Resolver because all relays in a deployment
// will share network conditions.
var HandshakeTimeout = 10 * time.Second

const DefaultMaxEntries = 10000
const DefaultMaxBytesPerDirection int64 = 256 << 20

type Entry struct {
	Peer  peer.ID
	Owner crypto.PubKey
}

type Resolver struct {
	host                 host.Host
	maxEntries           int
	maxBytesPerDirection int64

	mu     sync.RWMutex
	table  map[string]Entry
	byPeer map[peer.ID]string
}

func New(h host.Host) *Resolver {
	return &Resolver{
		host:                 h,
		maxEntries:           DefaultMaxEntries,
		maxBytesPerDirection: DefaultMaxBytesPerDirection,
		table:                make(map[string]Entry),
		byPeer:               make(map[peer.ID]string),
	}
}

func (r *Resolver) Start() {
	r.host.SetStreamHandler(RegisterProtocol, r.HandleRegister)
	r.host.SetStreamHandler(ResolveProtocol, r.HandleResolve)
	r.host.Network().Notify(r)
}

func (r *Resolver) Stop() {
	r.host.RemoveStreamHandler(RegisterProtocol)
	r.host.RemoveStreamHandler(ResolveProtocol)
	r.host.Network().StopNotify(r)
	r.mu.Lock()
	r.table = make(map[string]Entry)
	r.byPeer = make(map[peer.ID]string)
	r.mu.Unlock()
}

func (r *Resolver) Lookup(id string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.table[id]
	return e, ok
}

func (r *Resolver) HandleRegister(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(HandshakeTimeout))

	pub := s.Conn().RemotePublicKey()
	if pub == nil {
		_ = s.Reset()
		return
	}

	warpHex, sig, err := ReadRegisterFrame(s)
	if err != nil {
		log.Printf("aliasresolver: register decode from %s: %v", s.Conn().RemotePeer(), err)
		_ = s.Reset()
		return
	}

	// Verify over raw bytes so encoding-case differences don't matter.
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
	if _, exists := r.table[warpHex]; !exists && len(r.table) >= r.maxEntries {
		r.mu.Unlock()
		log.Printf("aliasresolver: table full (%d), rejecting %s", r.maxEntries, remote)
		_ = WriteStatus(s, false)
		return
	}
	if prev, ok := r.byPeer[remote]; ok && prev != warpHex {
		delete(r.table, prev)
	}
	r.table[warpHex] = Entry{Peer: remote, Owner: pub}
	r.byPeer[remote] = warpHex
	r.mu.Unlock()

	_ = WriteStatus(s, true)
}

func (r *Resolver) HandleResolve(s network.Stream) {
	_ = s.SetDeadline(time.Now().Add(HandshakeTimeout))

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

	ctx, cancel := context.WithTimeout(context.Background(), HandshakeTimeout)
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

	_ = s.SetDeadline(time.Time{})
	_ = upstream.SetDeadline(time.Time{})

	pipe(s, upstream, r.maxBytesPerDirection)
}

func pipe(down, up network.Stream, maxBytes int64) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.CopyN(up, down, maxBytes)
		_ = up.CloseWrite()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.CopyN(down, up, maxBytes)
		_ = down.CloseWrite()
	}()
	wg.Wait()
	_ = down.Close()
	_ = up.Close()
}

var ErrInvalidWarpIDHex = errors.New("aliasresolver: warp id must be 64 hex chars")

func WriteRegisterFrame(w io.Writer, warpIDHex string, sig []byte) error {
	idBytes, err := decodeWarpID(warpIDHex)
	if err != nil {
		return err
	}
	if len(sig) == 0 {
		return errors.New("aliasresolver: signature is empty")
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

func WriteResolveFrame(w io.Writer, warpIDHex string) error {
	idBytes, err := decodeWarpID(warpIDHex)
	if err != nil {
		return err
	}
	_, err = w.Write(idBytes)
	return err
}

func ReadResolveFrame(r io.Reader) (string, error) {
	var idBytes [WarpIDByteLen]byte
	if _, err := io.ReadFull(r, idBytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(idBytes[:]), nil
}

func WriteStatus(w io.Writer, ok bool) error {
	b := byte(statusDenied)
	if ok {
		b = statusOK
	}
	_, err := w.Write([]byte{b})
	return err
}

func ReadStatus(r io.Reader) (bool, error) {
	var b [1]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return false, err
	}
	return b[0] == statusOK, nil
}

var _ network.Notifiee = (*Resolver)(nil)

func (r *Resolver) Listen(_ network.Network, _ ma.Multiaddr)      {}
func (r *Resolver) ListenClose(_ network.Network, _ ma.Multiaddr) {}
func (r *Resolver) Connected(_ network.Network, _ network.Conn)   {}

func (r *Resolver) Disconnected(n network.Network, c network.Conn) {
	p := c.RemotePeer()
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

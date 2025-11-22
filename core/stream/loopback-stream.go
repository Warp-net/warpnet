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

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

package stream

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	log "github.com/sirupsen/logrus"
)

const loopbackStreamName = "loopback"

var _ network.Conn = (*LoopbackConn)(nil)

type LoopbackConn struct {
	stream   *LoopbackStream
	openedAt time.Time
}

func (c *LoopbackConn) Close() error {
	return c.stream.Close()
}

func (c *LoopbackConn) LocalPeer() peer.ID {
	return c.stream.localPeerID
}

func (c *LoopbackConn) RemotePeer() peer.ID {
	return c.stream.localPeerID
}

func (c *LoopbackConn) RemotePublicKey() p2pCrypto.PubKey {
	return nil
}

func (c *LoopbackConn) As(_ any) bool {
	return false
}

func (c *LoopbackConn) ConnState() network.ConnectionState {
	return network.ConnectionState{
		StreamMultiplexer:         loopbackStreamName,
		Security:                  loopbackStreamName,
		Transport:                 loopbackStreamName,
		UsedEarlyMuxerNegotiation: false,
	}
}

func (c *LoopbackConn) LocalMultiaddr() multiaddr.Multiaddr {
	return multiaddr.StringCast("/ip4/0.0.0.0/tcp/0")
}

func (c *LoopbackConn) RemoteMultiaddr() multiaddr.Multiaddr {
	return multiaddr.StringCast("/ip4/0.0.0.0/tcp/0")
}

func (c *LoopbackConn) Stat() network.ConnStats {
	return network.ConnStats{
		Stats: network.Stats{
			Direction: network.DirInbound,
			Opened:    c.openedAt,
			Limited:   false,
			Extra:     nil,
		},
		NumStreams: 0,
	}
}

func (c *LoopbackConn) Scope() network.ConnScope {
	return nil
}

func (c *LoopbackConn) CloseWithError(_ network.ConnErrorCode) error {
	return c.stream.Close()
}

func (c *LoopbackConn) ID() string {
	return c.LocalPeer().String()
}

func (c *LoopbackConn) NewStream(_ context.Context) (network.Stream, error) {
	return c.stream, nil
}

func (c *LoopbackConn) GetStreams() []network.Stream {
	return []network.Stream{c.stream}
}

func (c *LoopbackConn) IsClosed() bool {
	return c.stream.isFullyClosed()
}

// =======================================================================================================

type LoopbackStream struct {
	writeConn                   net.Conn
	readConn                    net.Conn
	proto                       warpnet.WarpProtocolID
	localPeerID                 warpnet.WarpPeerID
	isReadClosed, isWriteClosed *atomic.Bool
	writeMx, readMx             *sync.Mutex
}

func (s *LoopbackStream) CloseRead() error {
	s.readMx.Lock()
	defer s.readMx.Unlock()

	if s.isReadClosed.Swap(true) {
		return nil
	}
	return s.readConn.Close()
}

func (s *LoopbackStream) CloseWrite() error {
	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	if s.isWriteClosed.Swap(true) {
		return nil
	}
	return s.writeConn.Close()
}

func (s *LoopbackStream) Reset() error {
	return nil
}

func (s *LoopbackStream) ResetWithError(err network.StreamErrorCode) error {
	log.Errorf("loopback stream: reset with failure: %v", err)
	return nil
}

func (s *LoopbackStream) Read(p []byte) (int, error) {
	s.readMx.Lock()
	defer s.readMx.Unlock()

	if s.isReadClosed.Load() {
		return 0, nil
	}
	return s.readConn.Read(p)
}

func (s *LoopbackStream) Write(p []byte) (int, error) {
	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	if s.isWriteClosed.Load() {
		return 0, nil
	}
	return s.writeConn.Write(p)
}

func (s *LoopbackStream) Close() error {
	if s.isFullyClosed() {
		return nil
	}
	_ = s.CloseWrite()
	_ = s.CloseRead()
	return nil
}

func (s *LoopbackStream) isFullyClosed() bool {
	return s.isReadClosed.Load() && s.isWriteClosed.Load()
}

func (s *LoopbackStream) SetDeadline(t time.Time) error {
	errWrite := s.writeConn.SetDeadline(t)
	errRead := s.readConn.SetDeadline(t)
	return errors.Join(errWrite, errRead)
}
func (s *LoopbackStream) SetReadDeadline(t time.Time) error  { return s.readConn.SetReadDeadline(t) }
func (s *LoopbackStream) SetWriteDeadline(t time.Time) error { return s.writeConn.SetWriteDeadline(t) }
func (s *LoopbackStream) ID() string                         { return loopbackStreamName }
func (s *LoopbackStream) Scope() network.StreamScope         { return nil } // optionally implement your own
func (s *LoopbackStream) Protocol() protocol.ID              { return s.proto }
func (s *LoopbackStream) SetProtocol(p protocol.ID) error    { s.proto = p; return nil }
func (s *LoopbackStream) Stat() network.Stats                { return network.Stats{Direction: network.DirInbound} }
func (s *LoopbackStream) Conn() network.Conn {
	return &LoopbackConn{stream: s, openedAt: time.Now()}
}

func NewLoopbackStream(
	nodeId warpnet.WarpPeerID, proto warpnet.WarpProtocolID,
) (r warpnet.WarpStream, w warpnet.WarpStream) {
	reader1, writer2 := net.Pipe()
	reader2, writer1 := net.Pipe()

	reader := &LoopbackStream{
		readConn: reader1, writeConn: writer1, localPeerID: nodeId,
		proto: proto, isReadClosed: new(atomic.Bool), isWriteClosed: new(atomic.Bool),
		writeMx: new(sync.Mutex), readMx: new(sync.Mutex),
	}

	writer := &LoopbackStream{
		readConn: reader2, writeConn: writer2, localPeerID: nodeId,
		proto: proto, isReadClosed: new(atomic.Bool), isWriteClosed: new(atomic.Bool),
		writeMx: new(sync.Mutex), readMx: new(sync.Mutex),
	}

	return reader, writer
}

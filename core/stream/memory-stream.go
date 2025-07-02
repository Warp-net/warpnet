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
	"fmt"
	"github.com/Warp-net/warpnet/core/warpnet"
	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"io"
	"net"
	"sync/atomic"
	"time"
)

type LoopbackConn struct {
	Proto       protocol.ID
	LocalPeerID warpnet.WarpPeerID
	WriteConn   net.Conn
	ReadConn    net.Conn
	isClosed    *atomic.Bool
}

func (l *LoopbackConn) Close() error {
	fmt.Println("LoopbackConn.Close")

	_ = l.WriteConn.Close()
	_ = l.ReadConn.Close()
	l.isClosed.Store(true)
	return nil
}

func (l *LoopbackConn) LocalPeer() peer.ID {
	return l.LocalPeerID
}

func (l *LoopbackConn) RemotePeer() peer.ID {
	return l.LocalPeerID
}

func (l *LoopbackConn) RemotePublicKey() p2pCrypto.PubKey {
	return nil
}

func (l *LoopbackConn) ConnState() network.ConnectionState {
	return network.ConnectionState{
		StreamMultiplexer:         "loopback",
		Security:                  "loopback",
		Transport:                 "loopback",
		UsedEarlyMuxerNegotiation: false,
	}
}

func (l *LoopbackConn) LocalMultiaddr() multiaddr.Multiaddr {
	return multiaddr.StringCast("/ip4/0.0.0.0/tcp/0")
}

func (l *LoopbackConn) RemoteMultiaddr() multiaddr.Multiaddr {
	return multiaddr.StringCast("/ip4/0.0.0.0/tcp/0")

}

func (l *LoopbackConn) Stat() network.ConnStats {
	return network.ConnStats{
		Stats: network.Stats{
			Direction: network.DirInbound,
			Opened:    time.Now(),
			Limited:   false,
			Extra:     nil,
		},
		NumStreams: 0,
	}
}

func (l *LoopbackConn) Scope() network.ConnScope {
	return nil
}

func (l *LoopbackConn) CloseWithError(errCode network.ConnErrorCode) error {
	if l.isClosed.Load() {
		return nil
	}
	fmt.Println("LoopbackConn.CloseWithError", errCode)
	_ = l.ReadConn.Close()
	_ = l.WriteConn.Close()
	return fmt.Errorf("connection closed with %v", errCode)
}

func (l *LoopbackConn) ID() string {
	return l.LocalPeerID.String()
}

func (l *LoopbackConn) NewStream(_ context.Context) (network.Stream, error) {
	if l.isClosed.Load() {
		return nil, io.ErrClosedPipe
	}
	return &LoopbackStream{
		WriteConn:   l.WriteConn,
		ReadConn:    l.ReadConn,
		Proto:       l.Proto,
		LocalPeerID: l.LocalPeerID,
	}, nil
}

func (l *LoopbackConn) GetStreams() []network.Stream {
	return []network.Stream{&LoopbackStream{
		WriteConn:   l.WriteConn,
		ReadConn:    l.ReadConn,
		Proto:       l.Proto,
		LocalPeerID: l.LocalPeerID,
	}}
}

func (l *LoopbackConn) IsClosed() bool {
	return l.isClosed.Load()
}

type LoopbackStream struct {
	WriteConn                   net.Conn
	ReadConn                    net.Conn
	Proto                       warpnet.WarpProtocolID
	LocalPeerID                 warpnet.WarpPeerID
	conn                        network.Conn
	isReadClosed, isWriteClosed *atomic.Bool
}

func (s *LoopbackStream) Protocol() protocol.ID           { return s.Proto }
func (s *LoopbackStream) SetProtocol(p protocol.ID) error { s.Proto = p; return nil }
func (s *LoopbackStream) Stat() network.Stats             { return network.Stats{Direction: network.DirInbound} }
func (s *LoopbackStream) Conn() network.Conn {
	return s.conn
}
func (s *LoopbackStream) CloseRead() error {
	if s.isReadClosed.Load() {
		return nil
	}
	fmt.Println("LoopbackStream.CloseRead")
	s.isReadClosed.Store(true)
	return s.ReadConn.Close()
}
func (s *LoopbackStream) CloseWrite() error {
	if s.isWriteClosed.Load() {
		return nil
	}
	fmt.Println("LoopbackStream.CloseWrite")
	s.isWriteClosed.Store(true)
	return s.WriteConn.Close()
}
func (s *LoopbackStream) Reset() error {
	return nil
}
func (s *LoopbackStream) ResetWithError(_ network.StreamErrorCode) error {
	return nil
}
func (s *LoopbackStream) Read(p []byte) (int, error) {
	if s.isReadClosed.Load() {
		return 0, io.ErrClosedPipe
	}
	return s.ReadConn.Read(p)
}

func (s *LoopbackStream) Write(p []byte) (int, error) {
	if s.isWriteClosed.Load() {
		return 0, io.ErrClosedPipe
	}
	return s.WriteConn.Write(p)
}
func (s *LoopbackStream) Close() error {
	if s.isWriteClosed.Load() && s.isReadClosed.Load() {
		return nil
	}
	fmt.Println("LoopbackStream.Close")

	_ = s.CloseWrite()
	_ = s.CloseRead()
	return nil
}

func (s *LoopbackStream) SetDeadline(t time.Time) error {
	_ = s.WriteConn.SetDeadline(t)
	return s.ReadConn.SetDeadline(t)
}
func (s *LoopbackStream) SetReadDeadline(t time.Time) error  { return s.ReadConn.SetReadDeadline(t) }
func (s *LoopbackStream) SetWriteDeadline(t time.Time) error { return s.WriteConn.SetWriteDeadline(t) }
func (s *LoopbackStream) ID() string                         { return "loopback" }
func (s *LoopbackStream) Scope() network.StreamScope         { return nil } // optionally implement your own

func NewLoopbackStream(nodeId warpnet.WarpPeerID, proto warpnet.WarpProtocolID) (r *LoopbackStream, w *LoopbackStream) {
	reader1, writer2 := net.Pipe()
	reader2, writer1 := net.Pipe()

	reader := &LoopbackStream{
		ReadConn: reader1, WriteConn: writer1, LocalPeerID: nodeId,
		Proto: proto, isReadClosed: new(atomic.Bool), isWriteClosed: new(atomic.Bool),
		conn: &LoopbackConn{
			WriteConn: writer1, ReadConn: reader1, Proto: proto,
			LocalPeerID: nodeId, isClosed: new(atomic.Bool),
		},
	}

	writer := &LoopbackStream{
		ReadConn: reader2, WriteConn: writer2, LocalPeerID: nodeId,
		Proto: proto, isReadClosed: new(atomic.Bool), isWriteClosed: new(atomic.Bool),
		conn: &LoopbackConn{
			WriteConn: writer2, ReadConn: reader2, Proto: proto,
			LocalPeerID: nodeId, isClosed: new(atomic.Bool),
		},
	}

	return reader, writer
}

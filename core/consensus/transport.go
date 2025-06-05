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

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package consensus

import (
	"context"
	"errors"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/retrier"
	"github.com/hashicorp/raft"
	gostream "github.com/libp2p/go-libp2p-gostream"
	"github.com/libp2p/go-libp2p/core/network"
	"net"
	"sync"
	"time"
)

const raftProtocol warpnet.WarpProtocolID = "/raft/1.0.0/rpc"

type addrProvider struct{}

func (ap *addrProvider) ServerAddr(id raft.ServerID) (raft.ServerAddress, error) {
	return raft.ServerAddress(id), nil
}

type streamLayer struct {
	host    NodeTransporter
	l       net.Listener
	lg      *consensusLogger
	retrier retrier.Retrier
	cache   *sync.Map
}

func newStreamLayer(h NodeTransporter, lg *consensusLogger) (*streamLayer, error) {
	peerIdCache := new(sync.Map)

	listener, err := gostream.Listen(h.Node(), raftProtocol)
	if err != nil {
		return nil, err
	}

	return &streamLayer{
		host:    h,
		l:       listener,
		lg:      lg,
		retrier: retrier.New(time.Second*1, 5, retrier.ArithmeticalBackoff),
		cache:   peerIdCache,
	}, nil
}

func (sl *streamLayer) Dial(address raft.ServerAddress, timeout time.Duration) (net.Conn, error) {
	if sl.host == nil {
		return nil, warpnet.WarpError("stream layer not initialized")
	}
	var pid warpnet.WarpPeerID
	value, ok := sl.cache.Load(string(address))
	if ok {
		pid = value.(warpnet.WarpPeerID)
	} else {
		pid = warpnet.FromStringToPeerID(string(address))
		sl.cache.Store(string(address), pid)
	}
	if pid.String() == "" {
		return nil, warpnet.WarpError("raft-transport: invalid server id")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	connectedness := sl.host.Network().Connectedness(pid)
	switch connectedness {
	case network.Limited:
		ctx = network.WithAllowLimitedConn(ctx, warpnet.WarpnetName)
	default:
	}
	conn, err := gostream.Dial(ctx, sl.host.Node(), pid, raftProtocol)

	if err != nil {
		sl.lg.Debug("raft-transport: dial failed to " + string(address) + ": " + err.Error())
		if errors.Is(err, warpnet.ErrAllDialsFailed) {
			err = warpnet.ErrAllDialsFailed
		}
	}
	return conn, err
}

func (sl *streamLayer) Accept() (conn net.Conn, err error) {
	err = sl.retrier.Try(context.Background(), func() error {
		conn, err = sl.l.Accept()
		if err != nil {
			sl.lg.Debug("raft-transport: accept: " + err.Error())
		}
		return err
	})

	return conn, err
}

func (sl *streamLayer) Addr() net.Addr {
	return sl.l.Addr()
}

func (sl *streamLayer) Close() error {
	sl.lg.Debug("raft-transport: stream closed")
	return sl.l.Close()
}

// Libp2p multiplexes streams over persistent TCP connections,
// so we disable connection pooling (MaxPool=0) to avoid unnecessary complexity.
// Check https://github.com/hashicorp/raft/net_transport.go#L606
const maxPoolConnections int = 0 //

func NewWarpnetConsensusTransport(node NodeTransporter, l *consensusLogger) (*raft.NetworkTransport, error) {
	p2pStream, err := newStreamLayer(node, l)
	if err != nil {
		return nil, err
	}

	transportConfig := &raft.NetworkTransportConfig{
		ServerAddressProvider: &addrProvider{},
		Logger:                l,
		Stream:                p2pStream,
		MaxPool:               maxPoolConnections,
		Timeout:               time.Minute,
	}

	return raft.NewNetworkTransportWithConfig(transportConfig), nil
}

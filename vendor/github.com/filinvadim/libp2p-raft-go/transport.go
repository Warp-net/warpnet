package raft

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/raft"
	gostream "github.com/libp2p/go-libp2p-gostream"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"net"
	"sync"
	"time"
)

const (
	raftProtocol protocol.ID = "/raft/1.0.0/rpc"

	// Libp2p multiplexes streams over persistent TCP connections,
	// so we disable connection pooling (MaxPool=0) to avoid unnecessary complexity.
	// Check https://github.com/hashicorp/raft/net_transport.go#L606
	maxPoolConnections int = 0
)

type NewTransportFunc func(node NodeTransporter, l Logger) (*raft.NetworkTransport, error)

func NewConsensusTransport(node NodeTransporter, l Logger) (*raft.NetworkTransport, error) {
	p2pStream, err := newStreamLayer(node)
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

type addrProvider struct{}

func (ap *addrProvider) ServerAddr(id raft.ServerID) (raft.ServerAddress, error) {
	return raft.ServerAddress(id), nil
}

type streamLayer struct {
	host  NodeTransporter
	l     net.Listener
	lg    Logger
	cache *sync.Map // for heavy base58 encoding
}

func newStreamLayer(h NodeTransporter) (*streamLayer, error) {
	peerIdCache := new(sync.Map)

	listener, err := gostream.Listen(h, raftProtocol)
	if err != nil {
		return nil, err
	}

	return &streamLayer{
		host:  h,
		l:     listener,
		cache: peerIdCache,
	}, nil
}

func (sl *streamLayer) Dial(address raft.ServerAddress, timeout time.Duration) (net.Conn, error) {
	if sl.host == nil {
		return nil, errors.New("raft-transport: not initialized")
	}
	var (
		pid peer.ID
		err error
	)
	value, ok := sl.cache.Load(string(address))
	if ok {
		pid = value.(peer.ID)
	} else {
		pid, err = peer.Decode(string(address))
		if err != nil {
			return nil, err
		}
		sl.cache.Store(string(address), pid)
	}
	if pid.String() == "" {
		return nil, fmt.Errorf("raft-transport: invalid server id: %s", string(address))
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	connectedness := sl.host.Network().Connectedness(pid)
	switch connectedness {
	case network.Limited:
		ctx = network.WithAllowLimitedConn(ctx, "raft-transport")
	default:
	}
	return gostream.Dial(ctx, sl.host, pid, raftProtocol)
}

func (sl *streamLayer) Accept() (conn net.Conn, err error) {
	return sl.l.Accept()
}

func (sl *streamLayer) Addr() net.Addr {
	return sl.l.Addr()
}

func (sl *streamLayer) Close() error {
	return sl.l.Close()
}

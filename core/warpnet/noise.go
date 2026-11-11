package warpnet

import (
	"context"
	coreconnmgr "github.com/libp2p/go-libp2p/core/connmgr"
	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/sec"
	tptu "github.com/libp2p/go-libp2p/p2p/net/upgrader"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	manet "github.com/multiformats/go-multiaddr/net"
	gonet "net"
)

func NewNoise(
	id protocol.ID, pk p2pCrypto.PrivKey, mxs []tptu.StreamMuxer, gater coreconnmgr.ConnectionGater,
) (*gatedNoise, error) {
	tpt, err := noise.New(id, pk, mxs)
	if err != nil {
		return nil, err
	}
	return &gatedNoise{Transport: tpt, gater: gater}, nil
}

type gatedNoise struct {
	*noise.Transport

	gater coreconnmgr.ConnectionGater
}

func (n *gatedNoise) SecureOutbound(ctx context.Context, insecure gonet.Conn, p WarpPeerID) (sec.SecureConn, error) {
	if !n.isAllowed(network.DirOutbound, p, insecure) {
		_ = insecure.Close()
		return nil, ErrPeerBlocked
	}
	return n.Transport.SecureOutbound(ctx, insecure, p)
}

func (n *gatedNoise) SecureInbound(ctx context.Context, insecure gonet.Conn, p WarpPeerID) (sec.SecureConn, error) {
	conn, err := n.Transport.SecureInbound(ctx, insecure, p)
	if err != nil {
		return conn, err
	}
	if !n.isAllowed(network.DirInbound, conn.RemotePeer(), insecure) {
		_ = conn.Close()
		return nil, ErrPeerBlocked
	}
	return conn, nil
}

func (n *gatedNoise) isAllowed(dir network.Direction, id WarpPeerID, conn gonet.Conn) bool {
	if n.gater == nil {
		return true
	}
	addrs, err := manet.WrapNetConn(conn)
	if err != nil {
		return true
	}
	return n.gater.InterceptSecured(dir, id, addrs)
}

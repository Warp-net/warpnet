// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package warpnet

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	gonet "net"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

type blocklist map[string]bool

func (b blocklist) IsBlocklisted(peerId string) bool { return b[peerId] }

func noiseIdentity(t *testing.T) (p2pCrypto.PrivKey, WarpPeerID) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	key, err := p2pCrypto.UnmarshalEd25519PrivateKey(priv)
	require.NoError(t, err)
	id, err := peer.IDFromPrivateKey(key)
	require.NoError(t, err)
	return key, id
}

func tcpPair(t *testing.T) (client, server gonet.Conn) {
	t.Helper()
	l, err := gonet.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	accepted := make(chan gonet.Conn, 1)
	go func() {
		c, _ := l.Accept()
		accepted <- c
	}()
	client, err = gonet.Dial("tcp", l.Addr().String())
	require.NoError(t, err)
	server = <-accepted
	require.NotNil(t, server)
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	return client, server
}

func TestABlockedPeerIsRefusedInTheHandshake(t *testing.T) {
	serverKey, serverID := noiseIdentity(t)
	clientKey, clientID := noiseIdentity(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("inbound", func(t *testing.T) {
		server, err := NewNoise(NoiseID, serverKey, nil, blocklistGater{blocklist: blocklist{clientID.String(): true}})
		require.NoError(t, err)
		client, err := NewNoise(NoiseID, clientKey, nil, nil)
		require.NoError(t, err)
		c, s := tcpPair(t)

		go func() { _, _ = client.SecureOutbound(ctx, c, serverID) }()
		_, err = server.SecureInbound(ctx, s, "")
		require.ErrorIs(t, err, ErrPeerBlocked)
	})

	t.Run("outbound", func(t *testing.T) {
		client, err := NewNoise(NoiseID, clientKey, nil, blocklistGater{blocklist: blocklist{serverID.String(): true}})
		require.NoError(t, err)
		c, s := tcpPair(t)

		_, err = client.SecureOutbound(ctx, c, serverID)
		require.ErrorIs(t, err, ErrPeerBlocked)
		require.NoError(t, s.SetReadDeadline(time.Now().Add(time.Second)))
		n, err := s.Read(make([]byte, 1))
		require.Zero(t, n, "not a single handshake byte leaves for a blocked peer")
		require.ErrorIs(t, err, io.EOF)
	})

	t.Run("allowed", func(t *testing.T) {
		server, err := NewNoise(NoiseID, serverKey, nil, blocklistGater{blocklist: blocklist{}})
		require.NoError(t, err)
		client, err := NewNoise(NoiseID, clientKey, nil, nil)
		require.NoError(t, err)
		c, s := tcpPair(t)

		outbound := make(chan error, 1)
		go func() {
			_, err := client.SecureOutbound(ctx, c, serverID)
			outbound <- err
		}()
		conn, err := server.SecureInbound(ctx, s, "")
		require.NoError(t, err)
		require.Equal(t, clientID, conn.RemotePeer())
		require.NoError(t, <-outbound)
	})
}

type securedCounter struct {
	blocklistGater
	mx      sync.Mutex
	secured map[WarpPeerID]int
}

func (c *securedCounter) InterceptSecured(dir network.Direction, id WarpPeerID, addrs network.ConnMultiaddrs) bool {
	c.mx.Lock()
	c.secured[id]++
	c.mx.Unlock()
	return c.blocklistGater.InterceptSecured(dir, id, addrs)
}

func (c *securedCounter) count(id WarpPeerID) int {
	c.mx.Lock()
	defer c.mx.Unlock()
	return c.secured[id]
}

func TestTheHostHandsItsGaterToTheHandshake(t *testing.T) {
	gater := &securedCounter{blocklistGater: blocklistGater{blocklist: blocklist{}}, secured: map[WarpPeerID]int{}}
	opts := append([]libp2p.Option{}, CommonOptions...)
	member, err := libp2p.New(append(opts, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"), libp2p.ConnectionGater(gater))...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = member.Close() })
	remote, err := libp2p.New(append(opts, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = remote.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, remote.Connect(ctx, peer.AddrInfo{ID: member.ID(), Addrs: member.Addrs()}))
	require.Equal(t, 2, gater.count(remote.ID()), "the handshake asks the gater before the upgrader does")
}

type lateBlock struct {
	mx      sync.Mutex
	id      string
	allowed int
}

func (b *lateBlock) IsBlocklisted(peerId string) bool {
	b.mx.Lock()
	defer b.mx.Unlock()
	if peerId != b.id {
		return false
	}
	if b.allowed > 0 {
		b.allowed--
		return false
	}
	return true
}

func TestABlockThatLandsMidConnectionStillCutsThePeer(t *testing.T) {
	for name, checksPassed := range map[string]int{
		"after the handshake, before the upgrade": 2,
		"after the upgrade, before registration":  3,
	} {
		t.Run(name, func(t *testing.T) {
			opts := append([]libp2p.Option{}, CommonOptions...)
			remote, err := libp2p.New(append(opts, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))...)
			require.NoError(t, err)
			t.Cleanup(func() { _ = remote.Close() })
			block := &lateBlock{id: remote.ID().String(), allowed: checksPassed}
			member, err := libp2p.New(append(opts, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"), NewConnectionGaterOption(block))...)
			require.NoError(t, err)
			t.Cleanup(func() { _ = member.Close() })

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = remote.Connect(ctx, peer.AddrInfo{ID: member.ID(), Addrs: member.Addrs()})
			require.Eventually(t, func() bool {
				return len(member.Network().ConnsToPeer(remote.ID())) == 0 &&
					len(remote.Network().ConnsToPeer(member.ID())) == 0
			}, 5*time.Second, 20*time.Millisecond, "the peer blocked mid-connection is left with no connection")
		})
	}
}

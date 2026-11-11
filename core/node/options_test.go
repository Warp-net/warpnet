// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

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
			opts := append([]libp2p.Option{}, warpnet.CommonOptions...)
			remote, err := libp2p.New(append(opts, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))...)
			require.NoError(t, err)
			t.Cleanup(func() { _ = remote.Close() })
			block := &lateBlock{id: remote.ID().String(), allowed: checksPassed}
			member, err := libp2p.New(append(opts, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"), NewBlocklistOption(block))...)
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

type mutableBlocklist struct {
	mx  sync.Mutex
	ids map[string]bool
}

func (b *mutableBlocklist) IsBlocklisted(peerId string) bool {
	b.mx.Lock()
	defer b.mx.Unlock()
	return b.ids[peerId]
}

func (b *mutableBlocklist) add(peerId string) {
	b.mx.Lock()
	defer b.mx.Unlock()
	b.ids[peerId] = true
}

func TestTheGaterCutsAPeerBlockedWhileConnected(t *testing.T) {
	opts := append([]libp2p.Option{}, warpnet.CommonOptions...)
	block := &mutableBlocklist{ids: map[string]bool{}}
	member, err := libp2p.New(append(opts, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"), newBlocklistOption(block, 20*time.Millisecond))...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = member.Close() })
	blocked, err := libp2p.New(append(opts, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = blocked.Close() })
	other, err := libp2p.New(append(opts, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = other.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	memberInfo := peer.AddrInfo{ID: member.ID(), Addrs: member.Addrs()}
	require.NoError(t, blocked.Connect(ctx, memberInfo))
	require.NoError(t, other.Connect(ctx, memberInfo))

	block.add(blocked.ID().String())
	require.Eventually(t, func() bool {
		return len(member.Network().ConnsToPeer(blocked.ID())) == 0
	}, 5*time.Second, 20*time.Millisecond, "the gater closes what the blocked peer already had open")
	require.Eventually(t, func() bool {
		return len(member.Peerstore().Addrs(blocked.ID())) == 0
	}, 5*time.Second, 20*time.Millisecond, "and the peerstore forgets it")
	require.NotEmpty(t, member.Network().ConnsToPeer(other.ID()), "and leaves every other peer connected")
}

func TestFindRelayNodeIdReadsTheRelayFromACircuitAddress(t *testing.T) {
	relayHost, err := libp2p.New(libp2p.NoListenAddrs)
	require.NoError(t, err)
	t.Cleanup(func() { _ = relayHost.Close() })

	circuit, err := warpnet.NewMultiaddr("/ip4/1.2.3.4/tcp/4001/p2p/" + relayHost.ID().String() + "/p2p-circuit")
	require.NoError(t, err)
	relayNodeId, ok := findRelayNodeId(circuit)
	require.True(t, ok)
	require.Equal(t, relayHost.ID(), relayNodeId)

	direct, err := warpnet.NewMultiaddr("/ip4/1.2.3.4/tcp/4001/p2p/" + relayHost.ID().String())
	require.NoError(t, err)
	_, ok = findRelayNodeId(direct)
	require.False(t, ok)
}

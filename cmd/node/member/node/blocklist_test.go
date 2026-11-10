// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package node

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	relayclient "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	"github.com/stretchr/testify/require"
)

const blockeeId = "blockee-1"

func TestABlockedUserLosesEveryWayIntoTheMemberNode(t *testing.T) {
	if testing.Short() {
		t.Skip("runs live libp2p hosts")
	}

	m, db, _ := newTestMemberNode(t)
	userRepo := database.NewUserRepo(db)
	m.opts = append(m.opts, libp2p.NoListenAddrs, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, m.Start())

	psk, err := security.GeneratePSK("testnet", semver.MustParse("0.0.0"))
	require.NoError(t, err)
	remoteOpts := append([]libp2p.Option{}, warpnet.CommonOptions...)
	remoteOpts = append(remoteOpts,
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.PrivateNetwork(warpnet.PSK(psk)),
	)
	remote, err := libp2p.New(remoteOpts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = remote.Close() })

	_, err = userRepo.Create(domain.User{Id: blockeeId, NodeId: remote.ID().String(), Network: "testnet"})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	remoteInfo := peer.AddrInfo{ID: remote.ID(), Addrs: remote.Addrs()}
	memberInfo := peer.AddrInfo{ID: m.Node().ID(), Addrs: m.Node().Addrs()}
	require.NoError(t, m.SimpleConnect(remoteInfo))
	_, err = relayclient.Reserve(ctx, remote, memberInfo)
	require.NoError(t, err, "the member relays for the peer before the block")

	const path protocol.ID = "/test/blocklist/1.0.0"
	incoming := make(chan network.Stream, 1)
	m.Node().SetStreamHandler(path, func(s network.Stream) { incoming <- s })
	s, err := remote.NewStream(ctx, m.Node().ID(), path)
	require.NoError(t, err)
	_, err = s.Write([]byte("open"))
	require.NoError(t, err)
	select {
	case accepted := <-incoming:
		defer accepted.Close()
	case <-ctx.Done():
		t.Fatal("stream was not accepted before the block")
	}

	resp := ownerStream(t, m, event.PRIVATE_POST_SETTINGS_BLOCK, event.BlockEvent{BlockerId: m.ownerId, BlockeeId: blockeeId})
	require.Contains(t, string(resp), "Accepted")

	require.Eventually(t, func() bool {
		return len(m.Network().ConnsToPeer(remote.ID())) == 0 && len(remote.Network().ConnsToPeer(m.Node().ID())) == 0
	}, 5*time.Second, 10*time.Millisecond, "every connection to the blocked peer is closed")
	_, err = s.Read(make([]byte, 1))
	require.Error(t, err, "open streams die with the connection")
	require.Empty(t, m.Peerstore().Addrs(remote.ID()), "the peerstore forgets the blocked peer")

	require.Error(t, m.SimpleConnect(remoteInfo), "the member does not dial the blocked peer")
	_, err = remote.NewStream(ctx, m.Node().ID(), path)
	require.Error(t, err, "the blocked peer cannot open a stream")
	_, err = relayclient.Reserve(ctx, remote, memberInfo)
	require.Error(t, err, "the blocked peer cannot renew its relay reservation")

	resp = ownerStream(t, m, event.PRIVATE_POST_SETTINGS_UNBLOCK, event.UnblockEvent{BlockerId: m.ownerId, BlockeeId: blockeeId})
	require.Contains(t, string(resp), "Accepted")
	m.Peerstore().AddAddrs(remote.ID(), remote.Addrs(), time.Hour)
	require.NoError(t, m.SimpleConnect(remoteInfo), "unblocking lets the peer back in")
}

func newGossipHost(t *testing.T, psk []byte, topicName string) (host.Host, *pubsub.Topic) {
	t.Helper()
	opts := append([]libp2p.Option{}, warpnet.CommonOptions...)
	h, err := libp2p.New(append(opts, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"), libp2p.PrivateNetwork(psk))...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ps, err := pubsub.NewGossipSub(ctx, h)
	require.NoError(t, err)
	topic, err := ps.Join(topicName)
	require.NoError(t, err)
	_, err = topic.Relay()
	require.NoError(t, err)
	return h, topic
}

func TestABlockedAuthorIsNotReadThroughAnotherPeer(t *testing.T) {
	if testing.Short() {
		t.Skip("runs live libp2p hosts")
	}
	const topicName = "blocklist-relayed-author"

	m, _, _ := newTestMemberNode(t)
	m.opts = append(m.opts, libp2p.NoListenAddrs, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, m.Start())

	var mx sync.Mutex
	read := map[string]bool{}
	require.NoError(t, m.pubsubService.Gossip().SubscribeRaw(topicName, func(data []byte) error {
		mx.Lock()
		defer mx.Unlock()
		read[strings.SplitN(string(data), "#", 2)[0]] = true
		return nil
	}))
	isRead := func(label string) bool {
		mx.Lock()
		defer mx.Unlock()
		return read[label]
	}

	psk, err := security.GeneratePSK("testnet", semver.MustParse("0.0.0"))
	require.NoError(t, err)
	relay, relayTopic := newGossipHost(t, psk, topicName)
	author, authorTopic := newGossipHost(t, psk, topicName)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, m.SimpleConnect(peer.AddrInfo{ID: relay.ID(), Addrs: relay.Addrs()}))
	require.NoError(t, author.Connect(ctx, peer.AddrInfo{ID: relay.ID(), Addrs: relay.Addrs()}))
	require.Empty(t, m.Network().ConnsToPeer(author.ID()), "the author reaches the member only through the relay")

	seq := 0
	publish := func(topic *pubsub.Topic, label string) {
		seq++
		_ = topic.Publish(ctx, []byte(fmt.Sprintf("%s#%d", label, seq)))
	}
	require.Eventually(t, func() bool { publish(authorTopic, "before"); return isRead("before") },
		30*time.Second, 200*time.Millisecond, "the relayed author is read before the block")

	require.NoError(t, m.BlocklistPermanent(author.ID().String()))
	require.Never(t, func() bool { publish(authorTopic, "after"); return isRead("after") },
		3*time.Second, 200*time.Millisecond, "a blocked author is dropped whoever relays it")
	require.Eventually(t, func() bool { publish(relayTopic, "relay"); return isRead("relay") },
		30*time.Second, 200*time.Millisecond, "the relay itself is still read")
}

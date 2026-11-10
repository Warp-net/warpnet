// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package pubsub

import (
	"context"
	"sync"
	"testing"

	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/stretchr/testify/require"
)

type peerBlacklist struct {
	mx  sync.Mutex
	ids map[warpnet.WarpPeerID]bool
}

func (b *peerBlacklist) Add(id warpnet.WarpPeerID) bool {
	b.mx.Lock()
	defer b.mx.Unlock()
	b.ids[id] = true
	return true
}

func (b *peerBlacklist) Contains(id warpnet.WarpPeerID) bool {
	b.mx.Lock()
	defer b.mx.Unlock()
	return b.ids[id]
}

func TestABlockedAuthorIsNotReadEvenThroughAnotherPeer(t *testing.T) {
	skipWithoutHosts(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	blacklist := &peerBlacklist{ids: map[warpnet.WarpPeerID]bool{}}
	readerNode := newLiveNode(t)
	reader := NewGossip(ctx, rating.NewPeersRatings(), WithBlacklist(blacklist))
	require.NoError(t, reader.Run(readerNode))
	t.Cleanup(func() { _ = reader.Close() })
	timeline := &postInbox{}
	require.NoError(t, reader.SubscribeRaw(timelineTopic, timeline.handle))

	author, authorNode, _ := ratedGossip(t)
	relay, relayNode, _ := ratedGossip(t)
	require.NoError(t, relay.SubscribeRaw(timelineTopic, func([]byte) error { return nil }))
	connect(t, relayNode, readerNode)
	connect(t, authorNode, relayNode)
	requireRead(t, timeline, author, timelineTopic, "before", "the author is read through the relay before the block")

	blacklist.Add(authorNode.host.ID())
	requireNotRead(t, timeline, author, timelineTopic, "after", "a blocked author is dropped whoever relays it")
	requireRead(t, timeline, relay, timelineTopic, "relay", "the relay itself is still read")
}

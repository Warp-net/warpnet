// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package pubsub

import (
	"context"
	"sync"
	"testing"

	"github.com/Warp-net/warpnet/core/rating"
	"github.com/stretchr/testify/require"
)

type peerBlocklist struct {
	mx  sync.Mutex
	ids map[string]bool
}

func (b *peerBlocklist) IsBlocklisted(peerId string) bool {
	b.mx.Lock()
	defer b.mx.Unlock()
	return b.ids[peerId]
}

func (b *peerBlocklist) add(peerId string) {
	b.mx.Lock()
	defer b.mx.Unlock()
	b.ids[peerId] = true
}

func TestABlockedAuthorIsNotReadEvenThroughAnotherPeer(t *testing.T) {
	skipWithoutHosts(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	blocklist := &peerBlocklist{ids: map[string]bool{}}
	readerNode := newLiveNode(t)
	reader := NewGossip(ctx, rating.NewPeersRatings())
	reader.SetBlocklist(blocklist)
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

	blocklist.add(authorNode.host.ID().String())
	requireNotRead(t, timeline, author, timelineTopic, "after", "a blocked author is dropped whoever relays it")
	requireRead(t, timeline, relay, timelineTopic, "relay", "the relay itself is still read")
}

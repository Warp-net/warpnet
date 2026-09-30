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

//nolint:all
package pubsub

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/rating"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/require"
)

// An author publishes to its own timeline without subscribing to it, so its
// posts reach followers through gossipsub fanout.
const timelineTopic = "user-update-rated-author"

const (
	readTimeout = 30 * time.Second
	quietWindow = 3 * time.Second
	publishTick = 200 * time.Millisecond
)

func skipWithoutHosts(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("runs live libp2p hosts")
	}
}

// ratedGossip is a node whose gossipsub weighs its peers by a real rating.
func ratedGossip(t *testing.T) (*Gossip, *liveNode, *rating.PeersRatings) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ratings := rating.NewPeersRatings()
	node := newLiveNode(t)
	g := NewGossip(ctx, ratings)
	require.NoError(t, g.Run(node))
	t.Cleanup(func() { _ = g.Close() })
	return g, node, ratings
}

// postInbox keeps every post a topic handler was handed.
type postInbox struct {
	mx    sync.Mutex
	posts []string
}

func (in *postInbox) handle(data []byte) error {
	in.mx.Lock()
	defer in.mx.Unlock()
	in.posts = append(in.posts, string(data))
	return nil
}

func (in *postInbox) has(label string) bool {
	in.mx.Lock()
	defer in.mx.Unlock()
	return slices.ContainsFunc(in.posts, func(p string) bool { return strings.HasPrefix(p, label+"#") })
}

type rawPublisher interface {
	PublishRaw(topicName string, data []byte) error
}

var postSeq atomic.Int64

func publishPost(from rawPublisher, topic, label string) {
	_ = from.PublishRaw(topic, []byte(label+"#"+strconv.FormatInt(postSeq.Add(1), 10)))
}

// requireRead keeps publishing posts labelled label until one is read.
func requireRead(t *testing.T, in *postInbox, from rawPublisher, topic, label string, msgAndArgs ...any) {
	t.Helper()
	require.Eventually(t, func() bool {
		publishPost(from, topic, label)
		return in.has(label)
	}, readTimeout, publishTick, msgAndArgs...)
}

// requireNotRead keeps publishing posts labelled label and makes sure none is read.
func requireNotRead(t *testing.T, in *postInbox, from rawPublisher, topic, label string, msgAndArgs ...any) {
	t.Helper()
	require.Never(t, func() bool {
		publishPost(from, topic, label)
		return in.has(label)
	}, quietWindow, publishTick, msgAndArgs...)
}

// requireFollowedBy waits until g knows follower is subscribed to topic.
func requireFollowedBy(t *testing.T, g *Gossip, topic string, follower peer.ID, msgAndArgs ...any) {
	t.Helper()
	require.Eventually(t, func() bool {
		return slices.Contains(g.pubsub.ListPeers(topic), follower)
	}, readTimeout, 50*time.Millisecond, msgAndArgs...)
}

func TestAFlooredPeerIsNotReadUntilItsRatingRecovers(t *testing.T) {
	skipWithoutHosts(t)

	reader, readerNode, ratings := ratedGossip(t)
	timeline := &postInbox{}
	require.NoError(t, reader.SubscribeRaw(timelineTopic, timeline.handle))

	floored, flooredNode, _ := ratedGossip(t)
	trusted, trustedNode, _ := ratedGossip(t)
	connect(t, flooredNode, readerNode)
	connect(t, trustedNode, readerNode)

	requireRead(t, timeline, floored, timelineTopic, "unrated", "a peer nobody has rated is read")

	ratings.Rate(flooredNode.host.ID(), rating.TierFloor)
	requireRead(t, timeline, trusted, timelineTopic, "trusted", "a trusted peer is read as before")
	requireFollowedBy(t, floored, timelineTopic, readerNode.host.ID(),
		"the floored peer still pushes its posts to the reader")
	requireNotRead(t, timeline, floored, timelineTopic, "floored", "the reader graylists a floored peer")

	ratings.Rate(flooredNode.host.ID(), rating.TierTrusted)
	requireRead(t, timeline, floored, timelineTopic, "recovered", "a peer whose rating recovers is read again")
}

// A follower reads an author's posts unless the author scores under the
// graylist threshold (-100), and pushes its own posts through fanout to every
// follower above it: on a timeline only the floor loses anything.
func TestOnlyAFlooredPeerIsGraylisted(t *testing.T) {
	skipWithoutHosts(t)
	const readerTimeline, peerTimeline = "user-update-rated-reader", "user-update-rated-peer"

	for _, tc := range []struct {
		tier     rating.Tier
		isRead   bool
		isServed bool
	}{
		{tier: rating.TierTrusted, isRead: true, isServed: true},
		{tier: rating.TierWatched, isRead: true, isServed: true},
		{tier: rating.TierDegraded, isRead: true, isServed: true},
		{tier: rating.TierFloor, isRead: false, isServed: false},
	} {
		t.Run(tc.tier.String(), func(t *testing.T) {
			reader, readerNode, ratings := ratedGossip(t)
			rated, ratedNode, _ := ratedGossip(t)
			fromRated, fromReader := &postInbox{}, &postInbox{}
			require.NoError(t, reader.SubscribeRaw(peerTimeline, fromRated.handle))
			require.NoError(t, rated.SubscribeRaw(readerTimeline, fromReader.handle))

			// rated before they meet, so no fanout picked the peer at a better tier
			ratings.Rate(ratedNode.host.ID(), tc.tier)
			connect(t, ratedNode, readerNode)

			if tc.isRead {
				requireRead(t, fromRated, rated, peerTimeline, "post", "a %s author is read", tc.tier)
			} else {
				requireNotRead(t, fromRated, rated, peerTimeline, "post", "a %s author is not read", tc.tier)
			}

			requireFollowedBy(t, reader, readerTimeline, ratedNode.host.ID(),
				"the reader knows the %s peer follows it", tc.tier)
			if tc.isServed {
				requireRead(t, fromReader, reader, readerTimeline, "post", "a %s follower is served", tc.tier)
			} else {
				requireNotRead(t, fromReader, reader, readerTimeline, "post", "a %s follower is not served", tc.tier)
			}
		})
	}
}

// The rating weighs peers, not what they post: gossipsub graylists the hop a
// message comes from, so a floored author is still read through another
// follower that relays for it.
func TestAFlooredAuthorIsStillReadThroughAnotherPeer(t *testing.T) {
	skipWithoutHosts(t)

	reader, readerNode, ratings := ratedGossip(t)
	timeline := &postInbox{}
	require.NoError(t, reader.SubscribeRaw(timelineTopic, timeline.handle))

	floored, flooredNode, _ := ratedGossip(t)
	ratings.Rate(flooredNode.host.ID(), rating.TierFloor)
	connect(t, flooredNode, readerNode)
	requireNotRead(t, timeline, floored, timelineTopic, "direct", "the reader drops what the floored peer sends it")

	relay, relayNode, _ := ratedGossip(t)
	require.NoError(t, relay.SubscribeRaw(timelineTopic, func([]byte) error { return nil }))
	connect(t, relayNode, readerNode)
	connect(t, flooredNode, relayNode)
	requireRead(t, timeline, floored, timelineTopic, "relayed", "a follower in between hands the reader its posts")
}

// pruneWatch records the peers that turned this router's GRAFT down.
type pruneWatch struct {
	mx sync.Mutex
	by map[peer.ID]bool
}

func (w *pruneWatch) Prune(p peer.ID, _ string) {
	w.mx.Lock()
	defer w.mx.Unlock()
	w.by[p] = true
}

func (w *pruneWatch) isPrunedBy(p peer.ID) bool {
	w.mx.Lock()
	defer w.mx.Unlock()
	return w.by[p]
}

func (*pruneWatch) OnNewOutboundStream(peer.ID, protocol.ID) {}
func (*pruneWatch) OnClosedOutboundStream(peer.ID)           {}
func (*pruneWatch) Join(string)                              {}
func (*pruneWatch) Leave(string)                             {}
func (*pruneWatch) Graft(peer.ID, string)                    {}
func (*pruneWatch) ValidateMessage(*pubsub.Message)          {}
func (*pruneWatch) DeliverMessage(*pubsub.Message)           {}
func (*pruneWatch) RejectMessage(*pubsub.Message, string)    {}
func (*pruneWatch) DuplicateMessage(*pubsub.Message)         {}
func (*pruneWatch) ThrottlePeer(peer.ID)                     {}
func (*pruneWatch) RecvRPC(*pubsub.RPC)                      {}
func (*pruneWatch) SendRPC(*pubsub.RPC, peer.ID)             {}
func (*pruneWatch) DropRPC(*pubsub.RPC, peer.ID)             {}
func (*pruneWatch) UndeliverableMessage(*pubsub.Message)     {}

// meshPeer publishes on a topic it relays, the way every CRDT broadcaster
// does: through its mesh, never through fanout.
type meshPeer struct {
	node   *liveNode
	topic  *pubsub.Topic
	prunes *pruneWatch
}

func newMeshPeer(t *testing.T, topicName string) *meshPeer {
	t.Helper()
	node := newLiveNode(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	prunes := &pruneWatch{by: map[peer.ID]bool{}}
	ps, err := pubsub.NewGossipSub(ctx, node.host, pubsub.WithRawTracer(prunes))
	require.NoError(t, err)
	topic, err := ps.Join(topicName)
	require.NoError(t, err)
	_, err = topic.Relay()
	require.NoError(t, err)
	return &meshPeer{node: node, topic: topic, prunes: prunes}
}

func (p *meshPeer) PublishRaw(_ string, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return p.topic.Publish(ctx, data)
}

// Through the mesh a peer reaches the reader only if the reader grafts it,
// which takes a score of zero, or reads its gossip, which takes the gossip
// threshold, just above the graylist. Watched and degraded peers lose the
// mesh and are still read, with nobody relaying for them, through gossip.
func TestADegradedPeerPublishingThroughTheMeshIsStillRead(t *testing.T) {
	skipWithoutHosts(t)
	const meshTopic = "rated-broadcast"

	reader, readerNode, ratings := ratedGossip(t)
	broadcasts := &postInbox{}
	require.NoError(t, reader.SubscribeRaw(meshTopic, broadcasts.handle))

	watched, degraded := newMeshPeer(t, meshTopic), newMeshPeer(t, meshTopic)
	ratings.Rate(watched.node.host.ID(), rating.TierWatched)
	ratings.Rate(degraded.node.host.ID(), rating.TierDegraded)
	connect(t, watched.node, readerNode)
	connect(t, degraded.node, readerNode)

	for _, p := range []*meshPeer{watched, degraded} {
		require.Eventually(t, func() bool { return p.prunes.isPrunedBy(readerNode.host.ID()) },
			readTimeout, 50*time.Millisecond, "the reader turns down the mesh link of a peer scoring under zero")
	}
	requireRead(t, broadcasts, watched, meshTopic, "watched", "a watched peer is still read through its gossip")
	requireRead(t, broadcasts, degraded, meshTopic, "degraded", "and so is a degraded one")
}

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

package pubsub

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestNewGossip(t *testing.T) {
	g := NewGossip(context.Background(), TopicHandler{
		TopicName: "topic-a",
		Handler:   func([]byte) error { return nil },
	})
	assert.NotNil(t, g)
	assert.False(t, g.IsGossipRunning())
	assert.Contains(t, g.handlersMap, "topic-a")
}

func TestGossip_NodeInfo_NilSafe(t *testing.T) {
	assert.Equal(t, warpnet.NodeInfo{}, (&Gossip{}).NodeInfo())
	assert.Equal(t, warpnet.NodeInfo{}, (*Gossip)(nil).NodeInfo())
}

// TestGossip_NotInitializedGuards verifies the public API refuses to operate
// until Run has flipped isRunning, rather than dereferencing a nil pubsub.
func TestGossip_NotInitializedGuards(t *testing.T) {
	g := NewGossip(context.Background())
	h := TopicHandler{TopicName: "t", Handler: func([]byte) error { return nil }}

	assert.ErrorIs(t, g.Subscribe(h), ErrPubsubNotInit)
	assert.ErrorIs(t, g.SubscribeRaw("t", func([]byte) error { return nil }), ErrPubsubNotInit)
	assert.ErrorIs(t, g.Unsubscribe("t"), ErrPubsubNotInit)
	assert.ErrorIs(t, g.Publish(event.Message{}, "t"), ErrPubsubNotInit)
	assert.ErrorIs(t, g.PublishRaw("t", []byte("{}")), ErrPubsubNotInit)
}

func TestGossip_Subscribers_UnknownTopic(t *testing.T) {
	g := NewGossip(context.Background())
	assert.Empty(t, g.Subscribers("missing"))
	assert.Empty(t, g.NotSubscribers("missing"))
}

// TestSelfPublish_Validation covers the message-validation branches that run
// before SelfPublish touches the node, so a zero-value Gossip is enough.
func TestSelfPublish_Validation(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		err := (&Gossip{}).SelfPublish([]byte("{"))
		assert.Error(t, err)
	})

	t.Run("empty destination", func(t *testing.T) {
		data, _ := json.Marshal(event.Message{Destination: ""})
		err := (&Gossip{}).SelfPublish(data)
		assert.ErrorIs(t, err, ErrPubsubNoPathFound)
	})

	t.Run("get route is a no-op", func(t *testing.T) {
		// IsGet() short-circuits before the node is used: a GET destination
		// is store-only and must not be re-streamed.
		data, _ := json.Marshal(event.Message{Destination: event.PUBLIC_GET_USER})
		err := (&Gossip{}).SelfPublish(data)
		assert.NoError(t, err)
	})
}

func TestNewDiscoveryTopicHandler(t *testing.T) {
	peerID, err := peer.Decode("12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j")
	assert.NoError(t, err)

	t.Run("empty data is ignored", func(t *testing.T) {
		called := false
		th := NewDiscoveryTopicHandler(func(warpnet.WarpAddrInfo) { called = true })
		assert.NoError(t, th.Handler(nil))
		assert.False(t, called)
	})

	t.Run("invalid json", func(t *testing.T) {
		th := NewDiscoveryTopicHandler(func(warpnet.WarpAddrInfo) {})
		assert.Error(t, th.Handler([]byte("{")))
	})

	t.Run("empty body", func(t *testing.T) {
		th := NewDiscoveryTopicHandler(func(warpnet.WarpAddrInfo) {})
		data, _ := json.Marshal(pubsubDiscoveryEnvelope{Body: nil})
		assert.ErrorIs(t, th.Handler(data), ErrPubsubEmptyMessage)
	})

	t.Run("valid body fans out to discovery handler", func(t *testing.T) {
		var got []peer.ID
		th := NewDiscoveryTopicHandler(func(info warpnet.WarpAddrInfo) {
			got = append(got, info.ID)
		})
		body, err := json.Marshal([]warpnet.WarpAddrInfo{{ID: peerID}})
		assert.NoError(t, err)
		data, err := json.Marshal(pubsubDiscoveryEnvelope{Body: body})
		assert.NoError(t, err)
		assert.NoError(t, th.Handler(data))
		assert.Equal(t, []peer.ID{peerID}, got)
	})
}

func TestNewDiscoveryRelayTopicHandler(t *testing.T) {
	th := NewDiscoveryRelayTopicHandler()
	assert.Equal(t, pubSubDiscoveryTopic, th.TopicName)
	assert.NoError(t, th.Handler([]byte("anything")))
	assert.NoError(t, th.Handler(nil))
}

func TestGossipErrors(t *testing.T) {
	// Sanity-check the sentinel error strings so errors.Is targets stay stable.
	assert.True(t, errors.Is(ErrPubsubNotInit, ErrPubsubNotInit))
	assert.Equal(t, "gossip: topic name is empty", ErrPubsubEmptyTopic.Error())
}

type liveNode struct {
	host warpnet.P2PNode

	mx          sync.Mutex
	selfStreams []selfStreamCall
	selfErr     error
}

type selfStreamCall struct {
	path stream.WarpRoute
	data []byte
}

func (n *liveNode) Node() warpnet.P2PNode { return n.host }

func (n *liveNode) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{ID: n.host.ID(), OwnerId: "owner-" + n.host.ID().String()}
}

func (n *liveNode) SelfStream(_, _ warpnet.WarpPeerID, path stream.WarpRoute, data any) ([]byte, error) {
	n.mx.Lock()
	defer n.mx.Unlock()
	bt, _ := data.([]byte)
	n.selfStreams = append(n.selfStreams, selfStreamCall{path: path, data: bt})
	return nil, n.selfErr
}

func (n *liveNode) calls() []selfStreamCall {
	n.mx.Lock()
	defer n.mx.Unlock()
	out := make([]selfStreamCall, len(n.selfStreams))
	copy(out, n.selfStreams)
	return out
}

func newLiveNode(t *testing.T) *liveNode {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return &liveNode{host: h}
}

func runningGossip(t *testing.T, handlers ...TopicHandler) (*Gossip, *liveNode) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	node := newLiveNode(t)
	g := NewGossip(ctx, handlers...)
	require.NoError(t, g.Run(node))
	t.Cleanup(func() { _ = g.Close() })

	require.True(t, g.IsGossipRunning())
	return g, node
}

func connect(t *testing.T, a, b *liveNode) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := a.host.Connect(ctx, peer.AddrInfo{ID: b.host.ID(), Addrs: b.host.Addrs()})
	require.NoError(t, err)
}

func TestGossip_RunTwiceIsRejected(t *testing.T) {
	g, node := runningGossip(t)

	assert.ErrorIs(t, g.Run(node), ErrAlreadyRunning,
		"a second Run must not swap the pubsub router out from under live subscriptions")
}

func TestGossip_RunWithoutNodeIsRejected(t *testing.T) {
	g := NewGossip(context.Background())
	err := g.runGossip()
	assert.Error(t, err, "gossip cannot start without a node")
	assert.False(t, g.IsGossipRunning())
}

func TestGossip_CloseIsIdempotentAndDisablesEverything(t *testing.T) {
	g, _ := runningGossip(t)

	require.NoError(t, g.SubscribeRaw("topic", func([]byte) error { return nil }))
	require.NoError(t, g.Close())
	assert.False(t, g.IsGossipRunning())

	assert.NoError(t, g.Close())

	assert.ErrorIs(t, g.Subscribe(TopicHandler{TopicName: "t", Handler: func([]byte) error { return nil }}), ErrPubsubNotInit)
	assert.ErrorIs(t, g.SubscribeRaw("t", func([]byte) error { return nil }), ErrPubsubNotInit)
	assert.ErrorIs(t, g.Unsubscribe("topic"), ErrPubsubNotInit)
	assert.ErrorIs(t, g.Publish(event.Message{}, "topic"), ErrPubsubNotInit)
	assert.ErrorIs(t, g.PublishRaw("topic", []byte("{}")), ErrPubsubNotInit)
}

func TestGossip_SubscribeRejectsEmptyTopic(t *testing.T) {
	g, _ := runningGossip(t)
	assert.ErrorIs(t, g.SubscribeRaw("", func([]byte) error { return nil }), ErrPubsubEmptyTopic)
}

func TestGossip_ResubscribeReusesTopicAndReplacesHandler(t *testing.T) {
	g, _ := runningGossip(t)

	require.NoError(t, g.SubscribeRaw("dup", func([]byte) error { return nil }))
	require.NoError(t, g.SubscribeRaw("dup", func([]byte) error { return nil }))

	g.mx.RLock()
	subs := len(g.subs)
	g.mx.RUnlock()
	assert.Equal(t, 1, subs, "re-subscribing must reuse the joined topic, not leak a second subscription")
}

func TestGossip_ResubscribeStillAllowsUnsubscribe(t *testing.T) {
	g, _ := runningGossip(t)

	require.NoError(t, g.SubscribeRaw("rejoin", func([]byte) error { return nil }))
	require.NoError(t, g.SubscribeRaw("rejoin", func([]byte) error { return nil }))
	require.NoError(t, g.SubscribeRaw("rejoin", func([]byte) error { return nil }))

	g.mx.RLock()
	subs := len(g.subs)
	g.mx.RUnlock()
	assert.Equal(t, 1, subs, "re-subscribing must not stack subscriptions")

	assert.NoError(t, g.Unsubscribe("rejoin"), "a re-subscribed topic must still be leavable")

	g.mx.RLock()
	_, stillTopic := g.topics["rejoin"]
	g.mx.RUnlock()
	assert.False(t, stillTopic)
}

func TestGossip_UnsubscribeUnknownTopicIsNoOp(t *testing.T) {
	g, _ := runningGossip(t)
	assert.NoError(t, g.Unsubscribe("never-joined"))
}

func TestGossip_UnsubscribeRemovesTopicAndHandler(t *testing.T) {
	g, _ := runningGossip(t)

	require.NoError(t, g.SubscribeRaw("leaving", func([]byte) error { return nil }))

	g.mx.RLock()
	_, hadTopic := g.topics["leaving"]
	_, hadHandler := g.handlersMap["leaving"]
	g.mx.RUnlock()
	require.True(t, hadTopic)
	require.True(t, hadHandler)

	require.NoError(t, g.Unsubscribe("leaving"))

	g.mx.RLock()
	_, stillTopic := g.topics["leaving"]
	_, stillHandler := g.handlersMap["leaving"]
	_, stillRelay := g.relayCancelFuncs["leaving"]
	subCount := len(g.subs)
	g.mx.RUnlock()

	assert.False(t, stillTopic, "topic must be dropped")
	assert.False(t, stillHandler, "handler must be dropped so late messages are not delivered")
	assert.False(t, stillRelay, "relay cancel func must be dropped")
	assert.Zero(t, subCount)
}

func TestGossip_RouterTornDownWhileWaitingForTheLock(t *testing.T) {
	g, _ := runningGossip(t)

	require.NoError(t, g.SubscribeRaw("before", func([]byte) error { return nil }))

	g.mx.Lock()
	g.pubsub = nil
	g.mx.Unlock()
	require.True(t, g.IsGossipRunning())

	assert.ErrorIs(t, g.SubscribeRaw("after", func([]byte) error { return nil }), ErrPubsubNotInit)
	assert.ErrorIs(t, g.Publish(event.Message{Body: json.RawMessage(`{}`)}, "after"), ErrPubsubNotInit)
	assert.ErrorIs(t, g.PublishRaw("after", []byte(`{}`)), ErrPubsubNotInit)
}

func TestGossip_UnsubscribeReportsATopicItCannotClose(t *testing.T) {
	g, _ := runningGossip(t)

	require.NoError(t, g.SubscribeRaw("stuck", func([]byte) error { return nil }))

	g.mx.RLock()
	topic := g.topics["stuck"]
	g.mx.RUnlock()
	require.NotNil(t, topic)

	strayRelay, err := topic.Relay()
	require.NoError(t, err)
	defer strayRelay()

	assert.Error(t, g.Unsubscribe("stuck"))
}

func TestGossip_SubscribersOnJoinedTopicWithoutPeers(t *testing.T) {
	g, _ := runningGossip(t)
	require.NoError(t, g.SubscribeRaw("lonely", func([]byte) error { return nil }))

	assert.Empty(t, g.Subscribers("lonely"), "nobody else has joined yet")
	assert.Empty(t, g.Subscribers("not-joined"))
	assert.Empty(t, g.NotSubscribers("not-joined"))
}

func captureTopic(t *testing.T, g *Gossip, topic string) <-chan []byte {
	t.Helper()
	out := make(chan []byte, 8)
	require.NoError(t, g.SubscribeRaw(topic, func(data []byte) error {
		select {
		case out <- data:
		default:
		}
		return nil
	}))
	return out
}

func TestGossip_PublishSignsAndFillsEnvelope(t *testing.T) {
	g, node := runningGossip(t)
	received := captureTopic(t, g, "signed")

	body := json.RawMessage(`{"text":"hello"}`)
	require.NoError(t, g.Publish(event.Message{
		Body:        body,
		Destination: "/public/post/tweet",
	}, "signed"))

	var got event.Message
	select {
	case raw := <-received:
		require.NoError(t, json.Unmarshal(raw, &got))
	case <-time.After(10 * time.Second):
		t.Fatal("published message never came back through the local subscription")
	}

	assert.NotEmpty(t, got.MessageId, "an unset message id must be generated for dedup")
	assert.Equal(t, node.host.ID().String(), string(got.NodeId), "the publishing node must be named")
	assert.Equal(t, "0.0.0", got.Version)
	assert.False(t, got.Timestamp.IsZero())
	assert.Equal(t, time.UTC, got.Timestamp.Location(), "timestamps must be normalised to UTC")

	sig, err := base64.StdEncoding.DecodeString(got.Signature)
	require.NoError(t, err)

	pub, err := node.host.Peerstore().PubKey(node.host.ID()).Raw()
	require.NoError(t, err)
	assert.True(t, ed25519.Verify(ed25519.PublicKey(pub), got.SigningBytes(), sig),
		"a forged or unsigned gossip message must never be indistinguishable from a real one")
}

func TestGossip_SignatureBreaksOnTamperedBody(t *testing.T) {
	g, node := runningGossip(t)
	received := captureTopic(t, g, "tamper")

	require.NoError(t, g.Publish(event.Message{
		Body:        json.RawMessage(`{"text":"original"}`),
		Destination: "/public/post/tweet",
	}, "tamper"))

	var got event.Message
	select {
	case raw := <-received:
		require.NoError(t, json.Unmarshal(raw, &got))
	case <-time.After(10 * time.Second):
		t.Fatal("no message received")
	}

	sig, err := base64.StdEncoding.DecodeString(got.Signature)
	require.NoError(t, err)
	pub, err := node.host.Peerstore().PubKey(node.host.ID()).Raw()
	require.NoError(t, err)

	got.Body = json.RawMessage(`{"text":"malicious rewrite"}`)
	assert.False(t, ed25519.Verify(ed25519.PublicKey(pub), got.SigningBytes(), sig),
		"rewriting the body must invalidate the signature")
}

func TestGossip_PublishPreservesSuppliedIdentity(t *testing.T) {
	g, _ := runningGossip(t)
	received := captureTopic(t, g, "identity")

	ts := time.Date(2026, 2, 1, 10, 0, 0, 0, time.FixedZone("CET", 3600))
	require.NoError(t, g.Publish(event.Message{
		Body:        json.RawMessage(`{}`),
		Destination: "/public/post/tweet",
		MessageId:   "stable-id",
		NodeId:      "explicit-node",
		Version:     "9.9.9",
		Timestamp:   ts,
	}, "identity"))

	var got event.Message
	select {
	case raw := <-received:
		require.NoError(t, json.Unmarshal(raw, &got))
	case <-time.After(10 * time.Second):
		t.Fatal("no message received")
	}

	assert.Equal(t, "stable-id", string(got.MessageId))
	assert.Equal(t, "explicit-node", string(got.NodeId))
	assert.Equal(t, "9.9.9", got.Version)
	assert.True(t, ts.UTC().Equal(got.Timestamp), "the supplied instant must be preserved, only the zone normalised")
}

func TestGossip_PublishToSeveralTopicsJoinsEach(t *testing.T) {
	g, _ := runningGossip(t)

	require.NoError(t, g.Publish(event.Message{
		Body:        json.RawMessage(`{}`),
		Destination: "/public/post/tweet",
	}, "fanout-a", "fanout-b"))

	g.mx.RLock()
	_, hasA := g.topics["fanout-a"]
	_, hasB := g.topics["fanout-b"]
	g.mx.RUnlock()

	assert.True(t, hasA)
	assert.True(t, hasB)
}

func TestGossip_PublishRawDeliversBytesVerbatim(t *testing.T) {
	g, _ := runningGossip(t)
	received := captureTopic(t, g, "raw")

	payload := []byte(`{"not":"an event envelope"}`)
	require.NoError(t, g.PublishRaw("raw", payload))

	select {
	case raw := <-received:
		assert.Equal(t, payload, raw, "PublishRaw must not re-encode or sign the payload")
	case <-time.After(10 * time.Second):
		t.Fatal("no raw message received")
	}
}

func TestGossip_MessageReachesAnotherNodeHandler(t *testing.T) {
	const topic = "/warpnet/test/timeline"

	delivered := make(chan []byte, 4)
	receiver, receiverNode := runningGossip(t, TopicHandler{
		TopicName: topic,
		Handler: func(data []byte) error {
			select {
			case delivered <- data:
			default:
			}
			return nil
		},
	})
	_ = receiver

	sender, senderNode := runningGossip(t)
	connect(t, senderNode, receiverNode)

	require.NoError(t, sender.SubscribeRaw(topic, func([]byte) error { return nil }))

	deadline := time.After(30 * time.Second)
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()

	for {
		require.NoError(t, sender.Publish(event.Message{
			Body:        json.RawMessage(`{"text":"cross-node"}`),
			Destination: "/public/post/tweet",
		}, topic))

		select {
		case raw := <-delivered:
			var msg event.Message
			require.NoError(t, json.Unmarshal(raw, &msg))
			assert.Equal(t, senderNode.host.ID().String(), string(msg.NodeId),
				"the receiving node must be able to attribute the message to its author")
			assert.NotEmpty(t, msg.Signature, "cross-node gossip must arrive signed")
			return
		case <-tick.C:
		case <-deadline:
			t.Fatal("gossip message never reached the peer node")
		}
	}
}

func TestGossip_SelfPublishForwardsTheSendersOwnSignature(t *testing.T) {
	g, node := runningGossip(t)

	original := event.Message{
		Body:        json.RawMessage(`{"text":"relayed"}`),
		Destination: string(event.PUBLIC_POST_RETWEET),
		Timestamp:   time.Now().UTC(),
		Signature:   "senders-own-signature",
	}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	require.NoError(t, g.SelfPublish(data))

	calls := node.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, stream.WarpRoute(original.Destination), calls[0].path)

	var forwarded event.Message
	require.NoError(t, json.Unmarshal(calls[0].data, &forwarded))
	assert.Equal(t, "senders-own-signature", forwarded.Signature,
		"a relayed message keeps its sender's signature: the self-stream reports that sender, so it is the key the middleware checks")
}

func TestGossip_SelfPublishGetRouteIsStoreOnly(t *testing.T) {
	g, node := runningGossip(t)

	data, err := json.Marshal(event.Message{Destination: string(event.PUBLIC_GET_USER)})
	require.NoError(t, err)

	require.NoError(t, g.SelfPublish(data))
	assert.Empty(t, node.calls(), "a GET route must never be replayed as a write")
}

func TestGossip_SelfPublishPropagatesStreamError(t *testing.T) {
	g, node := runningGossip(t)
	node.selfErr = assert.AnError

	data, err := json.Marshal(event.Message{
		Body:        json.RawMessage(`{}`),
		Destination: string(event.PUBLIC_POST_RETWEET),
		Timestamp:   time.Now().UTC(),
	})
	require.NoError(t, err)

	assert.ErrorIs(t, g.SelfPublish(data), assert.AnError)
}

func TestGossip_PublishPeerInfoAlwaysAdvertisesSelf(t *testing.T) {
	g, node := runningGossip(t)
	received := captureTopic(t, g, pubSubDiscoveryTopic)

	require.NoError(t, g.publishPeerInfo())

	select {
	case raw := <-received:
		var msg event.Message
		require.NoError(t, json.Unmarshal(raw, &msg))
		assert.Equal(t, pubSubDiscoveryTopic, msg.Destination)

		var infos []warpnet.WarpAddrInfo
		require.NoError(t, json.Unmarshal(msg.Body, &infos))
		require.NotEmpty(t, infos)
		assert.Equal(t, node.host.ID(), infos[0].ID, "the first advertised record is always this node")
		assert.NotEmpty(t, infos[0].Addrs)
	case <-time.After(10 * time.Second):
		t.Fatal("peer info was never published")
	}
}

func TestGossip_PublishPeerInfoIncludesConnectedPeerAndRespectsLimit(t *testing.T) {
	g, node := runningGossip(t)
	received := captureTopic(t, g, pubSubDiscoveryTopic)

	other := newLiveNode(t)
	connect(t, node, other)

	deadline := time.After(20 * time.Second)
	for {
		require.NoError(t, g.publishPeerInfo())

		select {
		case raw := <-received:
			var msg event.Message
			require.NoError(t, json.Unmarshal(raw, &msg))

			var infos []warpnet.WarpAddrInfo
			require.NoError(t, json.Unmarshal(msg.Body, &infos))
			assert.LessOrEqual(t, len(infos), defaultPublishPeerInfoLimit+1,
				"peer info must stay bounded so the discovery topic cannot be flooded")

			ids := make([]string, 0, len(infos))
			for _, i := range infos {
				ids = append(ids, i.ID.String())
			}
			if slices.Contains(ids, other.host.ID().String()) {
				return
			}
		case <-deadline:
			t.Fatal("a connected peer was never advertised in peer info")
		}
	}
}

func TestGossip_RunPeerInfoPublishingStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	node := newLiveNode(t)

	g := NewGossip(ctx)
	require.NoError(t, g.Run(node))

	done := make(chan struct{})
	go func() {
		g.runPeerInfoPublishing(10 * time.Millisecond)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("peer info publisher ignored context cancellation")
	}
	_ = g.Close()
}

func TestGossip_RunListenerStopsWhenNotRunning(t *testing.T) {
	g, _ := runningGossip(t)
	require.NoError(t, g.Close())

	done := make(chan error, 1)
	go func() { done <- g.runListener() }()

	select {
	case err := <-done:
		assert.NoError(t, err, "a closed gossip must let its listener exit cleanly")
	case <-time.After(10 * time.Second):
		t.Fatal("listener did not stop after Close")
	}
}

func TestGossip_RunListenerNilReceiver(t *testing.T) {
	assert.ErrorIs(t, (*Gossip)(nil).runListener(), ErrListenerMalformed)
}

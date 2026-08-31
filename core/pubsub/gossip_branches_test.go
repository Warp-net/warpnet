//nolint:all
package pubsub

import (
	"context"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/require"
)

func TestSubscribersAndNotSubscribers(t *testing.T) {
	g, node := runningGossip(t)

	t.Run("an unknown topic has nobody", func(t *testing.T) {
		require.Empty(t, g.Subscribers("never-joined"))
		require.Empty(t, g.NotSubscribers("never-joined"))
	})

	const topic = "peers-topic"
	require.NoError(t, g.SubscribeRaw(topic, func([]byte) error { return nil }))

	t.Run("an isolated node has no subscribers", func(t *testing.T) {
		require.Empty(t, g.Subscribers(topic))
	})

	t.Run("known peers that never joined are reported", func(t *testing.T) {
		other := newLiveNode(t)
		node.host.Peerstore().AddAddrs(other.host.ID(), other.host.Addrs(), time.Hour)

		ids := make([]warpnet.WarpPeerID, 0)
		for _, info := range g.NotSubscribers(topic) {
			ids = append(ids, info.ID)
		}
		require.Contains(t, ids, other.host.ID())
	})
}

func TestPublishRaw(t *testing.T) {
	g, _ := runningGossip(t)

	t.Run("joins the topic on demand", func(t *testing.T) {
		require.NoError(t, g.PublishRaw("fresh-topic", []byte("payload")))
	})

	t.Run("publishes to an already-joined topic", func(t *testing.T) {
		require.NoError(t, g.SubscribeRaw("joined-topic", func([]byte) error { return nil }))
		require.NoError(t, g.PublishRaw("joined-topic", []byte("payload")))
	})

	t.Run("refuses when the gossip is down", func(t *testing.T) {
		down := NewGossip(context.Background())
		require.ErrorIs(t, down.PublishRaw("topic", []byte("payload")), ErrPubsubNotInit)

		var nilGossip *Gossip
		require.ErrorIs(t, nilGossip.PublishRaw("topic", []byte("payload")), ErrPubsubNotInit)
	})
}

func TestSubscribeAppliesEveryHandler(t *testing.T) {
	g, _ := runningGossip(t)

	require.NoError(t, g.Subscribe(
		TopicHandler{TopicName: "topic-a", Handler: func([]byte) error { return nil }},
		TopicHandler{TopicName: "topic-b", Handler: func([]byte) error { return nil }},
	))

	// an empty topic name is rejected, and the failure surfaces to the caller
	require.Error(t, g.Subscribe(
		TopicHandler{TopicName: "", Handler: func([]byte) error { return nil }},
	))
}

func TestRunPreSubscribesHandlers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	node := newLiveNode(t)
	g := NewGossip(ctx, TopicHandler{
		TopicName: "preloaded-topic",
		Handler:   func([]byte) error { return nil },
	})
	require.NoError(t, g.Run(node))
	t.Cleanup(func() { _ = g.Close() })

	require.True(t, g.IsGossipRunning())
	// an isolated node has no peers on the topic, but the topic itself is joined
	require.Empty(t, g.Subscribers("preloaded-topic"))
	require.NoError(t, g.PublishRaw("preloaded-topic", []byte("payload")))
}

func TestRunRejectsAnInvalidPreSubscription(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	node := newLiveNode(t)
	g := NewGossip(ctx, TopicHandler{TopicName: "", Handler: func([]byte) error { return nil }})

	err := g.Run(node)
	require.Error(t, err)
	t.Cleanup(func() { _ = g.Close() })
}

func TestSelfPublish(t *testing.T) {
	g, node := runningGossip(t)

	t.Run("rejects a payload that is not an envelope", func(t *testing.T) {
		require.Error(t, g.SelfPublish([]byte("{")))
	})

	t.Run("rejects an envelope with no destination", func(t *testing.T) {
		payload, err := json.Marshal(event.Message{MessageId: "msg-1"})
		require.NoError(t, err)
		require.ErrorIs(t, g.SelfPublish(payload), ErrPubsubNoPathFound)
	})

	t.Run("a read route is stored without a self stream", func(t *testing.T) {
		before := len(node.calls())
		payload, err := json.Marshal(event.Message{
			MessageId: "msg-2", Destination: event.PUBLIC_GET_INFO,
		})
		require.NoError(t, err)
		require.NoError(t, g.SelfPublish(payload))
		require.Len(t, node.calls(), before)
	})

	t.Run("a write route reaches the node", func(t *testing.T) {
		before := len(node.calls())
		payload, err := json.Marshal(event.Message{
			MessageId: "msg-3", Destination: event.PUBLIC_POST_TIMELINE, NodeId: "node-1",
		})
		require.NoError(t, err)
		require.NoError(t, g.SelfPublish(payload))
		require.Greater(t, len(node.calls()), before)
	})
}

func TestRoundTripThroughAHandler(t *testing.T) {
	g, _ := runningGossip(t)

	got := make(chan []byte, 1)
	require.NoError(t, g.SubscribeRaw("loop-topic", func(data []byte) error {
		select {
		case got <- data:
		default:
		}
		return nil
	}))

	require.NoError(t, g.Publish(event.Message{
		Body:      []byte(`{"a":1}`),
		MessageId: "msg-1",
		NodeId:    "node-1",
		Timestamp: time.Now(),
	}, "loop-topic"))

	select {
	case data := <-got:
		require.NotEmpty(t, data)
	case <-time.After(15 * time.Second):
		t.Fatal("the published message never reached the topic handler")
	}
}

func TestRunListenerFallsBackToSelfPublish(t *testing.T) {
	g, node := runningGossip(t)

	// Relay-only subscription: no handler is registered for this topic, so the
	// listener must hand the message to the node itself.
	require.NoError(t, g.SubscribeRaw("relayed-topic", nil))

	require.NoError(t, g.Publish(event.Message{
		Body:        []byte(`{"a":1}`),
		MessageId:   "msg-2",
		NodeId:      "node-1",
		Destination: event.PUBLIC_POST_TIMELINE,
		Timestamp:   time.Now(),
	}, "relayed-topic"))

	require.Eventually(t, func() bool {
		return len(node.calls()) > 0
	}, 15*time.Second, 100*time.Millisecond)
}

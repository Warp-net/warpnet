//nolint:all
package pubsub

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/moderator/vote"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/stretchr/testify/require"
)

// signedEnvelope builds a gossip envelope signed by a freshly minted node key,
// mirroring what Gossip.Publish puts on the wire.
func signedEnvelope(t *testing.T, body any) ([]byte, string) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	peerID, err := warpnet.IDFromPublicKey(pub)
	require.NoError(t, err)

	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)

	msg := event.Message{
		Body:      bodyBytes,
		NodeId:    peerID.String(),
		Timestamp: time.Now(),
		MessageId: "msg-1",
		Version:   "0.0.0",
	}
	msg.Signature = security.Sign(priv, msg.SigningBytes())

	raw, err := json.Marshal(msg)
	require.NoError(t, err)
	return raw, peerID.String()
}

func TestVerifiedEnvelope(t *testing.T) {
	t.Run("garbage is an error", func(t *testing.T) {
		msg, err := verifiedEnvelope("votes", []byte("{"))
		require.Error(t, err)
		require.Nil(t, msg)
	})

	t.Run("malformed node id is dropped silently", func(t *testing.T) {
		raw, err := json.Marshal(event.Message{NodeId: "not-a-peer-id"})
		require.NoError(t, err)

		msg, err := verifiedEnvelope("votes", raw)
		require.NoError(t, err)
		require.Nil(t, msg)
	})

	t.Run("a forged signature is dropped silently", func(t *testing.T) {
		raw, _ := signedEnvelope(t, vote.Event{ReportID: "r-1"})

		var msg event.Message
		require.NoError(t, json.Unmarshal(raw, &msg))
		msg.Signature = "not-the-signature"
		tampered, err := json.Marshal(msg)
		require.NoError(t, err)

		got, err := verifiedEnvelope("votes", tampered)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("a tampered body is dropped silently", func(t *testing.T) {
		raw, _ := signedEnvelope(t, vote.Event{ReportID: "r-1"})

		var msg event.Message
		require.NoError(t, json.Unmarshal(raw, &msg))
		msg.Body = []byte(`{"report_id":"r-2"}`)
		tampered, err := json.Marshal(msg)
		require.NoError(t, err)

		got, err := verifiedEnvelope("votes", tampered)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("a correctly signed envelope passes", func(t *testing.T) {
		raw, nodeId := signedEnvelope(t, vote.Event{ReportID: "r-1"})

		msg, err := verifiedEnvelope("votes", raw)
		require.NoError(t, err)
		require.NotNil(t, msg)
		require.Equal(t, nodeId, msg.NodeId)
	})
}

// A moderator pubsub whose gossip has never run must refuse every operation
// rather than publishing into the void.
func TestModeratorPubSubBeforeRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	g := NewPubSub(ctx)
	require.NotNil(t, g)

	require.Error(t, g.PublishUpdateToFollowers("owner-1", event.PUBLIC_POST_MODERATION_RESULT, struct{}{}))
	require.Error(t, g.PublishVote(vote.Event{ReportID: "r-1"}))
	require.Error(t, g.SubscribeReports(func(event.ReportEvent) error { return nil }))
	require.Error(t, g.SubscribeVotes(func(vote.Event) error { return nil }))
	require.NoError(t, g.Close())
}

func TestModeratorPubSubNilReceiver(t *testing.T) {
	var g *moderatorPubSub
	require.Error(t, g.PublishUpdateToFollowers("owner-1", "dest", struct{}{}))
	require.Error(t, g.PublishVote(vote.Event{}))
	require.Error(t, g.SubscribeReports(func(event.ReportEvent) error { return nil }))
	require.Error(t, g.SubscribeVotes(func(vote.Event) error { return nil }))
}

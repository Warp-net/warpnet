//nolint:all
package pubsub

import (
	"context"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/moderator/vote"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/libp2p/go-libp2p"
	"github.com/stretchr/testify/require"
)

// liveNode is the minimal PubsubServerNodeConnector backed by a real libp2p
// host on loopback — gossip refuses to start without one.
type liveNode struct{ host warpnet.P2PNode }

func (n *liveNode) Node() warpnet.P2PNode { return n.host }

func (n *liveNode) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{ID: n.host.ID(), OwnerId: "moderator-owner"}
}

func (n *liveNode) SelfStream(_, _ warpnet.WarpPeerID, _ stream.WarpRoute, _ any) ([]byte, error) {
	return nil, nil
}

func (n *liveNode) GenericStream(_ string, _ stream.WarpRoute, _ any) ([]byte, error) {
	return nil, nil
}

func runningPubSub(t *testing.T) *moderatorPubSub {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	g := NewPubSub(ctx)
	require.NoError(t, g.Run(&liveNode{host: h}))
	t.Cleanup(func() { _ = g.Close() })
	return g
}

func TestModeratorPubSubRunIsIdempotent(t *testing.T) {
	g := runningPubSub(t)
	require.NoError(t, g.Run(nil), "a second Run must be a no-op, not a restart")
}

func TestModeratorPubSubPublishAndSubscribe(t *testing.T) {
	g := runningPubSub(t)

	reports := make(chan event.ReportEvent, 1)
	require.NoError(t, g.SubscribeReports(func(ev event.ReportEvent) error {
		reports <- ev
		return nil
	}))

	votes := make(chan vote.Event, 1)
	require.NoError(t, g.SubscribeVotes(func(ev vote.Event) error {
		votes <- ev
		return nil
	}))

	require.NoError(t, g.PublishVote(vote.Event{
		ReportID: "report-1",
		Type:     domain.ModerationTweetType,
		Result:   domain.FAIL,
	}))

	select {
	case got := <-votes:
		require.Equal(t, "report-1", got.ReportID)
		require.NotEmpty(t, got.ModeratorID, "the voter id comes from the verified envelope")
	case <-time.After(10 * time.Second):
		t.Fatal("published vote never came back on the votes topic")
	}

	require.NoError(t, g.PublishUpdateToFollowers(
		"owner-1", event.PUBLIC_POST_MODERATION_RESULT, event.ModerationVerdictEvent{UserID: "owner-1"},
	))
}

func TestModeratorPubSubPublishRejectsUnmarshalableBody(t *testing.T) {
	g := runningPubSub(t)
	require.Error(t, g.PublishUpdateToFollowers("owner-1", "dest", make(chan int)))
}

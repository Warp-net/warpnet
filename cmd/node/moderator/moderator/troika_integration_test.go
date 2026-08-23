//nolint:all
package moderator

import (
	"context"
	"crypto/ed25519"
	"sync"
	"testing"
	"time"

	memberpubsub "github.com/Warp-net/warpnet/cmd/node/member/pubsub"
	modpubsub "github.com/Warp-net/warpnet/cmd/node/moderator/pubsub"
	"github.com/Warp-net/warpnet/cmd/node/moderator/round"
	"github.com/Warp-net/warpnet/cmd/node/moderator/vote"
	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

// troikaConnector backs both the Moderator and its pubsub with one real
// libp2p host. Streams stay canned: PUBLIC_GET_TWEET serves the offending
// tweet, PUBLIC_POST_MODERATION_RESULT records the reporter delivery.
type troikaConnector struct {
	host    warpnet.P2PNode
	ownerId string

	mu          sync.Mutex
	deliveries  []event.ModerationVerdictEvent
	deliveredTo []string
}

func (c *troikaConnector) Node() warpnet.P2PNode  { return c.host }
func (c *troikaConnector) ID() warpnet.WarpPeerID { return c.host.ID() }
func (c *troikaConnector) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{ID: c.host.ID(), OwnerId: c.ownerId}
}
func (c *troikaConnector) SelfStream(warpnet.WarpPeerID, warpnet.WarpPeerID, stream.WarpRoute, any) ([]byte, error) {
	return nil, nil
}
func (c *troikaConnector) GenericStream(nodeIdStr string, path stream.WarpRoute, data any) ([]byte, error) {
	switch path {
	case event.PUBLIC_GET_TWEET:
		return json.Marshal(domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"})
	case event.PUBLIC_POST_MODERATION_RESULT:
		if r, ok := data.(event.ModerationVerdictEvent); ok {
			c.mu.Lock()
			c.deliveries = append(c.deliveries, r)
			c.deliveredTo = append(c.deliveredTo, nodeIdStr)
			c.mu.Unlock()
		}
		return []byte(event.Accepted), nil
	}
	return []byte(event.Accepted), nil
}

func (c *troikaConnector) takeDeliveries() ([]event.ModerationVerdictEvent, []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]event.ModerationVerdictEvent(nil), c.deliveries...),
		append([]string(nil), c.deliveredTo...)
}

func newTroikaHost(t *testing.T) warpnet.P2PNode {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func hostPrivKey(t *testing.T, h warpnet.P2PNode) ed25519.PrivateKey {
	t.Helper()
	sk := h.Peerstore().PrivKey(h.ID())
	require.NotNil(t, sk)
	raw, err := sk.Raw()
	require.NoError(t, err)
	require.Len(t, raw, ed25519.PrivateKeySize, "libp2p host identity must be ed25519")
	return ed25519.PrivateKey(raw)
}

// TestTroikaIntegration_RealGossip runs the full protocol over an actual
// gossipsub network: a member publishes a report, three moderators vote on
// the real votes topic, the deterministic chair aggregates and delivers a
// signed verdict to the reporter exactly once, and the Final announcement
// clears the round on every moderator.
func TestTroikaIntegration_RealGossip(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Three moderators on real hosts.
	moderators := make([]*Moderator, 0, 3)
	connectors := make([]*troikaConnector, 0, 3)
	hosts := make([]warpnet.P2PNode, 0, 4)
	for i := 0; i < 3; i++ {
		h := newTroikaHost(t)
		conn := &troikaConnector{host: h}
		ps := modpubsub.NewPubSub(ctx)
		require.NoError(t, ps.Run(conn))
		t.Cleanup(func() { _ = ps.Close() })

		m, err := NewModerator(ctx, conn, ps, ps, ps, hostPrivKey(t, h))
		require.NoError(t, err)
		// Short window so the round closes inside the test; no volunteer
		// suppression or takeover is exercised here.
		m.rounds = round.NewRegistry(m.selfID(), m, round.Schedule{
			Window: 5 * time.Second, Failover: time.Hour, Step: time.Hour,
		})
		require.NoError(t, m.Start())
		t.Cleanup(m.Close)

		moderators = append(moderators, m)
		connectors = append(connectors, conn)
		hosts = append(hosts, h)
	}

	// The reporting member node, also observing the offender's followers
	// topic so the isolation broadcast can be asserted.
	memberHost := newTroikaHost(t)
	hosts = append(hosts, memberHost)
	memberConn := &troikaConnector{host: memberHost, ownerId: "reporter-owner"}
	memberPS := memberpubsub.NewPubSub(ctx)
	memberPS.Run(memberConn)
	t.Cleanup(func() { _ = memberPS.Close() })
	require.True(t, memberPS.Gossip().IsGossipRunning())
	require.NoError(t, memberPS.SubscribeUserUpdate("offender"))
	// Join the reports and votes topics locally so Subscribers() can
	// observe the moderators' mesh membership below.
	require.NoError(t, memberPS.Gossip().SubscribeRaw(event.ReportsTopic, func([]byte) error { return nil }))
	require.NoError(t, memberPS.Gossip().SubscribeRaw(vote.Topic, func([]byte) error { return nil }))

	// Full mesh between all four hosts.
	for i, a := range hosts {
		for _, b := range hosts[i+1:] {
			require.NoError(t, a.Connect(ctx, peer.AddrInfo{ID: b.ID(), Addrs: b.Addrs()}))
		}
	}

	// Wait for the gossip mesh: the member must see all three moderators
	// on both topics before the report is worth publishing, plus a couple
	// of heartbeats for the mesh GRAFT to complete — subscription
	// announcements land before the actual mesh links do.
	require.Eventually(t, func() bool {
		return len(memberPS.Gossip().Subscribers(event.ReportsTopic)) >= 3 &&
			len(memberPS.Gossip().Subscribers(vote.Topic)) >= 3
	}, 20*time.Second, 200*time.Millisecond, "moderators never joined the moderation topics")
	time.Sleep(2 * time.Second)

	require.NoError(t, memberPS.PublishReport(event.ReportEvent{
		Type:         domain.ModerationTweetType,
		TargetUserID: "offender",
		TargetNodeID: memberHost.ID().String(),
		ObjectID:     func() *domain.ID { id := domain.ID("tweet-1"); return &id }(),
		Reason:       "Hate",
	}))

	// Exactly one moderator (the deterministic chair) delivers the verdict.
	countDeliveries := func() (int, event.ModerationVerdictEvent, string) {
		total := 0
		var got event.ModerationVerdictEvent
		var to string
		for _, c := range connectors {
			ds, tos := c.takeDeliveries()
			total += len(ds)
			if len(ds) > 0 {
				got, to = ds[0], tos[0]
			}
		}
		return total, got, to
	}
	require.Eventually(t, func() bool {
		n, _, _ := countDeliveries()
		return n >= 1
	}, 20*time.Second, 200*time.Millisecond, "no verdict was ever delivered")

	// Grace period: a second finalizer would surface here.
	time.Sleep(1 * time.Second)
	total, got, deliveredTo := countDeliveries()
	require.Equal(t, 1, total, "exactly one moderator must deliver the verdict")

	require.Equal(t, memberHost.ID().String(), deliveredTo, "the verdict must go to the reporter's node")
	require.Equal(t, domain.FAIL, got.Verdict)
	require.Equal(t, memberPS.OwnerID(), got.ReporterID, "the reporter identity must be the one the member node stamped")
	require.Len(t, got.Voters, 3, "all three moderators must have voted")

	// The signature must verify against the pubkey recovered from the
	// chair's peer id — the exact check member nodes run.
	chairPeer := warpnet.FromStringToPeerID(got.ModeratorID)
	require.NotEmpty(t, chairPeer)
	pubKey := warpnet.FromIDToPubKey(chairPeer)
	require.NotEmpty(t, pubKey)
	require.NoError(t, got.Verify(pubKey))

	// The Final announcement clears the round everywhere: no parked
	// takeovers may remain.
	require.Eventually(t, func() bool {
		for _, m := range moderators {
			if m.rounds.Len() != 0 {
				return false
			}
		}
		return true
	}, 10*time.Second, 200*time.Millisecond, "the Final announcement must clear every moderator's round")
}

func (*troikaConnector) Rating() rating.Rater { return rating.Nop{} }

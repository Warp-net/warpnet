//nolint:all
package moderator

// Regression: a Final announcement is honored only from a peer that voted in
// the round (round.hasVoter, consulted by Registry.MarkFinalized). An ordinary
// member node that never judged anything cannot end a round by broadcasting a
// signed Final, so it can neither silence a report nor relabel the audit
// reference. Before the fix such a Final closed every moderator's round before
// the tally and filed the judged text under the stranger's chosen verdict.

import (
	"context"
	"sync"
	"testing"
	"time"

	memberpubsub "github.com/Warp-net/warpnet/cmd/node/member/pubsub"
	modpubsub "github.com/Warp-net/warpnet/cmd/node/moderator/pubsub"
	"github.com/Warp-net/warpnet/cmd/node/moderator/round"
	"github.com/Warp-net/warpnet/cmd/node/moderator/vote"
	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestTroikaIntegration_ForeignFinalRejected(t *testing.T) {
	// Every moderator's model condemns the tweet: the honest outcome is FAIL.
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	const window = 10 * time.Second

	moderators := make([]*Moderator, 0, 3)
	connectors := make([]*troikaConnector, 0, 3)
	hosts := make([]warpnet.P2PNode, 0, 5)
	for i := 0; i < 3; i++ {
		h := newTroikaHost(t)
		conn := &troikaConnector{host: h}
		ps := modpubsub.NewPubSub(ctx, rating.NewPeersRatings())
		require.NoError(t, ps.Run(conn))
		t.Cleanup(func() { _ = ps.Close() })

		m, err := NewModerator(ctx, conn, ps, ps, ps, hostPrivKey(t, h))
		require.NoError(t, err)
		// No volunteer suppression or takeover: all three vote at once and
		// the deterministic chair carries the decision at the window close.
		m.rounds = round.NewRegistry(m.selfID(), m, round.Schedule{
			Window: window, Failover: time.Hour, Step: time.Hour,
		})
		require.NoError(t, m.Start())
		t.Cleanup(m.Close)

		moderators = append(moderators, m)
		connectors = append(connectors, conn)
		hosts = append(hosts, h)
	}

	reporterHost := newTroikaHost(t)
	hosts = append(hosts, reporterHost)
	reporterPS := memberpubsub.NewPubSub(ctx, rating.NewPeersRatings(), nil)
	reporterPS.Run(&troikaConnector{host: reporterHost, ownerId: "reporter-owner"})
	t.Cleanup(func() { _ = reporterPS.Close() })
	require.NoError(t, reporterPS.Gossip().SubscribeRaw(event.ReportsTopic, func([]byte) error { return nil }))

	// The attacker is an ordinary member node: no engine, no ballot, no role.
	// It only listens to the two moderation topics to learn the round id.
	attackerHost := newTroikaHost(t)
	hosts = append(hosts, attackerHost)
	attackerPS := memberpubsub.NewPubSub(ctx, rating.NewPeersRatings(), nil)
	attackerPS.Run(&troikaConnector{host: attackerHost, ownerId: "attacker-owner"})
	t.Cleanup(func() { _ = attackerPS.Close() })

	var (
		mu       sync.Mutex
		reportID string
		voters   = map[domain.ID]struct{}{}
	)
	require.NoError(t, attackerPS.Gossip().SubscribeRaw(event.ReportsTopic, func(data []byte) error {
		var msg event.Message
		var rep event.ReportEvent
		if json.Unmarshal(data, &msg) != nil || json.Unmarshal(msg.Body, &rep) != nil {
			return nil
		}
		mu.Lock()
		reportID = rep.ReportID()
		mu.Unlock()
		return nil
	}))
	require.NoError(t, attackerPS.Gossip().SubscribeRaw(vote.Topic, func(data []byte) error {
		var msg event.Message
		var v vote.Event
		if json.Unmarshal(data, &msg) != nil || json.Unmarshal(msg.Body, &v) != nil || v.Final {
			return nil
		}
		mu.Lock()
		voters[msg.NodeId] = struct{}{}
		mu.Unlock()
		return nil
	}))

	for i, a := range hosts {
		for _, b := range hosts[i+1:] {
			require.NoError(t, a.Connect(ctx, peer.AddrInfo{ID: b.ID(), Addrs: b.Addrs()}))
		}
	}
	require.Eventually(t, func() bool {
		return len(reporterPS.Gossip().Subscribers(event.ReportsTopic)) >= 3 &&
			len(attackerPS.Gossip().Subscribers(event.ReportsTopic)) >= 3 &&
			len(attackerPS.Gossip().Subscribers(vote.Topic)) >= 3
	}, 20*time.Second, 200*time.Millisecond, "moderators never joined the moderation topics")
	time.Sleep(2 * time.Second)

	require.NoError(t, reporterPS.PublishReport(event.ReportEvent{
		Type:         domain.ModerationTweetType,
		TargetUserID: "offender",
		TargetNodeID: reporterHost.ID().String(),
		ObjectID:     func() *domain.ID { id := domain.ID("tweet-1"); return &id }(),
		Reason:       "Hate",
	}))
	opened := time.Now()

	// Wait until the attacker has seen the report and all three ballots, so
	// the rounds are open and fully voted when the forged Final lands.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return reportID != "" && len(voters) == 3
	}, window/2, 100*time.Millisecond, "the attacker never saw the report and all three ballots")

	// A Final the attacker was never entitled to, with the verdict reversed.
	mu.Lock()
	id := reportID
	mu.Unlock()
	body, err := json.Marshal(vote.Event{ReportID: id, Final: true, Result: domain.OK})
	require.NoError(t, err)
	require.NoError(t, attackerPS.Gossip().Publish(event.Message{Body: body}, vote.Topic))

	// The forged Final does not silence the open rounds.
	time.Sleep(time.Second)
	require.Less(t, time.Since(opened), window, "sanity: the window must still be open here")
	for _, m := range moderators {
		require.Equal(t, 1, m.rounds.Len(), "a non-voter's Final must not close the round")
	}

	// The deterministic chair still carries the honest verdict, exactly once.
	countDeliveries := func() (int, event.ModerationVerdictEvent) {
		total := 0
		var got event.ModerationVerdictEvent
		for _, c := range connectors {
			ds, _ := c.takeDeliveries()
			total += len(ds)
			if len(ds) > 0 {
				got = ds[0]
			}
		}
		return total, got
	}
	require.Eventually(t, func() bool {
		n, _ := countDeliveries()
		return n >= 1
	}, 20*time.Second, 200*time.Millisecond, "the chair never delivered the verdict")
	time.Sleep(time.Second) // a second finalizer would surface here
	total, got := countDeliveries()
	require.Equal(t, 1, total, "exactly one moderator delivers the verdict")
	require.Equal(t, domain.FAIL, got.Verdict, "the quorum's FAIL stands; the forged OK was ignored")

	// Every moderator files the quorum's real verdict, not the stranger's.
	for _, m := range moderators {
		safe, unsafe := m.corpus.Len()
		require.Equal(t, 0, safe, "the forged OK must never become a reference")
		require.Equal(t, 1, unsafe, "the quorum's FAIL is the only reference")
	}

	// The chair's own Final clears the rounds everywhere.
	require.Eventually(t, func() bool {
		for _, m := range moderators {
			if m.rounds.Len() != 0 {
				return false
			}
		}
		return true
	}, 10*time.Second, 200*time.Millisecond, "the chair's Final must clear every round")
}

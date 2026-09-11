// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package pubsub

import (
	"context"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const scoredPeer = "12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo"

// appScore is what gossipsub asks on every scoring pass.
func appScore(t *testing.T, g *Gossip, peerID warpnet.WarpPeerID) float64 {
	t.Helper()
	opts := g.scoreOptions()
	require.Len(t, opts, 1, "gossipsub is configured with the peer score and nothing else")
	return g.standings.Peer(peerID).GossipScore
}

func TestAPeerNobodyHasRatedScoresLikeOneInGoodStanding(t *testing.T) {
	g := NewGossip(context.Background(), warpnet.NewPeerStandings())

	assert.Equal(t, float64(0), appScore(t, g, warpnet.FromStringToPeerID(scoredPeer)))
}

func TestAPeerScoresWhatItsStandingSays(t *testing.T) {
	standings := warpnet.NewPeerStandings()
	g := NewGossip(context.Background(), standings)
	peerID := warpnet.FromStringToPeerID(scoredPeer)

	standings.Apply(warpnet.PeerStanding{PeerID: scoredPeer, GossipScore: -60})
	assert.Equal(t, float64(-60), appScore(t, g, peerID))

	standings.Apply(warpnet.PeerStanding{PeerID: scoredPeer, GossipScore: 0})
	assert.Equal(t, float64(0), appScore(t, g, peerID), "a standing that recovers is scored again")
}

// Only a peer this node witnessed misbehaving itself goes low enough to
// be graylisted; remote evidence alone cannot take it there.
func TestOnlyTheWorstStandingIsGraylisted(t *testing.T) {
	standings := warpnet.NewPeerStandings()
	g := NewGossip(context.Background(), standings)
	peerID := warpnet.FromStringToPeerID(scoredPeer)

	for _, tc := range []struct {
		score     float64
		graylists bool
	}{
		{score: 0, graylists: false},
		{score: -10, graylists: false},
		{score: -60, graylists: false},
		{score: -200, graylists: true},
	} {
		standings.Apply(warpnet.PeerStanding{PeerID: scoredPeer, GossipScore: tc.score})
		assert.Equal(t, tc.graylists, appScore(t, g, peerID) < graylistThreshold, "score %v", tc.score)
	}
}

func TestAGossipWithNoStandingsScoresNobody(t *testing.T) {
	g := NewGossip(context.Background(), nil)

	assert.Equal(t, float64(0), appScore(t, g, warpnet.FromStringToPeerID(scoredPeer)))
}

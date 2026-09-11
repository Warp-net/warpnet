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

// scoredBy is what gossipsub weighs a peer by on every scoring pass.
func scoredBy(t *testing.T, g *Gossip, peerID warpnet.WarpPeerID) float64 {
	t.Helper()
	require.Len(t, g.scoreOptions(), 1,
		"gossipsub is configured with the peer score and nothing else")
	return g.peerScore(peerID)
}

func TestAPeerNobodyHasRatedScoresLikeAnyOther(t *testing.T) {
	g := NewGossip(context.Background(), warpnet.NewPeerLimiter())

	assert.Equal(t, float64(0), scoredBy(t, g, warpnet.FromStringToPeerID(scoredPeer)))
}

func TestAPeerScoresWhatItsLimitsSay(t *testing.T) {
	limits := warpnet.NewPeerLimiter()
	g := NewGossip(context.Background(), limits)
	peerID := warpnet.FromStringToPeerID(scoredPeer)

	limits.Limit(warpnet.PeerLimits{PeerID: scoredPeer, GossipScore: -60})
	assert.Equal(t, float64(-60), scoredBy(t, g, peerID))

	limits.Limit(warpnet.PeerLimits{PeerID: scoredPeer, GossipScore: 0})
	assert.Equal(t, float64(0), scoredBy(t, g, peerID), "a score that recovers is weighed again")
}

// Only a peer this node witnessed misbehaving itself goes low enough to
// be graylisted; remote evidence alone cannot take it there.
func TestOnlyTheWorstScoreIsGraylisted(t *testing.T) {
	limits := warpnet.NewPeerLimiter()
	g := NewGossip(context.Background(), limits)
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
		limits.Limit(warpnet.PeerLimits{PeerID: scoredPeer, GossipScore: tc.score})
		assert.Equal(t, tc.graylists, scoredBy(t, g, peerID) < graylistThreshold, "score %v", tc.score)
	}
}

func TestAGossipWithNoLimiterScoresEveryPeerTheSame(t *testing.T) {
	g := NewGossip(context.Background(), nil)

	assert.Equal(t, float64(0), scoredBy(t, g, warpnet.FromStringToPeerID(scoredPeer)))
}

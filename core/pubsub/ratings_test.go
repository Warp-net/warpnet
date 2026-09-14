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

// scoring answers what it was told to score a peer.
type scoring map[string]float64

func (s scoring) GossipScore(peerID warpnet.WarpPeerID) float64 {
	return s[peerID.String()]
}

// scoredBy is what gossipsub weighs a peer by on every scoring pass.
func scoredBy(t *testing.T, g *Gossip, peerID warpnet.WarpPeerID) float64 {
	t.Helper()
	require.Len(t, g.scoreOptions(), 1,
		"gossipsub is configured with the peer score and nothing else")
	return g.peerScore(peerID)
}

func TestAPeerNobodyHasRatedScoresLikeAnyOther(t *testing.T) {
	g := NewGossip(context.Background(), scoring{})

	assert.Equal(t, float64(0), scoredBy(t, g, warpnet.FromStringToPeerID(scoredPeer)))
}

func TestAPeerScoresWhatItsRatingSays(t *testing.T) {
	scores := scoring{}
	g := NewGossip(context.Background(), scores)
	peerID := warpnet.FromStringToPeerID(scoredPeer)

	scores[scoredPeer] = -60
	assert.Equal(t, float64(-60), scoredBy(t, g, peerID))

	scores[scoredPeer] = 0
	assert.Equal(t, float64(0), scoredBy(t, g, peerID), "a rating that recovers is weighed again")
}

// Only a peer this node witnessed misbehaving itself goes low enough to
// be graylisted; remote evidence alone cannot take it there.
func TestOnlyTheWorstScoreIsGraylisted(t *testing.T) {
	scores := scoring{}
	g := NewGossip(context.Background(), scores)
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
		scores[scoredPeer] = tc.score
		assert.Equal(t, tc.graylists, scoredBy(t, g, peerID) < graylistThreshold, "score %v", tc.score)
	}
}

func TestAGossipWithNoRatingsScoresEveryPeerTheSame(t *testing.T) {
	g := NewGossip(context.Background(), nil)

	assert.Equal(t, float64(0), scoredBy(t, g, warpnet.FromStringToPeerID(scoredPeer)))
}

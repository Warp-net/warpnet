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

func TestAPeerNobodyHasRatedScoresLikeOneInGoodStanding(t *testing.T) {
	g := NewGossip(context.Background())

	assert.Equal(t, float64(0), g.appScore(warpnet.FromStringToPeerID(scoredPeer)))
}

func TestAPeerScoresWhatItsStandingSays(t *testing.T) {
	g := NewGossip(context.Background())
	peerID := warpnet.FromStringToPeerID(scoredPeer)

	g.Apply(warpnet.PeerStanding{PeerID: scoredPeer, GossipScore: -60})
	assert.Equal(t, float64(-60), g.appScore(peerID))

	g.Apply(warpnet.PeerStanding{PeerID: scoredPeer, GossipScore: 0})
	assert.Equal(t, float64(0), g.appScore(peerID), "a standing that recovers is scored again")
}

// Only a peer this node witnessed misbehaving itself goes low enough to
// be graylisted; remote evidence alone cannot take it there.
func TestOnlyTheWorstStandingIsGraylisted(t *testing.T) {
	g := NewGossip(context.Background())
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
		g.Apply(warpnet.PeerStanding{PeerID: scoredPeer, GossipScore: tc.score})
		assert.Equal(t, tc.graylists, g.appScore(peerID) < graylistThreshold, "score %v", tc.score)
	}
}

func TestScoreOptionsAskTheStandingOfEveryPeer(t *testing.T) {
	g := NewGossip(context.Background())
	g.Apply(warpnet.PeerStanding{PeerID: scoredPeer, GossipScore: -60})

	opts := g.scoreOptions()
	require.Len(t, opts, 1, "gossipsub is configured with the peer score and nothing else")
}

func TestApplyIgnoresAStandingThatNamesNobody(t *testing.T) {
	g := NewGossip(context.Background())

	assert.NotPanics(t, func() { g.Apply(warpnet.PeerStanding{GossipScore: -200}) })

	var nilGossip *Gossip
	assert.NotPanics(t, func() { nilGossip.Apply(warpnet.PeerStanding{PeerID: scoredPeer}) })
	assert.Equal(t, float64(0), nilGossip.appScore(warpnet.FromStringToPeerID(scoredPeer)))
}

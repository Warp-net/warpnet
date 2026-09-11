// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package warpnet

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const limitedPeer = "12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo"

func TestAPeerNobodyHasRatedMayHaveEverything(t *testing.T) {
	limiter := NewPeerLimiter()

	limits := limiter.PeerLimits(FromStringToPeerID(limitedPeer))
	assert.Equal(t, float64(1), limits.RateMultiplier, "it spends its whole allowance")
	assert.True(t, limits.InRoutingTable, "and keeps its place in the routing table")
	assert.Zero(t, limits.GossipScore)
	assert.Zero(t, limits.ConnTag)
}

func TestLimitsAreRememberedUntilTheyAreReplaced(t *testing.T) {
	limiter := NewPeerLimiter()
	peerID := FromStringToPeerID(limitedPeer)

	limiter.Limit(PeerLimits{
		PeerID: limitedPeer, ConnTag: 10, GossipScore: -60, RateMultiplier: 0.25,
	})

	limits := limiter.PeerLimits(peerID)
	assert.Equal(t, 10, limits.ConnTag)
	assert.Equal(t, float64(-60), limits.GossipScore)
	assert.Equal(t, 0.25, limits.RateMultiplier)
	assert.False(t, limits.InRoutingTable)

	limiter.Limit(PeerLimits{
		PeerID: limitedPeer, ConnTag: 60, RateMultiplier: 1, InRoutingTable: true,
	})

	limits = limiter.PeerLimits(peerID)
	assert.Equal(t, 60, limits.ConnTag, "limits that recover replace the old ones")
	assert.True(t, limits.InRoutingTable)
}

func TestLimitsThatNameNobodyAreIgnored(t *testing.T) {
	limiter := NewPeerLimiter()

	limiter.Limit(PeerLimits{RateMultiplier: 0.1})

	assert.Equal(t, float64(1), limiter.PeerLimits("").RateMultiplier)
}

func TestANilLimiterLimitsNobody(t *testing.T) {
	var limiter *PeerLimiter
	peerID := FromStringToPeerID(limitedPeer)

	assert.NotPanics(t, func() {
		limiter.Limit(PeerLimits{PeerID: limitedPeer, RateMultiplier: 0.1})
	})
	assert.Equal(t, float64(1), limiter.PeerLimits(peerID).RateMultiplier)
	assert.True(t, limiter.PeerLimits(peerID).InRoutingTable)
}

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTierThresholds(t *testing.T) {
	for _, tc := range []struct {
		score Score
		want  Tier
	}{
		{MaxScore, TierTrusted},
		{800, TierTrusted},
		{799, TierWatched},
		{600, TierWatched}, // the remote-only floor
		{500, TierWatched},
		{499, TierDegraded},
		{200, TierDegraded},
		{199, TierFloor},
		{MinScore, TierFloor},
	} {
		assert.Equal(t, tc.want, TierOf(tc.score), "score %d", tc.score)
	}
}

func TestEnforcementKnobsWorsenMonotonically(t *testing.T) {
	bands := []Tier{TierTrusted, TierWatched, TierDegraded, TierFloor}

	for i := 1; i < len(bands); i++ {
		prev, cur := bands[i-1], bands[i]
		assert.Less(t, ConnTagValue(cur), ConnTagValue(prev),
			"connection priority must fall as standing falls")
		assert.Less(t, GossipAppScore(cur), GossipAppScore(prev),
			"gossipsub score must fall as standing falls")
		assert.Less(t, LimitMultiplier(cur), LimitMultiplier(prev),
			"rate limits must tighten as standing falls")
	}
}

func TestOnlyFloorIsGraylistedAndEvictedFromDHT(t *testing.T) {
	for _, b := range []Tier{TierTrusted, TierWatched, TierDegraded} {
		assert.Greater(t, GossipAppScore(b), float64(GossipGraylistThreshold),
			"%s must stay above the gossipsub graylist", b)
		assert.True(t, AllowInDHT(b), "%s must stay in the routing table", b)
	}
	assert.Less(t, GossipAppScore(TierFloor), float64(GossipGraylistThreshold))
	assert.False(t, AllowInDHT(TierFloor))
}

func TestTrustedIsInert(t *testing.T) {
	assert.Equal(t, float64(0), GossipAppScore(TierTrusted))
	assert.Equal(t, float64(1), LimitMultiplier(TierTrusted))
	assert.True(t, AllowInDHT(TierTrusted))
}

func TestLimitMultiplierNeverReachesZero(t *testing.T) {
	// A low rating slows a peer down; it never refuses it service.
	for _, b := range []Tier{TierTrusted, TierWatched, TierDegraded, TierFloor} {
		assert.Positive(t, LimitMultiplier(b), "%s must still be served", b)
	}
}

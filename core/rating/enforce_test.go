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
		assert.Equal(t, tc.want, tc.score.Tier(), "score %d", tc.score)
	}
}

func TestEnforcementKnobsWorsenMonotonically(t *testing.T) {
	tiers := []Tier{TierTrusted, TierWatched, TierDegraded, TierFloor}

	for i := 1; i < len(tiers); i++ {
		prev, cur := tiers[i-1], tiers[i]
		assert.Less(t, cur.ConnTag(), prev.ConnTag(), "connection priority must fall as standing falls")
		assert.Less(t, cur.GossipScore(), prev.GossipScore(), "gossipsub score must fall as standing falls")
		assert.Less(t, cur.LimitMultiplier(), prev.LimitMultiplier(), "rate limits must tighten as standing falls")
	}
}

func TestOnlyFloorIsGraylistedAndEvictedFromDHT(t *testing.T) {
	for _, tier := range []Tier{TierTrusted, TierWatched, TierDegraded} {
		assert.Greater(t, tier.GossipScore(), float64(GossipGraylistThreshold),
			"%s must stay above the gossipsub graylist", tier)
		assert.True(t, tier.AllowedInDHT(), "%s must stay in the routing table", tier)
	}
	assert.Less(t, TierFloor.GossipScore(), float64(GossipGraylistThreshold))
	assert.False(t, TierFloor.AllowedInDHT())
}

func TestTrustedIsInert(t *testing.T) {
	assert.Equal(t, float64(0), TierTrusted.GossipScore())
	assert.Equal(t, float64(1), TierTrusted.LimitMultiplier())
	assert.True(t, TierTrusted.AllowedInDHT())
}

func TestLimitMultiplierNeverReachesZero(t *testing.T) {
	// A low rating slows a peer down; it never refuses it service.
	for _, tier := range []Tier{TierTrusted, TierWatched, TierDegraded, TierFloor} {
		assert.Positive(t, tier.LimitMultiplier(), "%s must still be served", tier)
	}
}

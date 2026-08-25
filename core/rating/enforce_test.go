// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBandThresholds(t *testing.T) {
	for _, tc := range []struct {
		score Score
		want  Band
	}{
		{MaxScore, BandTrusted},
		{800, BandTrusted},
		{799, BandWatched},
		{600, BandWatched}, // the remote-only floor
		{500, BandWatched},
		{499, BandDegraded},
		{200, BandDegraded},
		{199, BandFloor},
		{MinScore, BandFloor},
	} {
		assert.Equal(t, tc.want, BandOf(tc.score), "score %d", tc.score)
	}
}

func TestEnforcementKnobsWorsenMonotonically(t *testing.T) {
	bands := []Band{BandTrusted, BandWatched, BandDegraded, BandFloor}

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
	for _, b := range []Band{BandTrusted, BandWatched, BandDegraded} {
		assert.Greater(t, GossipAppScore(b), float64(GossipGraylistThreshold),
			"%s must stay above the gossipsub graylist", b)
		assert.True(t, AllowInDHT(b), "%s must stay in the routing table", b)
	}
	assert.Less(t, GossipAppScore(BandFloor), float64(GossipGraylistThreshold))
	assert.False(t, AllowInDHT(BandFloor))
}

func TestTrustedIsInert(t *testing.T) {
	assert.Equal(t, float64(0), GossipAppScore(BandTrusted))
	assert.Equal(t, float64(1), LimitMultiplier(BandTrusted))
	assert.True(t, AllowInDHT(BandTrusted))
}

func TestLimitMultiplierNeverReachesZero(t *testing.T) {
	// A low rating slows a peer down; it never refuses it service.
	for _, b := range []Band{BandTrusted, BandWatched, BandDegraded, BandFloor} {
		assert.Positive(t, LimitMultiplier(b), "%s must still be served", b)
	}
}

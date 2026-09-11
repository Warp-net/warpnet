// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"sync"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		assert.Less(t, cur.RateMultiplier(), prev.RateMultiplier(), "rate limits must tighten as standing falls")
	}
}

func TestOnlyTheFloorLeavesTheRoutingTable(t *testing.T) {
	for _, tier := range []Tier{TierTrusted, TierWatched, TierDegraded} {
		assert.True(t, tier.InRoutingTable(), "%s must stay in the routing table", tier)
	}
	assert.False(t, TierFloor.InRoutingTable())
}

func TestTrustedIsInert(t *testing.T) {
	assert.Equal(t, float64(0), TierTrusted.GossipScore())
	assert.Equal(t, float64(1), TierTrusted.RateMultiplier())
	assert.True(t, TierTrusted.InRoutingTable())
}

func TestLimitMultiplierNeverReachesZero(t *testing.T) {
	// A low rating slows a peer down; it never refuses it service.
	for _, tier := range []Tier{TierTrusted, TierWatched, TierDegraded, TierFloor} {
		assert.Positive(t, tier.RateMultiplier(), "%s must still be served", tier)
	}
}

// recordingLimiter is whoever holds what the rating decided a peer may have.
type recordingLimiter struct {
	mu     sync.Mutex
	limits []warpnet.PeerLimits
}

func (r *recordingLimiter) Limit(limits warpnet.PeerLimits) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limits = append(r.limits, limits)
}

func (r *recordingLimiter) handed() []warpnet.PeerLimits {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]warpnet.PeerLimits(nil), r.limits...)
}

func TestLimitsAreHandedOnWhenTheyMoveAndNotBefore(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	limiter := &recordingLimiter{}
	e.Enforce(limiter, nil) // a module with nothing to apply is not a panic

	recordN(t, e, other.id, KindBadSignature, 2) // 1000 -> 500: watched
	flushNow(t, e)
	e.publishLimits()

	handed := limiter.handed()
	require.Len(t, handed, 1)
	assert.Equal(t, other.id.String(), handed[0].PeerID)
	assert.Equal(t, TierWatched.RateMultiplier(), handed[0].RateMultiplier)
	assert.Equal(t, TierWatched.ConnTag(), handed[0].ConnTag)
	assert.Equal(t, TierWatched.GossipScore(), handed[0].GossipScore)
	assert.True(t, handed[0].InRoutingTable)

	e.publishLimits()
	assert.Len(t, limiter.handed(), 1, "limits that hold are handed on once")

	recordN(t, e, other.id, KindBadSignature, 2) // 500 -> 0: the floor
	flushNow(t, e)
	e.publishLimits()

	handed = limiter.handed()
	require.Len(t, handed, 2, "limits that move are handed on again")
	assert.Equal(t, TierFloor.RateMultiplier(), handed[1].RateMultiplier)
	assert.False(t, handed[1].InRoutingTable, "only the floor leaves the routing table")
}

func TestAPeerNobodyHasObservedIsNeverHandedOn(t *testing.T) {
	self := newIdentity(t)
	ghost := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	limiter := &recordingLimiter{}
	e.Enforce(limiter)

	require.Equal(t, MaxScore, e.Score(ghost.id)) // indexes it, empty
	e.publishLimits()

	assert.Empty(t, limiter.handed(), "nothing has been said about this peer, so there is nothing to hand on")
}

// Evidence decays, so a standing recovers with no event to announce it.
func TestRecoveredLimitsAreHandedOn(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	limiter := &recordingLimiter{}
	e.Enforce(limiter)

	recordN(t, e, other.id, KindBadSignature, 4) // the floor
	flushNow(t, e)
	e.publishLimits()
	require.Len(t, limiter.handed(), 1)

	clock.advance(Network.Retention())
	e.publishLimits()

	handed := limiter.handed()
	require.Len(t, handed, 2)
	assert.Equal(t, TierTrusted.RateMultiplier(), handed[1].RateMultiplier,
		"past retention the peer is trusted again")
	assert.True(t, handed[1].InRoutingTable)
}

func TestAnEngineWithNoLimiterDoesNothing(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	recordN(t, e, other.id, KindBadSignature, 2)
	flushNow(t, e)

	assert.NotPanics(t, func() { e.publishLimits() })

	var nilEngine *Engine
	assert.NotPanics(t, func() { nilEngine.Enforce(&recordingLimiter{}) })
}

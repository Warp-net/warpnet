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
		assert.Less(t, cur.LimitMultiplier(), prev.LimitMultiplier(), "rate limits must tighten as standing falls")
	}
}

func TestOnlyTheFloorIsEvictedFromTheDHT(t *testing.T) {
	for _, tier := range []Tier{TierTrusted, TierWatched, TierDegraded} {
		assert.True(t, tier.AllowedInDHT(), "%s must stay in the routing table", tier)
	}
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

// recordingEnforcer is a module that acts on what the rating concluded.
type recordingEnforcer struct {
	mu        sync.Mutex
	standings []warpnet.PeerStanding
}

func (r *recordingEnforcer) Apply(standing warpnet.PeerStanding) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.standings = append(r.standings, standing)
}

func (r *recordingEnforcer) applied() []warpnet.PeerStanding {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]warpnet.PeerStanding(nil), r.standings...)
}

func TestAStandingIsAnnouncedWhenItMovesAndNotBefore(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	enforcer := &recordingEnforcer{}
	e.Enforce(enforcer, nil) // a module with nothing to apply is not a panic

	recordN(t, e, other.id, KindBadSignature, 2) // 1000 -> 500: watched
	flushNow(t, e)
	e.publishStandings()

	applied := enforcer.applied()
	require.Len(t, applied, 1)
	assert.Equal(t, other.id.String(), applied[0].PeerID)
	assert.Equal(t, TierWatched.LimitMultiplier(), applied[0].LimitMultiplier)
	assert.Equal(t, TierWatched.ConnTag(), applied[0].ConnTag)
	assert.Equal(t, TierWatched.GossipScore(), applied[0].GossipScore)
	assert.True(t, applied[0].AllowedInDHT)

	e.publishStandings()
	assert.Len(t, enforcer.applied(), 1, "a standing that holds is announced once")

	recordN(t, e, other.id, KindBadSignature, 2) // 500 -> 0: the floor
	flushNow(t, e)
	e.publishStandings()

	applied = enforcer.applied()
	require.Len(t, applied, 2, "a standing that moves is announced again")
	assert.Equal(t, TierFloor.LimitMultiplier(), applied[1].LimitMultiplier)
	assert.False(t, applied[1].AllowedInDHT, "only the floor leaves the routing table")
}

func TestAPeerNobodyHasObservedIsNeverAnnounced(t *testing.T) {
	self := newIdentity(t)
	ghost := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	enforcer := &recordingEnforcer{}
	e.Enforce(enforcer)

	require.Equal(t, MaxScore, e.Score(ghost.id)) // indexes it, empty
	e.publishStandings()

	assert.Empty(t, enforcer.applied(), "nothing has been said about this peer, so there is nothing to apply")
}

// Evidence decays, so a standing recovers with no event to announce it.
func TestARecoveredStandingIsAnnounced(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	enforcer := &recordingEnforcer{}
	e.Enforce(enforcer)

	recordN(t, e, other.id, KindBadSignature, 4) // the floor
	flushNow(t, e)
	e.publishStandings()
	require.Len(t, enforcer.applied(), 1)

	clock.advance(Network.Retention())
	e.publishStandings()

	applied := enforcer.applied()
	require.Len(t, applied, 2)
	assert.Equal(t, TierTrusted.LimitMultiplier(), applied[1].LimitMultiplier,
		"past retention the peer is trusted again")
	assert.True(t, applied[1].AllowedInDHT)
}

func TestAnEngineWithNoEnforcerDoesNothing(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	recordN(t, e, other.id, KindBadSignature, 2)
	flushNow(t, e)

	assert.NotPanics(t, func() { e.publishStandings() })

	var nilEngine *Engine
	assert.NotPanics(t, func() { nilEngine.Enforce(&recordingEnforcer{}) })
}

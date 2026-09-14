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

func TestAWorseTierIsWorseOnEveryAxis(t *testing.T) {
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
		assert.True(t, tier.IsAllowedInDHT(), "%s must stay in the routing table", tier)
	}
	assert.False(t, TierFloor.IsAllowedInDHT())
}

func TestATrustedPeerIsNeverHeldBack(t *testing.T) {
	assert.Equal(t, float64(0), TierTrusted.GossipScore())
	assert.Equal(t, float64(1), TierTrusted.RateMultiplier())
	assert.True(t, TierTrusted.IsAllowedInDHT())
}

func TestRateMultiplierNeverReachesZero(t *testing.T) {
	// A low rating slows a peer down; it never refuses it service.
	for _, tier := range []Tier{TierTrusted, TierWatched, TierDegraded, TierFloor} {
		assert.Positive(t, tier.RateMultiplier(), "%s must still be served", tier)
	}
}

// recordingRatings is where the engine records how it rates a peer.
type recordingRatings struct {
	mu    sync.Mutex
	rated []ratedPeer
}

type ratedPeer struct {
	peerID warpnet.WarpPeerID
	tier   Tier
}

func (r *recordingRatings) Rate(peerID warpnet.WarpPeerID, tier Tier) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rated = append(r.rated, ratedPeer{peerID: peerID, tier: tier})
}

func (r *recordingRatings) recorded() []ratedPeer {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ratedPeer(nil), r.rated...)
}

func TestARatingIsRecordedWhenItMovesAndNotBefore(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	ratings := &recordingRatings{}
	e := newRatingEngine(t, self, newFakeStore(self.id), clock, ratings)

	recordN(t, e, other.id, KindBadSignature, 2) // 1000 -> 500: watched
	flushNow(t, e)
	require.NoError(t, e.ratePeers())

	recorded := ratings.recorded()
	require.Len(t, recorded, 1)
	assert.Equal(t, other.id.String(), recorded[0].peerID.String())
	assert.Equal(t, TierWatched.RateMultiplier(), recorded[0].tier.RateMultiplier())
	assert.Equal(t, TierWatched.ConnTag(), recorded[0].tier.ConnTag())
	assert.Equal(t, TierWatched.GossipScore(), recorded[0].tier.GossipScore())
	assert.True(t, recorded[0].tier.IsAllowedInDHT())

	require.NoError(t, e.ratePeers())
	assert.Len(t, ratings.recorded(), 1, "limits that hold are recorded on once")

	recordN(t, e, other.id, KindBadSignature, 2) // 500 -> 0: the floor
	flushNow(t, e)
	require.NoError(t, e.ratePeers())

	recorded = ratings.recorded()
	require.Len(t, recorded, 2, "limits that move are recorded on again")
	assert.Equal(t, TierFloor.RateMultiplier(), recorded[1].tier.RateMultiplier())
	assert.False(t, recorded[1].tier.IsAllowedInDHT(), "only the floor leaves the routing table")
}

func TestAPeerNobodyHasObservedIsNeverRecorded(t *testing.T) {
	self := newIdentity(t)
	ghost := newIdentity(t)
	clock := newClock()
	ratings := &recordingRatings{}
	e := newRatingEngine(t, self, newFakeStore(self.id), clock, ratings)

	require.Equal(t, MaxScore, e.Score(ghost.id)) // indexes it, empty
	require.NoError(t, e.ratePeers())

	assert.Empty(t, ratings.recorded(), "nothing has been said about this peer, so there is nothing to record")
}

// Evidence decays, so a standing recovers with no event to announce it.
func TestARecoveredRatingIsRecorded(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	ratings := &recordingRatings{}
	e := newRatingEngine(t, self, newFakeStore(self.id), clock, ratings)

	recordN(t, e, other.id, KindBadSignature, 4) // the floor
	flushNow(t, e)
	require.NoError(t, e.ratePeers())
	require.Len(t, ratings.recorded(), 1)

	clock.advance(Network.Retention())
	require.NoError(t, e.ratePeers())

	recorded := ratings.recorded()
	require.Len(t, recorded, 2)
	assert.Equal(t, TierTrusted.RateMultiplier(), recorded[1].tier.RateMultiplier(),
		"past retention the peer is trusted again")
	assert.True(t, recorded[1].tier.IsAllowedInDHT())
}

func TestAnEngineWithNowhereToRecordDoesNothing(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock) // nowhere to record

	recordN(t, e, other.id, KindBadSignature, 2)
	flushNow(t, e)

	assert.NoError(t, e.ratePeers(), "a node that enforces nothing still rates its peers")
}

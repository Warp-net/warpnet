// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
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

func TestARatingIsRecordedOnEveryPass(t *testing.T) {
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
	recorded = ratings.recorded()
	require.Len(t, recorded, 2, "limits that hold are recorded again, so they never lapse")
	assert.Equal(t, TierWatched, recorded[1].tier)

	recordN(t, e, other.id, KindBadSignature, 2) // 500 -> 0: the floor
	flushNow(t, e)
	require.NoError(t, e.ratePeers())

	recorded = ratings.recorded()
	require.Len(t, recorded, 3, "limits that move are recorded as they move")
	assert.Equal(t, TierFloor.RateMultiplier(), recorded[2].tier.RateMultiplier())
	assert.False(t, recorded[2].tier.IsAllowedInDHT(), "only the floor leaves the routing table")
}

// A rating that holds is still a rating: the entry the modules read must
// not lapse into trusted while the engine goes on scoring the peer low.
func TestARatingThatHoldsDoesNotLapse(t *testing.T) {
	self := newIdentity(t)
	peer := newIdentity(t)
	clock := newClock()

	const ttl = 100 * time.Millisecond
	ratings := &PeersRatings{tiers: lru.NewLRU[string, Tier](peersRatingsCacheSize, nil, ttl)}
	e := newRatingEngine(t, self, newFakeStore(self.id), clock, ratings)

	recordN(t, e, peer.id, KindBadSignature, 20)
	flushNow(t, e)
	require.NoError(t, e.ratePeers())
	require.Equal(t, TierFloor, ratings.Tier(peer.id))

	time.Sleep(2 * ttl)
	require.NoError(t, e.ratePeers())

	assert.Equal(t, TierFloor, ratings.Tier(peer.id), "the pass records the floor again before it can lapse")
}

// The modules read a bounded cache. A floored peer must not fall out of it
// because many ordinary peers were rated after it.
func TestAFlooredPeerIsNotEvictedByTrustedOnes(t *testing.T) {
	r := NewPeersRatings()
	floored := newIdentity(t).id
	r.Rate(floored, TierFloor)

	for i := range peersRatingsCacheSize {
		r.Rate(warpnet.WarpPeerID(fmt.Sprintf("trusted-%d", i)), TierTrusted)
	}

	assert.Equal(t, TierFloor, r.Tier(floored))
	assert.Equal(t, 1, r.tiers.Len(), "a trusted peer reads as trusted without an entry")
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

// Remote evidence alone may take a peer down to watched. It has to reach
// the modules to do that, even about a peer this node never charged.
func TestRemoteEvidenceAloneReachesTheModules(t *testing.T) {
	self := newIdentity(t)
	peer := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	ratings := NewPeersRatings()
	e := newRatingEngine(t, self, store, clock, ratings)

	for range 3 {
		observer := newIdentity(t)
		store.merge(signedRecord(observer, peer.id, Network, bucketAt(clock.Now()), genA,
			kindCount{KindBadSignature, 4}))
	}
	require.NoError(t, e.ratePeers())

	require.Equal(t, TierWatched, e.Score(peer.id).Tier(), "three acquainted accusers take the peer to watched")
	assert.Equal(t, TierWatched, ratings.Tier(peer.id), "and the modules must see it")
}

// A peer that recovered must be seen to recover, even when a record about
// it is deleted in between.
func TestAPeerRecoversAfterARecordAboutItIsDeleted(t *testing.T) {
	self := newIdentity(t)
	peer := newIdentity(t)
	remote := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	ratings := NewPeersRatings()
	e := newRatingEngine(t, self, store, clock, ratings)

	recordN(t, e, peer.id, KindBadSignature, 5)
	flushNow(t, e)
	require.NoError(t, e.ratePeers())
	require.Equal(t, TierFloor, ratings.Tier(peer.id))

	// Another observer's GC tombstones its own record about the peer.
	e.onDelete(signedRecord(remote, peer.id, Network, bucketAt(clock.Now()), genB, kindCount{KindDialFailure, 1}))

	clock.advance(4 * Network.HalfLife()) // 1250 decays to 78: trusted again
	require.NoError(t, e.ratePeers())

	require.Equal(t, TierTrusted, e.Score(peer.id).Tier(), "the evidence has decayed")
	assert.Equal(t, TierTrusted, ratings.Tier(peer.id), "and the modules must stop holding the peer back")
}

// A restart must not amnesty every offender: the flush loop rates what the
// records already say, without waiting for the peer to offend again.
func TestARestartDoesNotAmnestyTheFloored(t *testing.T) {
	self := newIdentity(t)
	peer := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)

	first := newRatingEngine(t, self, store, clock, NewPeersRatings())
	recordN(t, first, peer.id, KindBadSignature, 5)
	flushNow(t, first)
	require.NoError(t, first.Close())

	ratings := NewPeersRatings()
	restarted := newRatingEngine(t, self, store, clock, ratings)
	require.NoError(t, restarted.ratePeers())

	assert.Equal(t, TierFloor, ratings.Tier(peer.id), "the records on disk still floor the peer")
}

// What the network wrote about this node is replicated to it like any
// other record, and it is still not a rating this node acts on.
func TestANodeNeverRatesItself(t *testing.T) {
	self := newIdentity(t)
	observer := newIdentity(t)
	clock := newClock()
	ratings := &recordingRatings{}
	store := newFakeStore(self.id)
	e := newRatingEngine(t, self, store, clock, ratings)

	store.merge(signedRecord(
		observer, self.id, Network, bucketAt(clock.Now()), genA, kindCount{KindBadSignature, 2},
	))
	require.NoError(t, e.ratePeers())

	assert.Empty(t, ratings.recorded(), "a node holds no rating of itself")
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

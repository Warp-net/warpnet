/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU Affero General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU Affero General Public License for more details.

 You should have received a copy of the GNU Affero General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

 WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
 Use at your own risk. The maintainers shall not be liable for any damages or data loss
 resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// How fast evidence fades is part of what a peer is promised.
func TestHalfLivesAreHalfADayOnTheWireAndAWeekElsewhere(t *testing.T) {
	assert.Equal(t, 12*time.Hour, Network.HalfLife())
	assert.Equal(t, 7*24*time.Hour, Application.HalfLife())
	assert.Equal(t, 7*24*time.Hour, Moderation.HalfLife())
	assert.Equal(t, 4*24*time.Hour, Network.Retention())
	assert.Equal(t, 56*24*time.Hour, Application.Retention())
}

// What each tier costs a peer, knob by knob.
func TestEveryTierCostsWhatItSays(t *testing.T) {
	for _, tc := range []struct {
		tier       Tier
		connTag    int
		gossip     float64
		multiplier float64
		inDHT      bool
	}{
		{TierTrusted, 60, 0, 1, true},
		{TierWatched, 30, -10, 0.5, true},
		{TierDegraded, 10, -60, 0.25, true},
		{TierFloor, 1, -200, 0.1, false},
	} {
		assert.Equal(t, tc.connTag, tc.tier.ConnTag(), "%s", tc.tier)
		assert.Equal(t, tc.gossip, tc.tier.GossipScore(), "%s", tc.tier)
		assert.Equal(t, tc.multiplier, tc.tier.RateMultiplier(), "%s", tc.tier)
		assert.Equal(t, tc.inDHT, tc.tier.IsAllowedInDHT(), "%s", tc.tier)
	}
}

// A forged record is wire behaviour: every node type can witness it, so
// a relay charges it too.
func TestAForgeryIsChargedOnTheWire(t *testing.T) {
	assert.Equal(t, Network, KindForgedRecord.Dimension())
	assert.Equal(t, int32(400), KindForgedRecord.Weight())

	self, liar, peer := newIdentity(t), newIdentity(t), newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	relay := newTestEngine(t, self, store, clock, warpnet.RelayNode)

	store.merge(signedRecord(liar, liar.id, Network, bucketAt(clock.Now()), genA, kindCount{KindBadSignature, 1}))
	flushNow(t, relay)
	assert.Equal(t, MaxScore-400, relay.Score(liar.id), "a relay charges the self-rating it was handed")
	assert.Equal(t, MaxScore, relay.Score(peer.id))
}

// A generation is exactly sixteen random bytes; hex of any other length
// is malformed even though it decodes.
func TestAGenerationOfTheWrongLengthIsMalformed(t *testing.T) {
	self, peer := newIdentity(t), newIdentity(t)
	clock := newClock()
	for _, gen := range []string{"abcd", genA + "00", genA[:30]} {
		r := record{
			PeerID: peer.id.String(), ObserverID: self.id.String(), Dimension: Network.String(),
			Bucket: int64(bucketAt(clock.Now())), Generation: gen,
			Offences: []domain.OffenceCount{{Kind: KindBadSignature.String(), Count: 1}},
		}
		assert.ErrorIs(t, r.validate(clock.Now()), ErrRecordBadGeneration, "generation %q", gen)
	}
}

// Once the settings page has asked what the network says about this node,
// the node holds its own record set in the index; it still never rates
// itself.
func TestANodeThatReadItsOwnRatingStillNeverRatesItself(t *testing.T) {
	self, observer := newIdentity(t), newIdentity(t)
	clock := newClock()
	ratings := &recordingRatings{}
	store := newFakeStore(self.id)
	e := newRatingEngine(t, self, store, clock, ratings)

	store.merge(signedRecord(observer, self.id, Network, bucketAt(clock.Now()), genA, kindCount{KindBadSignature, 2}))
	own, err := e.Own()
	require.NoError(t, err)
	require.EqualValues(t, 500, own.Overall, "the page shows what the observer wrote")

	require.NoError(t, e.ratePeers())
	assert.Empty(t, ratings.recorded(), "a node holds no rating of itself")
}

// One remote observer alone, however loud, never moves a peer out of
// trusted; two reach watched, and no number of them reaches degraded.
func TestRemoteEvidenceMovesAPeerByTiersNotByVolume(t *testing.T) {
	self := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)
	loud := func(n int) entries {
		es := make(entries, 0, n)
		for range n {
			es = append(es, testEntry(newIdentity(t).id.String(), Network, bucketAt(clock.Now()), genA,
				kindCount{KindBadSignature, 100}))
		}
		return es
	}

	assert.Equal(t, TierTrusted, e.score(loud(1), Network, clock.Now()).Tier())
	assert.Equal(t, TierWatched, e.score(loud(2), Network, clock.Now()).Tier())
	assert.Equal(t, TierWatched, e.score(loud(3), Network, clock.Now()).Tier())
	assert.Equal(t, TierWatched, e.score(loud(50), Network, clock.Now()).Tier())
}

// An observer's voice needs an hour of acquaintance, not just a connection.
func TestAnObserverNeedsAnHourOfAcquaintance(t *testing.T) {
	self, observer, victim := newIdentity(t), newIdentity(t), newIdentity(t)
	clock := newClock()
	for _, tc := range []struct {
		connected time.Duration
		counts    bool
	}{
		{59 * time.Minute, false},
		{61 * time.Minute, true},
	} {
		store := newFakeStore(self.id)
		e, err := NewEngine(t.Context(), store, fakeConns{opened: clock.Now().Add(-tc.connected)}, self.priv,
			warpnet.MemberNode, WithClock(clock.Now), WithFlushInterval(time.Hour))
		require.NoError(t, err)
		store.merge(signedRecord(observer, victim.id, Network, bucketAt(clock.Now()), genA,
			kindCount{KindBadSignature, 1}))
		assert.Equal(t, tc.counts, e.Score(victim.id) < MaxScore, "connected for %s", tc.connected)
		require.NoError(t, e.Close())
	}
}

// How often an ordinary event may repeat before it is an offence.
func TestRepetitionThresholds(t *testing.T) {
	for _, tc := range []struct {
		name      string
		event     warpnet.PeerEventType
		route     string
		threshold int
		kind      Kind
	}{
		{"four reconnections", warpnet.PeerConnected, "", 4, KindConnectionFlap},
		{"thirty-two sightings", warpnet.PeerDiscovered, "", 32, KindDiscoveryFlood},
		{"twenty limited writes", warpnet.PeerRateLimited, "/public/post/tweet/0.0.0", 20, KindWriteFlood},
	} {
		self, peer := newIdentity(t), newIdentity(t)
		clock := newClock()
		store := newFakeStore(self.id)
		e := newMemberEngine(t, self, store, clock)

		charged := func() uint32 {
			e.mu.Lock()
			defer e.mu.Unlock()
			return e.counters[pendingKey{peerID: peer.id.String(), dim: tc.kind.Dimension(), bucket: bucketAt(clock.Now())}][tc.kind]
		}
		for i := 1; i <= tc.threshold; i++ {
			require.NoError(t, e.observe(warpnet.PeerEvent{PeerID: peer.id.String(), Type: tc.event, Route: tc.route}))
			want := uint32(0)
			if i == tc.threshold {
				want = 1
			}
			require.Equal(t, want, charged(), "%s: after %d", tc.name, i)
		}
	}
}

// A record merged while the peer's history was still loading is newer
// than the loaded copy of it, and the load must not overwrite it.
func TestALoadDoesNotOverwriteANewerMerge(t *testing.T) {
	p := &indexedPeer{peerID: "peer", slots: make(map[slot][]kindCount)}
	merged := testEntry("observer", Network, 10, genA, kindCount{KindBadSignature, 3})
	loaded := testEntry("observer", Network, 10, genA, kindCount{KindBadSignature, 1})

	p.set(merged.slot(), merged.counts)
	p.fill(loaded)

	es, _ := p.entries()
	assert.Equal(t, []kindCount{{KindBadSignature, 3}}, es[0].counts)
}

// A memoised score is only briefly memoised: evidence decays by the
// second, and a score read a minute later reflects that.
func TestAMemoisedScoreFollowsTheClock(t *testing.T) {
	self, peer := newIdentity(t), newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	recordN(t, e, peer.id, KindBadSignature, 1)
	flushNow(t, e)
	require.Equal(t, Score(750), e.Score(peer.id))

	clock.advance(scoreTTL / 2)
	assert.Equal(t, Score(750), e.Score(peer.id), "within its lifetime the memo is reused")

	clock.advance(scoreTTL)
	assert.Greater(t, e.Score(peer.id), Score(750), "past it the decay shows")
}

// The stored value is what every version of every replica decodes: its
// field names are the wire contract as much as the signed bytes are.
func TestTheStoredRecordKeepsItsWireNames(t *testing.T) {
	rec := domain.RatingRecord{
		PeerID: "peer", ObserverID: "observer", Dimension: "net", Bucket: 490000, Generation: genA,
		Offences:  []domain.OffenceCount{{Kind: "bad_signature", Count: 2}},
		UpdatedAt: time.UnixMilli(1764000000123).UTC(), Signature: "sig",
	}
	wire, err := json.Marshal(rec)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"peer_id":"peer","observer_id":"observer","dimension":"net","bucket":490000,
		"generation":"00112233445566778899aabbccddeeff",
		"offences":[{"kind":"bad_signature","count":2}],
		"updated_at":"2025-11-24T16:00:00.123Z","signature":"sig"
	}`, string(wire))
}

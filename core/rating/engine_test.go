// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func flushNow(t *testing.T, e *Engine) {
	t.Helper()
	require.NoError(t, e.flush())
}

func recordN(t *testing.T, e *Engine, id warpnet.WarpPeerID, k Kind, n int) {
	t.Helper()
	for range n {
		require.NoError(t, e.record(id, k))
	}
}

func scoreOn(t *testing.T, e *Engine, id warpnet.WarpPeerID, dim Dimension) Score {
	t.Helper()
	p, err := e.peer(id.String())
	require.NoError(t, err)
	es, _ := p.entries()
	return e.score(es, dim, e.now())
}

func pendingCount(e *Engine, id warpnet.WarpPeerID, k Kind, b bucket) uint32 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.counters[pendingKey{peerID: id.String(), dim: k.Dimension(), bucket: b}][k]
}

func TestNewEngineValidatesItsDependencies(t *testing.T) {
	self := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)

	_, err := NewEngine(t.Context(), nil, acquainted(clock), self.priv, warpnet.MemberNode)
	assert.ErrorIs(t, err, ErrNilStore)

	_, err = NewEngine(t.Context(), store, nil, self.priv, warpnet.MemberNode)
	assert.ErrorIs(t, err, ErrNilConnections)

	_, err = NewEngine(t.Context(), store, acquainted(clock), nil, warpnet.MemberNode)
	assert.ErrorIs(t, err, ErrPrivateKeyRequired)
}

func TestEngineRefusesToRateItself(t *testing.T) {
	self := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	assert.ErrorIs(t, e.record(self.id, KindBadSignature), ErrSelfRated)
	flushNow(t, e)

	assert.Zero(t, store.len())
	assert.Equal(t, MaxScore, e.Score(self.id))
}

func TestEngineRefusesKindsOutsideItsRole(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	relay := newTestEngine(t, self, store, clock, warpnet.RelayNode)

	assert.ErrorIs(t, relay.record(other.id, KindAuditInvalid), ErrForeignDimension)
	assert.ErrorIs(t, relay.record(other.id, KindWriteFlood), ErrForeignDimension)
	flushNow(t, relay)
	assert.Zero(t, store.len(), "a relay can only witness wire behaviour")

	require.NoError(t, relay.record(other.id, KindBadSignature))
	flushNow(t, relay)
	assert.Equal(t, 1, store.len())
}

func TestRecordFoldsIntoOneRecordPerBucket(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	recordN(t, e, other.id, KindRateLimitHit, 5)
	require.NoError(t, e.record(other.id, KindMalformedFrame))
	flushNow(t, e)

	records := store.records()
	require.Len(t, records, 1, "one peer, one dimension, one bucket, one generation")
	r := record(records[0])
	require.NoError(t, r.verify())
	require.NoError(t, r.validate(clock.Now()))
	assert.Equal(t, other.id.String(), r.PeerID)
	assert.Equal(t, self.id.String(), r.ObserverID)
	assert.Equal(t, Network.String(), r.Dimension)
	assert.Equal(t, int64(bucketAt(clock.Now())), r.Bucket)
	assert.Equal(t, e.generation, r.Generation)
	assert.Equal(t, []domain.OffenceCount{
		{Kind: KindMalformedFrame.String(), Count: 1},
		{Kind: KindRateLimitHit.String(), Count: 5},
	}, r.Offences, "offences are canonical: ascending by kind name")
}

func TestRecordIsNonBlockingWhenPersistenceIsBroken(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	store.setPutErr(errors.New("datastore is down"))
	e := newMemberEngine(t, self, store, clock)

	done := make(chan struct{})
	go func() {
		defer close(done)
		recordN(t, e, other.id, KindRateLimitHit, 100_000)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Record blocked while the datastore was failing")
	}
	assert.EqualValues(t, 100_000, pendingCount(e, other.id, KindRateLimitHit, bucketAt(clock.Now())))
}

func TestFailedWriteStaysDirtyAndRetries(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	store.setPutErr(errors.New("datastore is down"))
	e := newMemberEngine(t, self, store, clock)

	require.NoError(t, e.record(other.id, KindBadSignature))
	require.Error(t, e.flush(), "a failed write must be reported, not swallowed")
	require.Zero(t, store.len())

	store.setPutErr(nil)
	flushNow(t, e)
	assert.Equal(t, 1, store.len(), "a failed write must be retried, not dropped")
}

func TestScoreDropsOnFirstHandEvidence(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	require.Equal(t, MaxScore, e.Score(other.id), "an unseen node starts at full trust")

	recordN(t, e, other.id, KindBadSignature, 2)
	flushNow(t, e)

	assert.Equal(t, Score(500), e.Score(other.id))
	assert.Equal(t, TierWatched, e.Score(other.id).Tier())
}

func TestFirstHandEvidenceReachesTheFloor(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	recordN(t, e, other.id, KindBadSignature, 4)
	flushNow(t, e)

	assert.Equal(t, MinScore, e.Score(other.id))
	assert.Equal(t, TierFloor, e.Score(other.id).Tier())
}

func TestScoreRecoversOverTime(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	recordN(t, e, other.id, KindBadSignature, 2)
	flushNow(t, e)
	damaged := e.Score(other.id)

	clock.advance(Network.HalfLife())
	assert.Greater(t, e.Score(other.id), damaged, "the score must heal as evidence ages")

	clock.advance(Network.Retention())
	assert.Equal(t, MaxScore, e.Score(other.id), "past retention the offence is forgotten")
}

func TestOverallScoreIsWorstDimension(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	e.record(other.id, KindRateLimitHit)              // network, cheap
	recordN(t, e, other.id, KindForeignAuthorship, 2) // application, expensive
	flushNow(t, e)

	assert.Equal(t, scoreOn(t, e, other.id, Application), e.Score(other.id),
		"overall must be the minimum across dimensions")
	assert.Less(t, e.Score(other.id), scoreOn(t, e, other.id, Network))
}

// Remote evidence alone must never push a peer below TierWatched, however
// many observers pile on: reaching TierDegraded takes first-hand evidence.
func TestRemoteEvidenceCannotReachDegraded(t *testing.T) {
	self := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)
	now := clock.Now()

	for _, observers := range []int{1, 3, 50, 200} {
		t.Run(fmt.Sprintf("%d_observers", observers), func(t *testing.T) {
			es := make(entries, 0, observers)
			for range observers {
				es = append(es, testEntry(newIdentity(t).id.String(), Network, bucketAt(now), genA,
					// Everything they can throw, at full trust.
					kindCount{KindBadSignature, 50},
					kindCount{KindPrivateRouteDenied, 50},
					kindCount{KindForgedRecord, 50},
				))
			}
			score := e.score(es, Network, now)
			assert.GreaterOrEqual(t, score, MaxScore-capRemoteTotal)
			assert.LessOrEqual(t, score.Tier(), TierWatched)
		})
	}
}

func TestSingleRemoteObserverIsCappedTighter(t *testing.T) {
	self := newIdentity(t)
	accuser := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	es := entries{testEntry(accuser.id.String(), Network, bucketAt(clock.Now()), genA, kindCount{KindBadSignature, 100})}
	assert.Equal(t, MaxScore-capPerObserver, e.score(es, Network, clock.Now()))
}

func TestDistrustedAccuserIsDiscounted(t *testing.T) {
	self := newIdentity(t)
	accuser := newIdentity(t)
	victim := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	store.merge(signedRecord(accuser, victim.id, Network, bucketAt(clock.Now()), genA, kindCount{KindBadSignature, 1}))
	trusted := e.Score(victim.id)
	require.Less(t, trusted, MaxScore)

	recordN(t, e, accuser.id, KindBadSignature, 2) // our own evidence halves the accuser's weight
	flushNow(t, e)
	e.index.forget(victim.id.String())

	assert.Greater(t, e.Score(victim.id), trusted, "an accuser we distrust must move the score less")
}

func TestOtherDimensionsAreIgnored(t *testing.T) {
	self := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	es := entries{testEntry(self.id.String(), Moderation, bucketAt(clock.Now()), genA, kindCount{KindAuditInvalid, 4})}
	assert.Equal(t, MaxScore, e.score(es, Network, clock.Now()),
		"a moderation offence must not move the network score")
}

func TestStatelessRestartRecovery(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()

	store := newFakeStore(self.id)
	first := newMemberEngine(t, self, store, clock)
	recordN(t, first, other.id, KindBadSignature, 2)
	flushNow(t, first)

	before := first.Score(other.id)
	require.Equal(t, Score(500), before)
	replayed := store.records() // what peers still hold
	require.NoError(t, first.Close())

	empty := newFakeStore(self.id)
	second := newMemberEngine(t, self, empty, clock)
	require.Equal(t, MaxScore, second.Score(other.id), "an empty replica knows nothing yet")

	for _, rec := range replayed {
		empty.merge(rec) // the DAG replays our own previous generation
	}
	assert.Equal(t, before, second.Score(other.id),
		"after replay the restarted node must be back where it was")

	require.NoError(t, second.record(other.id, KindBadSignature))
	flushNow(t, second)
	assert.Equal(t, Score(250), second.Score(other.id))
	assert.Equal(t, 2, empty.len(), "the new generation writes its own key")
}

func TestUnverifiableRecordChargesNobody(t *testing.T) {
	self := newIdentity(t)
	liar := newIdentity(t)
	victim := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	rec := signedRecord(liar, victim.id, Network, bucketAt(clock.Now()), genA, kindCount{KindBadSignature, 1})
	rec.Offences[0].Count = 500 // breaks the signature
	store.merge(rec)
	flushNow(t, e)

	assert.Equal(t, MaxScore, e.Score(victim.id), "a forged accusation must not land")
	assert.Equal(t, MaxScore, e.Score(liar.id),
		"an unverifiable record names an observer that may be innocent")
}

func TestSignedButIllegalRecordChargesItsAuthorOnce(t *testing.T) {
	self := newIdentity(t)
	liar := newIdentity(t)
	victim := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	rec := signedRecord(liar, victim.id, Network, bucketAt(clock.Now()), genA,
		kindCount{KindModerationUpheld, 1}) // an application kind on a network record
	store.merge(rec)
	flushNow(t, e)

	assert.Equal(t, MaxScore, e.Score(victim.id))
	charged := e.Score(liar.id)
	assert.Equal(t, MaxScore-Score(KindForgedRecord.Weight()), charged,
		"the author of a signed illegal record is chargeable")

	// The forgery stays in the store; reloading the victim must not charge again.
	e.index.forget(victim.id.String())
	assert.Equal(t, MaxScore, e.Score(victim.id))
	flushNow(t, e)
	assert.Equal(t, charged, e.Score(liar.id), "one forgery is one charge, however often it is reloaded")
}

func TestLateRecordIsDroppedWithoutBlame(t *testing.T) {
	self := newIdentity(t)
	peer := newIdentity(t)
	victim := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	stale := signedRecord(peer, victim.id, Network,
		bucketAt(clock.Now().Add(-Network.Retention()-2*time.Hour)), genA, kindCount{KindBadSignature, 4})
	store.merge(stale)
	flushNow(t, e)

	assert.Equal(t, MaxScore, e.Score(victim.id), "evidence past retention is ignored")
	assert.Equal(t, MaxScore, e.Score(peer.id), "an honest node that has not pruned yet is not a forger")
}

func TestGCExpiresOnlyOwnRecordsPerDimension(t *testing.T) {
	self := newIdentity(t)
	peer := newIdentity(t)
	rated := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	require.NoError(t, e.record(rated.id, KindBadSignature))
	flushNow(t, e)
	foreign := signedRecord(peer, rated.id, Network, bucketAt(clock.Now()), genB, kindCount{KindBadSignature, 1})
	store.merge(foreign)
	require.Equal(t, 2, store.len())

	clock.advance(Network.Retention() + time.Hour)
	e.gc()

	now := clock.Now()
	assert.ElementsMatch(t, []expiry{
		{dimension: Network.String(), beforeBucket: int64(bucketAt(now.Add(-Network.Retention())))},
		{dimension: Application.String(), beforeBucket: int64(bucketAt(now.Add(-Application.Retention())))},
	}, store.expiries(), "every dimension this node witnesses is pruned at its own retention")

	records := store.records()
	require.Len(t, records, 1, "only our own expired record may go")
	assert.Equal(t, foreign, records[0], "another node's evidence is never ours to prune")
}

func TestEvictedPeerReloadsFromTheStore(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	recordN(t, e, other.id, KindBadSignature, 2)
	flushNow(t, e)
	expected := e.Score(other.id)

	e.index.forget(other.id.String()) // eviction: the index forgets, the store does not
	require.False(t, e.index.has(other.id.String()))

	assert.Equal(t, expected, e.Score(other.id), "an evicted peer must reload from the store")
	assert.True(t, e.index.has(other.id.String()), "and re-enter the index")
}

func TestUnobservedPeerIsNotRequeriedForever(t *testing.T) {
	self := newIdentity(t)
	ghost := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	require.Equal(t, MaxScore, e.Score(ghost.id))
	assert.True(t, e.index.has(ghost.id.String()),
		"a peer nobody observed is indexed empty so it is not re-queried on every request")
}

// The clock is frozen: if invalidation were broken both reads would
// return the memoised value.
func TestNewEvidenceInvalidatesTheCachedScore(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	require.Equal(t, MaxScore, e.Score(other.id))

	recordN(t, e, other.id, KindBadSignature, 2)
	flushNow(t, e)
	first := e.Score(other.id)
	require.Less(t, first, MaxScore, "our own flushed evidence must land at once")

	recordN(t, e, other.id, KindBadSignature, 2)
	flushNow(t, e)
	assert.Less(t, e.Score(other.id), first)
}

func TestMergedForeignRecordInvalidatesTheCachedScore(t *testing.T) {
	self := newIdentity(t)
	peer := newIdentity(t)
	victim := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	require.Equal(t, MaxScore, e.Score(victim.id))

	store.merge(signedRecord(peer, victim.id, Network, bucketAt(clock.Now()), genA, kindCount{KindBadSignature, 2}))

	assert.Less(t, e.Score(victim.id), MaxScore, "a delta merged from the DAG must not read as the cached score")
}

// A merged record for a peer the index does not hold must not create a
// one-record view that shadows the rest of the peer's history in the store.
func TestMergedRecordForUnindexedPeerDoesNotShadowHistory(t *testing.T) {
	self := newIdentity(t)
	peer := newIdentity(t)
	victim := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	// Heavy history on disk only, as after an eviction.
	store.seed(signedRecord(self, victim.id, Network, bucketAt(clock.Now()), genA, kindCount{KindBadSignature, 2}))

	// A benign record arrives over the DAG for the unindexed peer.
	store.merge(signedRecord(peer, victim.id, Network, bucketAt(clock.Now()), genB, kindCount{KindRateLimitHit, 1}))

	// Own history costs 500; the merged remote rate-limit hit costs 15 more.
	// A one-record view would have read 985.
	assert.Equal(t, Score(485), e.Score(victim.id),
		"the score must come from the full stored history, not the last delta")
}

func TestSettledBucketsAreFreed(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	require.NoError(t, e.record(other.id, KindRateLimitHit))
	flushNow(t, e)

	clock.advance(2 * time.Hour)
	require.NoError(t, e.record(other.id, KindRateLimitHit))
	flushNow(t, e)

	e.mu.Lock()
	defer e.mu.Unlock()
	require.Len(t, e.counters, 1, "flushed past buckets must be freed, they can never change again")
	for key := range e.counters {
		assert.Equal(t, bucketAt(clock.Now()), key.bucket)
	}
}

func TestScoreFailsOpenWhenTheStoreCannotBeRead(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	recordN(t, e, other.id, KindBadSignature, 4)
	flushNow(t, e)
	require.Equal(t, TierFloor, e.Score(other.id).Tier(), "with a working store the engine reports the real tier")

	e.index.forget(other.id.String())
	store.setListErr(errors.New("datastore is down"))
	assert.Equal(t, TierTrusted, e.Score(other.id).Tier(), "a standing the engine cannot read must cost the peer nothing")

	store.setListErr(nil)
	assert.Equal(t, TierFloor, e.Score(other.id).Tier(), "a failed load is retried, not remembered as an empty peer")
}

func TestEngineIgnoresUnacquaintedObservers(t *testing.T) {
	self := newIdentity(t)
	stranger := newIdentity(t)
	victim := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e, err := NewEngine(t.Context(), store, fakeConns{}, self.priv, warpnet.MemberNode,
		WithClock(clock.Now), WithFlushInterval(time.Hour))
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Close() })

	store.merge(signedRecord(stranger, victim.id, Network, bucketAt(clock.Now()), genA, kindCount{KindBadSignature, 2}))

	assert.Equal(t, MaxScore, e.Score(victim.id), "an observer we are not connected to has no voice")

	known := newMemberEngine(t, self, store, clock)
	assert.Less(t, known.Score(victim.id), MaxScore, "the same record counts once the observer is known")
}

func TestViewIsThePublicMedianWithRecentTallies(t *testing.T) {
	self := newIdentity(t)
	a := newIdentity(t)
	b := newIdentity(t)
	victim := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	hour := bucketAt(clock.Now())
	store.merge(signedRecord(a, victim.id, Network, hour, genA, kindCount{KindBadSignature, 1})) // 750
	store.merge(signedRecord(b, victim.id, Network, hour, genA, kindCount{KindRateLimitHit, 3})) // 955

	view, err := e.View(victim.id)
	require.NoError(t, err)
	assert.Equal(t, victim.id.String(), view.NodeID)
	assert.Equal(t, 2, view.Observers)
	assert.EqualValues(t, 852, view.Overall, "median of {750, 955}")
	assert.Equal(t, TierTrusted.String(), view.Tier)
	require.Len(t, view.Dimensions, 1)
	assert.Equal(t, Network.String(), view.Dimensions[0].Name)
	assert.Equal(t, []domain.OffenceTally{
		{Kind: KindRateLimitHit.String(), Count: 3, LastAt: hour.start()},
		{Kind: KindBadSignature.String(), Count: 1, LastAt: hour.start()},
	}, view.Dimensions[0].Recent, "raw counts, busiest first")
}

func TestOwnIsWhatOthersWroteAboutThisNode(t *testing.T) {
	self := newIdentity(t)
	peer := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	own, err := e.Own()
	require.NoError(t, err)
	assert.EqualValues(t, MaxScore, own.Overall)
	assert.Zero(t, own.Observers)

	store.merge(signedRecord(peer, self.id, Network, bucketAt(clock.Now()), genA, kindCount{KindBadSignature, 1}))

	own, err = e.Own()
	require.NoError(t, err)
	assert.Equal(t, self.id.String(), own.NodeID)
	assert.EqualValues(t, 750, own.Overall)
	assert.Equal(t, 1, own.Observers)
}

func TestNilEngineIsSafe(t *testing.T) {
	var e *Engine
	id := warpnet.FromStringToPeerID("12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo")
	assert.NotPanics(t, func() {
		require.NoError(t, e.record(id, KindBadSignature))
		assert.Equal(t, MaxScore, e.Score(id))
		_, err := e.View(id)
		assert.NoError(t, err)
		_, err = e.Own()
		assert.NoError(t, err)
		assert.NoError(t, e.Close())
	})
}

func observeN(t *testing.T, e *Engine, id warpnet.WarpPeerID, evType warpnet.PeerEventType, route string, n int) {
	t.Helper()
	for range n {
		require.NoError(t, e.observe(warpnet.PeerEvent{PeerID: id.String(), Type: evType, Route: route}))
	}
}

// Every offence a module can name has to be a kind the engine charges,
// or a module would be reporting into the void.
func TestEveryOffenceEventIsAKind(t *testing.T) {
	facts := map[warpnet.PeerEventType]bool{
		warpnet.PeerConnected:  true,
		warpnet.PeerDiscovered: true,
	}
	for _, evType := range []warpnet.PeerEventType{
		warpnet.PeerBadSignature, warpnet.PeerMissingSignature, warpnet.PeerMalformedFrame,
		warpnet.PeerOversizePayload, warpnet.PeerStaleMessage, warpnet.PeerPrivateRouteDenied,
		warpnet.PeerRateLimited, warpnet.PeerDialFailure, warpnet.PeerConnected, warpnet.PeerDiscovered,
		warpnet.PeerModerationUpheld, warpnet.PeerForeignAuthorship, warpnet.PeerVerdictMalformed,
		warpnet.PeerAuditWrong, warpnet.PeerAuditInvalid, warpnet.PeerAuditUnreachable,
	} {
		_, ok := ParseKind(string(evType))
		if facts[evType] {
			assert.False(t, ok, "%s is a plain fact and must not be an offence on its own", evType)
			continue
		}
		assert.True(t, ok, "%s names no offence the engine knows", evType)
	}
}

func TestObservedOffenceIsCharged(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	require.NoError(t, e.observe(warpnet.PeerEvent{PeerID: other.id.String(), Type: warpnet.PeerBadSignature}))
	flushNow(t, e)

	assert.Equal(t, MaxScore-Score(KindBadSignature.Weight()), e.Score(other.id))
}

func TestUnknownAndSelfObservationsAreRefused(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	// Each of these is refused, and says why, instead of vanishing.
	assert.ErrorIs(t,
		e.observe(warpnet.PeerEvent{PeerID: other.id.String(), Type: "a_type_from_the_future"}),
		ErrUnknownEvent)
	assert.ErrorIs(t,
		e.observe(warpnet.PeerEvent{PeerID: "not-a-peer-id", Type: warpnet.PeerBadSignature}),
		ErrEmptyPeer)
	assert.ErrorIs(t,
		e.observe(warpnet.PeerEvent{PeerID: self.id.String(), Type: warpnet.PeerBadSignature}),
		ErrSelfRated)
	flushNow(t, e)

	assert.Zero(t, store.len(), "nothing here names an offence by a peer")
	assert.Equal(t, MaxScore, e.Score(other.id))
}

// A relay hears about moderation and says nothing: it cannot witness it.
func TestObservationOutsideTheNodesRoleIsDropped(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	relay := newTestEngine(t, self, store, clock, warpnet.RelayNode)

	assert.ErrorIs(t,
		relay.observe(warpnet.PeerEvent{PeerID: other.id.String(), Type: warpnet.PeerModerationUpheld}),
		ErrForeignDimension)
	flushNow(t, relay)
	assert.Zero(t, store.len())

	require.NoError(t, relay.observe(warpnet.PeerEvent{PeerID: other.id.String(), Type: warpnet.PeerBadSignature}))
	flushNow(t, relay)
	assert.Equal(t, 1, store.len())
}

// Connecting, being discovered and being rate-limited are ordinary once
// and an offence in numbers. The threshold lives here, not in the module.
func TestRepetitionBecomesAnOffence(t *testing.T) {
	self := newIdentity(t)
	clock := newClock()

	t.Run("connections", func(t *testing.T) {
		other := newIdentity(t)
		e := newMemberEngine(t, self, newFakeStore(self.id), clock)

		observeN(t, e, other.id, warpnet.PeerConnected, "", flapThreshold-1)
		flushNow(t, e)
		require.Equal(t, MaxScore, e.Score(other.id), "reconnecting is not an offence")

		require.NoError(t, e.observe(warpnet.PeerEvent{PeerID: other.id.String(), Type: warpnet.PeerConnected}))
		flushNow(t, e)
		assert.Equal(t, MaxScore-Score(KindConnectionFlap.Weight()), e.Score(other.id))
	})

	t.Run("sightings", func(t *testing.T) {
		other := newIdentity(t)
		e := newMemberEngine(t, self, newFakeStore(self.id), clock)

		observeN(t, e, other.id, warpnet.PeerDiscovered, "", discoveryThreshold-1)
		flushNow(t, e)
		require.Equal(t, MaxScore, e.Score(other.id), "turning up in discovery is not an offence")

		require.NoError(t, e.observe(warpnet.PeerEvent{PeerID: other.id.String(), Type: warpnet.PeerDiscovered}))
		flushNow(t, e)
		assert.Equal(t, MaxScore-Score(KindDiscoveryFlood.Weight()), e.Score(other.id))
	})

	t.Run("rate limited writes", func(t *testing.T) {
		other := newIdentity(t)
		e := newMemberEngine(t, self, newFakeStore(self.id), clock)

		observeN(t, e, other.id, warpnet.PeerRateLimited, event.PUBLIC_POST_NODE_CHALLENGE, writeFloodThreshold)
		flushNow(t, e)

		// The hits are wire behaviour and the flood is application
		// behaviour, so they land on different axes.
		hits := min(Score(KindRateLimitHit.Weight()*writeFloodThreshold), Score(KindRateLimitHit.Ceiling()))
		assert.Equal(t, MaxScore-hits, scoreOn(t, e, other.id, Network),
			"the hits are charged up to their ceiling")
		assert.Equal(t, MaxScore-Score(KindWriteFlood.Weight()), scoreOn(t, e, other.id, Application),
			"and the flood is charged once, as an application offence")
	})

	t.Run("rate limited reads never flood", func(t *testing.T) {
		other := newIdentity(t)
		e := newMemberEngine(t, self, newFakeStore(self.id), clock)

		observeN(t, e, other.id, warpnet.PeerRateLimited, event.PUBLIC_GET_INFO, writeFloodThreshold+5)
		flushNow(t, e)

		hits := min(Score(KindRateLimitHit.Weight()*(writeFloodThreshold+5)), Score(KindRateLimitHit.Ceiling()))
		assert.Equal(t, MaxScore-hits, e.Score(other.id),
			"reading too often costs the hits and nothing more")
	})
}

func TestWindowReportsOnlyTheCrossing(t *testing.T) {
	b := newWindow(time.Minute, 3)
	assert.False(t, b.reached("peer"))
	assert.False(t, b.reached("peer"))
	assert.True(t, b.reached("peer"), "the third observation crosses the threshold")
	assert.False(t, b.reached("peer"), "and it is reported once, not on every one after")
	assert.False(t, b.reached("another"), "each peer is counted on its own")
}

func TestListenChargesFromEveryFanOutAndStopsWithTheEngine(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	wire := warpnet.NewPeerEmitter()
	handlers := warpnet.NewPeerEmitter()
	e.Listen(wire, nil, handlers) // a module with no fan-out yet is not a panic

	wire.Emit(warpnet.PeerEvent{PeerID: other.id.String(), Type: warpnet.PeerBadSignature})
	handlers.Emit(warpnet.PeerEvent{PeerID: other.id.String(), Type: warpnet.PeerForeignAuthorship})

	require.Eventually(t, func() bool {
		_ = e.flush()
		return e.Score(other.id) < MaxScore-Score(KindBadSignature.Weight())
	}, 5*time.Second, 10*time.Millisecond, "both channels must reach the engine")

	require.NoError(t, e.Close())
	wire.Emit(warpnet.PeerEvent{PeerID: other.id.String(), Type: warpnet.PeerBadSignature})
	after := e.Score(other.id)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, after, e.Score(other.id), "a closed engine stops listening")
}

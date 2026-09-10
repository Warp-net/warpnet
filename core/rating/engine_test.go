// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"errors"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func flushNow(t *testing.T, e *Engine) {
	t.Helper()
	require.NoError(t, e.flush())
}

func recordN(e *Engine, id warpnet.WarpPeerID, k Kind, n int) {
	for range n {
		e.Record(id, k)
	}
}

func scoreDimOf(t *testing.T, e *Engine, id warpnet.WarpPeerID, dim Dimension) Score {
	t.Helper()
	p, err := e.peer(id.String())
	require.NoError(t, err)
	obs, _ := p.entries()
	return localScore(obs, dim, e.self, e.now(), e.weightOf, e.countsTowardScore)
}

func pendingCount(e *Engine, id warpnet.WarpPeerID, k Kind, bucket int64) uint32 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.counters[pendingKey{peerId: id.String(), dim: k.Dimension(), bucket: bucket}][k]
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

	e.Record(self.id, KindBadSignature)
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

	relay.Record(other.id, KindAuditInvalid)
	relay.Record(other.id, KindWriteFlood)
	flushNow(t, relay)
	assert.Zero(t, store.len(), "a relay can only witness wire behaviour")

	relay.Record(other.id, KindBadSignature)
	flushNow(t, relay)
	assert.Equal(t, 1, store.len())
}

func TestRecordFoldsIntoOneRecordPerBucket(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	recordN(e, other.id, KindRateLimitHit, 5)
	e.Record(other.id, KindMalformedFrame)
	flushNow(t, e)

	records := store.records()
	require.Len(t, records, 1, "one peer, one dimension, one bucket, one generation")
	rec := records[0]
	require.NoError(t, verifyRecord(rec))
	require.NoError(t, validateRecord(rec, clock.Now()))
	assert.Equal(t, other.id.String(), rec.PeerId)
	assert.Equal(t, self.id.String(), rec.ObserverId)
	assert.Equal(t, Network.String(), rec.Dimension)
	assert.Equal(t, BucketOf(clock.Now()), rec.Bucket)
	assert.Equal(t, e.generation, rec.Generation)
	assert.Equal(t, []domain.OffenceCount{
		{Kind: KindMalformedFrame.String(), Count: 1},
		{Kind: KindRateLimitHit.String(), Count: 5},
	}, rec.Offences, "offences are canonical: ascending by kind name")
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
		recordN(e, other.id, KindRateLimitHit, 100_000)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Record blocked while the datastore was failing")
	}
	assert.EqualValues(t, 100_000, pendingCount(e, other.id, KindRateLimitHit, BucketOf(clock.Now())))
}

func TestFailedWriteStaysDirtyAndRetries(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	store.setPutErr(errors.New("datastore is down"))
	e := newMemberEngine(t, self, store, clock)

	e.Record(other.id, KindBadSignature)
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

	recordN(e, other.id, KindBadSignature, 2)
	flushNow(t, e)

	assert.Equal(t, Score(500), e.Score(other.id))
	assert.Equal(t, TierWatched, e.Tier(other.id))
}

func TestScoreRecoversOverTime(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	recordN(e, other.id, KindBadSignature, 2)
	flushNow(t, e)
	damaged := e.Score(other.id)

	clock.advance(halfLife[Network])
	assert.Greater(t, e.Score(other.id), damaged, "the score must heal as evidence ages")

	clock.advance(retention(Network))
	assert.Equal(t, MaxScore, e.Score(other.id), "past retention the offence is forgotten")
}

func TestOverallScoreIsWorstDimension(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	e := newMemberEngine(t, self, newFakeStore(self.id), clock)

	e.Record(other.id, KindRateLimitHit)           // network, cheap
	recordN(e, other.id, KindForeignAuthorship, 2) // application, expensive
	flushNow(t, e)

	assert.Equal(t, scoreDimOf(t, e, other.id, Application), e.Score(other.id),
		"overall must be the minimum across dimensions")
	assert.Less(t, e.Score(other.id), scoreDimOf(t, e, other.id, Network))
}

func TestStatelessRestartRecovery(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()

	store := newFakeStore(self.id)
	first := newMemberEngine(t, self, store, clock)
	recordN(first, other.id, KindBadSignature, 2)
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

	second.Record(other.id, KindBadSignature)
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

	rec := signedRecord(liar, victim.id, Network, BucketOf(clock.Now()), genA, kindCount{KindBadSignature, 1})
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

	rec := signedRecord(liar, victim.id, Network, BucketOf(clock.Now()), genA,
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
		BucketOf(clock.Now().Add(-retention(Network)-2*time.Hour)), genA, kindCount{KindBadSignature, 4})
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

	e.Record(rated.id, KindBadSignature)
	flushNow(t, e)
	foreign := signedRecord(peer, rated.id, Network, BucketOf(clock.Now()), genB, kindCount{KindBadSignature, 1})
	store.merge(foreign)
	require.Equal(t, 2, store.len())

	clock.advance(retention(Network) + time.Hour)
	e.gc()

	now := clock.Now()
	assert.ElementsMatch(t, []expiry{
		{dimension: Network.String(), beforeBucket: BucketOf(now.Add(-retention(Network)))},
		{dimension: Application.String(), beforeBucket: BucketOf(now.Add(-retention(Application)))},
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

	recordN(e, other.id, KindBadSignature, 2)
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

	recordN(e, other.id, KindBadSignature, 2)
	flushNow(t, e)
	first := e.Score(other.id)
	require.Less(t, first, MaxScore, "our own flushed evidence must land at once")

	recordN(e, other.id, KindBadSignature, 2)
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

	store.merge(signedRecord(peer, victim.id, Network, BucketOf(clock.Now()), genA, kindCount{KindBadSignature, 2}))

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
	store.seed(signedRecord(self, victim.id, Network, BucketOf(clock.Now()), genA, kindCount{KindBadSignature, 2}))

	// A benign record arrives over the DAG for the unindexed peer.
	store.merge(signedRecord(peer, victim.id, Network, BucketOf(clock.Now()), genB, kindCount{KindRateLimitHit, 1}))

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

	e.Record(other.id, KindRateLimitHit)
	flushNow(t, e)

	clock.advance(2 * time.Hour)
	e.Record(other.id, KindRateLimitHit)
	flushNow(t, e)

	e.mu.Lock()
	defer e.mu.Unlock()
	require.Len(t, e.counters, 1, "flushed past buckets must be freed, they can never change again")
	for key := range e.counters {
		assert.Equal(t, BucketOf(clock.Now()), key.bucket)
	}
}

func TestScoreFailsOpenWhenTheStoreCannotBeRead(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	recordN(e, other.id, KindBadSignature, 4)
	flushNow(t, e)
	require.Equal(t, TierFloor, e.Tier(other.id), "with a working store the engine reports the real tier")

	e.index.forget(other.id.String())
	store.setListErr(errors.New("datastore is down"))
	assert.Equal(t, TierTrusted, e.Tier(other.id), "a standing the engine cannot read must cost the peer nothing")

	store.setListErr(nil)
	assert.Equal(t, TierFloor, e.Tier(other.id), "a failed load is retried, not remembered as an empty peer")
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

	store.merge(signedRecord(stranger, victim.id, Network, BucketOf(clock.Now()), genA, kindCount{KindBadSignature, 2}))

	assert.Equal(t, MaxScore, e.Score(victim.id), "an observer we are not connected to has no voice")

	acquaintedEngine := newMemberEngine(t, self, store, clock)
	assert.Less(t, acquaintedEngine.Score(victim.id), MaxScore, "the same record counts once the observer is known")
}

func TestViewIsThePublicMedianWithRecentTallies(t *testing.T) {
	self := newIdentity(t)
	a := newIdentity(t)
	b := newIdentity(t)
	victim := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e := newMemberEngine(t, self, store, clock)

	bucket := BucketOf(clock.Now())
	store.merge(signedRecord(a, victim.id, Network, bucket, genA, kindCount{KindBadSignature, 1})) // 750
	store.merge(signedRecord(b, victim.id, Network, bucket, genA, kindCount{KindRateLimitHit, 3})) // 955

	view, err := e.View(victim.id)
	require.NoError(t, err)
	assert.Equal(t, victim.id.String(), view.NodeId)
	assert.Equal(t, 2, view.Observers)
	assert.EqualValues(t, 852, view.Overall, "median of {750, 955}")
	assert.Equal(t, TierTrusted.String(), view.Tier)
	require.Len(t, view.Dimensions, 1)
	assert.Equal(t, Network.String(), view.Dimensions[0].Name)
	assert.Equal(t, []domain.OffenceTally{
		{Kind: KindRateLimitHit.String(), Count: 3, LastAt: bucketTime(bucket)},
		{Kind: KindBadSignature.String(), Count: 1, LastAt: bucketTime(bucket)},
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

	store.merge(signedRecord(peer, self.id, Network, BucketOf(clock.Now()), genA, kindCount{KindBadSignature, 1}))

	own, err = e.Own()
	require.NoError(t, err)
	assert.Equal(t, self.id.String(), own.NodeId)
	assert.EqualValues(t, 750, own.Overall)
	assert.Equal(t, 1, own.Observers)
}

func TestNilEngineIsSafe(t *testing.T) {
	var e *Engine
	id := warpnet.FromStringToPeerID("12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo")
	assert.NotPanics(t, func() {
		e.Record(id, KindBadSignature)
		assert.Equal(t, MaxScore, e.Score(id))
		assert.Equal(t, TierTrusted, e.Tier(id))
		_, err := e.View(id)
		assert.NoError(t, err)
		_, err = e.Own()
		assert.NoError(t, err)
		assert.NoError(t, e.Close())
	})
}

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The store reports read and write failures instead of hiding them, so
// the tests assert on them once here and stay readable.
func flushNow(t *testing.T, s *Store) {
	t.Helper()
	require.NoError(t, s.flush())
}

func mustRecord(t *testing.T, s *Store, id warpnet.WarpPeerID, k Kind) {
	t.Helper()
	require.NoError(t, s.Record(id, k))
}

func mustRecordN(t *testing.T, s *Store, id warpnet.WarpPeerID, k Kind, n uint32) {
	t.Helper()
	require.NoError(t, s.RecordN(id, k, n))
}

func scoreOf(t *testing.T, s *Store, id warpnet.WarpPeerID) Score {
	t.Helper()
	score, err := s.Score(id)
	require.NoError(t, err)
	return score
}

func scoreDimOf(t *testing.T, s *Store, id warpnet.WarpPeerID, dim Dimension) Score {
	t.Helper()
	score, err := s.ScoreDim(id, dim)
	require.NoError(t, err)
	return score
}

func bandOf(t *testing.T, s *Store, id warpnet.WarpPeerID) Band {
	t.Helper()
	band, err := s.Band(id)
	require.NoError(t, err)
	return band
}

func newTestStore(t *testing.T, self identity, store Datastore, clock *fixedClock) *Store {
	t.Helper()
	s, err := NewStore(Config{
		Ctx:        t.Context(),
		Self:       self.id,
		PrivKey:    self.priv,
		Dimensions: []Dimension{Network, Application},
		Flush:      time.Hour, // tests drive the flush by hand
		Now:        clock.Now,
	}, opener(store))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func newClock() *fixedClock {
	return &fixedClock{now: time.Now().UTC().Truncate(time.Hour)}
}

func TestStoreRefusesToRateItself(t *testing.T) {
	self := newIdentity(t)
	clock := newClock()
	s := newTestStore(t, self, newMemStore(), clock)

	mustRecord(t, s, self.id, KindBadSignature)
	flushNow(t, s)

	assert.Equal(t, MaxScore, scoreOf(t, s, self.id))
}

func TestStoreRefusesKindsOutsideItsRole(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newMemStore()

	s, err := NewStore(Config{
		Ctx:        t.Context(),
		Self:       self.id,
		PrivKey:    self.priv,
		Dimensions: []Dimension{Network}, // a relay
		Flush:      time.Hour,
		Now:        clock.Now,
	}, opener(store))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	// A relay cannot witness a moderation verdict; the entry is
	// refused at the door rather than written and ignored later, and
	// the caller is told so rather than left guessing.
	err = s.RecordN(other.id, KindAuditInvalid, 1)
	assert.ErrorIs(t, err, ErrForeignDimension)
	flushNow(t, s)
	assert.Equal(t, 0, store.len())
}

func TestObserveFoldsIntoOneKeyPerTuple(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newMemStore()
	s := newTestStore(t, self, store, clock)

	for range 5 {
		mustRecord(t, s, other.id, KindRateLimitHit)
	}
	mustRecord(t, s, other.id, KindMalformedFrame)
	flushNow(t, s)

	keys := store.keys()
	require.Len(t, keys, 1, "one subject, one dimension, one bucket, one generation")

	raw, err := store.Get(t.Context(), ds.NewKey(keys[0]))
	require.NoError(t, err)
	var rec Record
	require.NoError(t, json.Unmarshal(raw, &rec))
	require.NoError(t, rec.Verify())
	assert.Equal(t, other.id.String(), rec.Subject)
	assert.Equal(t, self.id.String(), rec.Observer)
	assert.EqualValues(t, 6, rec.Total())
}

func TestObserveIsNonBlockingWhenPersistenceIsBroken(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newMemStore()
	store.putErr = errors.New("datastore is down")
	s := newTestStore(t, self, store, clock)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100_000 {
			_ = s.Record(other.id, KindRateLimitHit)
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Record blocked while the datastore was failing")
	}
	// Nothing is lost either: folding is in-memory, so a broken
	// datastore costs persistence, never entries.
	assert.EqualValues(t, 100_000, s.counters[pendingKey{
		subject: other.id.String(), dim: Network, bucket: BucketOf(clock.Now()),
	}][KindRateLimitHit])
}

func TestFailedWriteStaysDirtyAndRetries(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newMemStore()
	store.putErr = errors.New("datastore is down")
	s := newTestStore(t, self, store, clock)

	mustRecord(t, s, other.id, KindBadSignature)
	require.Error(t, s.flush(), "a failed write must be reported, not swallowed")
	require.Equal(t, 0, store.len())

	store.mu.Lock()
	store.putErr = nil
	store.mu.Unlock()

	flushNow(t, s)
	assert.Equal(t, 1, store.len(), "a failed write must be retried, not dropped")
}

func TestScoreDropsOnFirstHandEvidence(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	s := newTestStore(t, self, newMemStore(), clock)

	require.Equal(t, MaxScore, scoreOf(t, s, other.id), "an unseen node starts at full trust")

	mustRecordN(t, s, other.id, KindBadSignature, 2)
	flushNow(t, s)

	assert.Equal(t, Score(500), scoreOf(t, s, other.id))
	assert.Equal(t, BandWatched, bandOf(t, s, other.id))
}

func TestScoreRecoversOverTime(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	s := newTestStore(t, self, newMemStore(), clock)

	mustRecordN(t, s, other.id, KindBadSignature, 2)
	flushNow(t, s)
	damaged := scoreOf(t, s, other.id)

	clock.advance(halfLife[Network])
	halfway := scoreOf(t, s, other.id)
	assert.Greater(t, halfway, damaged, "the score must heal as evidence ages")

	clock.advance(retention(Network))
	assert.Equal(t, MaxScore, scoreOf(t, s, other.id), "past retention the offence is forgotten")
}

func TestOverallScoreIsWorstDimension(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	s := newTestStore(t, self, newMemStore(), clock)

	s.Record(other.id, KindRateLimitHit)          // network, cheap
	s.RecordN(other.id, KindForeignAuthorship, 2) // application, expensive
	flushNow(t, s)

	assert.Equal(t, scoreDimOf(t, s, other.id, Application), scoreOf(t, s, other.id),
		"overall must be the minimum across dimensions")
	assert.Less(t, scoreOf(t, s, other.id), scoreDimOf(t, s, other.id, Network))
}

// TestStatelessRestartRecovery is the reason the rating rides a CRDT
// at all. A relay holds no disk: when it dies its whole view goes with
// it, and the only way back is the DAG replaying its own past records.
// The generation segment is what keeps the fresh process from writing
// over that history as it arrives.
func TestStatelessRestartRecovery(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()

	store := newMemStore()
	first := newTestStore(t, self, store, clock)
	mustRecordN(t, first, other.id, KindBadSignature, 2)
	flushNow(t, first)

	before := scoreOf(t, first, other.id)
	require.Equal(t, Score(500), before)
	replayed := store.clone() // what peers still hold
	require.NoError(t, first.Close())

	// The process comes back with nothing of its own, then the DAG
	// hands its old records back.
	empty := newMemStore()
	second := newTestStore(t, self, empty, clock)
	require.Equal(t, MaxScore, scoreOf(t, second, other.id), "an empty replica knows nothing yet")

	for _, key := range replayed.keys() {
		value, err := replayed.Get(t.Context(), ds.NewKey(key))
		require.NoError(t, err)
		require.NoError(t, empty.Put(t.Context(), ds.NewKey(key), value))
		second.onPut(key, value) // the CRDT put hook
	}

	assert.Equal(t, before, scoreOf(t, second, other.id),
		"after replay the restarted node must be back where it was")

	// New entries from the fresh generation must add to the
	// replayed ones, not replace them.
	mustRecordN(t, second, other.id, KindBadSignature, 1)
	flushNow(t, second)
	assert.Equal(t, Score(250), scoreOf(t, second, other.id))
	assert.Equal(t, 2, empty.len(), "the new generation writes its own key")
}

func TestForgedRecordIsDroppedAndCharged(t *testing.T) {
	self := newIdentity(t)
	liar := newIdentity(t)
	victim := newIdentity(t)
	clock := newClock()
	s := newTestStore(t, self, newMemStore(), clock)

	t.Run("unverifiable record charges nobody", func(t *testing.T) {
		rec := signedRecord(liar, victim.id, Network, BucketOf(clock.Now()), genA,
			CountEntry{KindBadSignature, 1})
		rec.Counts[0].Count = 500 // breaks the signature
		payload, err := json.Marshal(rec)
		require.NoError(t, err)

		s.onPut(rec.Key(), payload)
		flushNow(t, s)

		assert.Equal(t, MaxScore, scoreOf(t, s, victim.id), "a forged accusation must not land")
		assert.Equal(t, MaxScore, scoreOf(t, s, liar.id),
			"an unverifiable record names an observer that may be innocent")
	})

	t.Run("signed but illegal record charges its author", func(t *testing.T) {
		// Correctly signed, and provably illegal: an application kind
		// carried on a network record.
		rec := Record{
			Subject:    victim.id.String(),
			Observer:   liar.id.String(),
			Dim:        Network,
			Bucket:     BucketOf(clock.Now()),
			Generation: genA,
			Counts:     []CountEntry{{KindModerationUpheld, 1}},
			UpdatedAt:  clock.Now(),
		}
		require.NoError(t, rec.Sign(liar.priv))
		payload, err := json.Marshal(rec)
		require.NoError(t, err)

		s.onPut(rec.Key(), payload)
		flushNow(t, s)

		assert.Equal(t, MaxScore, scoreOf(t, s, victim.id))
		assert.Less(t, scoreOf(t, s, liar.id), MaxScore, "the author of a signed illegal record is chargeable")
	})
}

func TestGCRemovesOnlyOwnExpiredRecords(t *testing.T) {
	self := newIdentity(t)
	peer := newIdentity(t)
	subject := newIdentity(t)
	clock := newClock()
	store := newMemStore()
	s := newTestStore(t, self, store, clock)

	mustRecord(t, s, subject.id, KindBadSignature)
	flushNow(t, s)

	// A foreign record in the same expired window.
	foreign := signedRecord(peer, subject.id, Network, BucketOf(clock.Now()), genB,
		CountEntry{KindBadSignature, 1})
	payload, err := json.Marshal(foreign)
	require.NoError(t, err)
	require.NoError(t, store.Put(t.Context(), ds.NewKey(foreign.Key()), payload))
	require.Equal(t, 2, store.len())

	clock.advance(retention(Network) + time.Hour)
	require.NoError(t, s.gcOwnExpired())

	keys := store.keys()
	require.Len(t, keys, 1, "only our own expired record may be deleted")
	assert.Contains(t, keys[0], peer.id.String(),
		"a CRDT delete is a propagating tombstone: never prune another node's evidence")
}

func TestEvictedSubjectFallsBackToQuery(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newMemStore()
	s := newTestStore(t, self, store, clock)

	mustRecordN(t, s, other.id, KindBadSignature, 2)
	flushNow(t, s)
	expected := scoreOf(t, s, other.id)

	// Simulate LRU eviction: the index forgets, the datastore does not.
	s.idx.mu.Lock()
	delete(s.idx.data, other.id.String())
	s.idx.mu.Unlock()
	s.idx.lru.Remove(other.id.String())
	require.False(t, s.idx.has(other.id.String()))

	assert.Equal(t, expected, scoreOf(t, s, other.id), "an evicted subject must reload from the datastore")
	assert.True(t, s.idx.has(other.id.String()), "and re-enter the index")
}

func TestUnobservedSubjectIsNotRequeriedForever(t *testing.T) {
	self := newIdentity(t)
	ghost := newIdentity(t)
	clock := newClock()
	s := newTestStore(t, self, newMemStore(), clock)

	require.Equal(t, MaxScore, scoreOf(t, s, ghost.id))
	assert.True(t, s.idx.has(ghost.id.String()),
		"a peer nobody observed is marked present so it is not re-queried on every request")
}

func TestNopRaterNeverPenalises(t *testing.T) {
	var r Rater = Nop{}
	id := warpnet.FromStringToPeerID("12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo")
	require.NoError(t, r.Record(id, KindBadSignature))

	score, err := r.Score(id)
	require.NoError(t, err)
	assert.Equal(t, MaxScore, score)

	band, err := r.Band(id)
	require.NoError(t, err)
	assert.Equal(t, BandTrusted, band)
}

func TestNilStoreIsSafe(t *testing.T) {
	var s *Store
	id := warpnet.FromStringToPeerID("12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo")
	assert.NotPanics(t, func() {
		assert.NoError(t, s.Record(id, KindBadSignature))
		assert.Equal(t, MaxScore, scoreOf(t, s, id))
		assert.Equal(t, BandTrusted, bandOf(t, s, id))
		_, err := s.Own()
		assert.NoError(t, err)
		assert.NoError(t, s.Close())
	})
}

var _ = context.Background

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

func newTestStore(t *testing.T, self identity, store Datastore, clock *fixedClock) *Store {
	t.Helper()
	s, err := NewStore(Config{
		Ctx:        t.Context(),
		Self:       self.id,
		PrivKey:    self.priv,
		Dimensions: []Dimension{Network, Application},
		Mode:       ModeEnforce,
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

	s.Observe(self.id, KindBadSignature)
	s.flush()

	assert.Equal(t, MaxScore, s.Score(self.id))
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
		Mode:       ModeEnforce,
		Flush:      time.Hour,
		Now:        clock.Now,
	}, opener(store))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	// A relay cannot witness a moderation verdict; the observation is
	// refused at the door rather than written and ignored later.
	s.ObserveN(other.id, KindAuditInvalid, 1)
	s.flush()
	assert.Equal(t, 0, store.len())
}

func TestObserveFoldsIntoOneKeyPerTuple(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	store := newMemStore()
	s := newTestStore(t, self, store, clock)

	for range 5 {
		s.Observe(other.id, KindRateLimitHit)
	}
	s.Observe(other.id, KindMalformedFrame)
	s.flush()

	keys := store.keys()
	require.Len(t, keys, 1, "one subject, one dimension, one bucket, one generation")

	raw, err := store.Get(t.Context(), ds.NewKey(keys[0]))
	require.NoError(t, err)
	var rec ObservationRecord
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
			s.Observe(other.id, KindRateLimitHit)
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Observe blocked while the datastore was failing")
	}
	// Nothing is lost either: folding is in-memory, so a broken
	// datastore costs persistence, never observations.
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

	s.Observe(other.id, KindBadSignature)
	s.flush()
	require.Equal(t, 0, store.len())

	store.mu.Lock()
	store.putErr = nil
	store.mu.Unlock()

	s.flush()
	assert.Equal(t, 1, store.len(), "a failed write must be retried, not dropped")
}

func TestScoreDropsOnFirstHandEvidence(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	s := newTestStore(t, self, newMemStore(), clock)

	require.Equal(t, MaxScore, s.Score(other.id), "an unseen node starts at full trust")

	s.ObserveN(other.id, KindBadSignature, 2)
	s.flush()

	assert.Equal(t, Score(500), s.Score(other.id))
	assert.Equal(t, BandWatched, s.Band(other.id))
}

func TestScoreRecoversOverTime(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	s := newTestStore(t, self, newMemStore(), clock)

	s.ObserveN(other.id, KindBadSignature, 2)
	s.flush()
	damaged := s.Score(other.id)

	clock.advance(halfLife[Network])
	halfway := s.Score(other.id)
	assert.Greater(t, halfway, damaged, "the score must heal as evidence ages")

	clock.advance(retention(Network))
	assert.Equal(t, MaxScore, s.Score(other.id), "past retention the offence is forgotten")
}

func TestOverallScoreIsWorstDimension(t *testing.T) {
	self := newIdentity(t)
	other := newIdentity(t)
	clock := newClock()
	s := newTestStore(t, self, newMemStore(), clock)

	s.Observe(other.id, KindRateLimitHit)          // network, cheap
	s.ObserveN(other.id, KindForeignAuthorship, 2) // application, expensive
	s.flush()

	assert.Equal(t, s.ScoreDim(other.id, Application), s.Score(other.id),
		"overall must be the minimum across dimensions")
	assert.Less(t, s.Score(other.id), s.ScoreDim(other.id, Network))
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
	first.ObserveN(other.id, KindBadSignature, 2)
	first.flush()

	before := first.Score(other.id)
	require.Equal(t, Score(500), before)
	replayed := store.clone() // what peers still hold
	require.NoError(t, first.Close())

	// The process comes back with nothing of its own, then the DAG
	// hands its old records back.
	empty := newMemStore()
	second := newTestStore(t, self, empty, clock)
	require.Equal(t, MaxScore, second.Score(other.id), "an empty replica knows nothing yet")

	for _, key := range replayed.keys() {
		value, err := replayed.Get(t.Context(), ds.NewKey(key))
		require.NoError(t, err)
		require.NoError(t, empty.Put(t.Context(), ds.NewKey(key), value))
		second.onPut(key, value) // the CRDT put hook
	}

	assert.Equal(t, before, second.Score(other.id),
		"after replay the restarted node must be back where it was")

	// New observations from the fresh generation must add to the
	// replayed ones, not replace them.
	second.ObserveN(other.id, KindBadSignature, 1)
	second.flush()
	assert.Equal(t, Score(250), second.Score(other.id))
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
		s.flush()

		assert.Equal(t, MaxScore, s.Score(victim.id), "a forged accusation must not land")
		assert.Equal(t, MaxScore, s.Score(liar.id),
			"an unverifiable record names an observer that may be innocent")
	})

	t.Run("signed but illegal record charges its author", func(t *testing.T) {
		// Correctly signed, and provably illegal: an application kind
		// carried on a network record.
		rec := ObservationRecord{
			Subject:    victim.id.String(),
			Observer:   liar.id.String(),
			Dim:        Network,
			Bucket:     BucketOf(clock.Now()),
			Generation: genA,
			Counts:     []CountEntry{{KindModerationUpheld, 1}},
			UpdatedAt:  clock.Now(),
		}
		rec.Sign(liar.priv)
		payload, err := json.Marshal(rec)
		require.NoError(t, err)

		s.onPut(rec.Key(), payload)
		s.flush()

		assert.Equal(t, MaxScore, s.Score(victim.id))
		assert.Less(t, s.Score(liar.id), MaxScore, "the author of a signed illegal record is chargeable")
	})
}

func TestGCRemovesOnlyOwnExpiredRecords(t *testing.T) {
	self := newIdentity(t)
	peer := newIdentity(t)
	subject := newIdentity(t)
	clock := newClock()
	store := newMemStore()
	s := newTestStore(t, self, store, clock)

	s.Observe(subject.id, KindBadSignature)
	s.flush()

	// A foreign record in the same expired window.
	foreign := signedRecord(peer, subject.id, Network, BucketOf(clock.Now()), genB,
		CountEntry{KindBadSignature, 1})
	payload, err := json.Marshal(foreign)
	require.NoError(t, err)
	require.NoError(t, store.Put(t.Context(), ds.NewKey(foreign.Key()), payload))
	require.Equal(t, 2, store.len())

	clock.advance(retention(Network) + time.Hour)
	s.gcOwnExpired()

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

	s.ObserveN(other.id, KindBadSignature, 2)
	s.flush()
	expected := s.Score(other.id)

	// Simulate LRU eviction: the index forgets, the datastore does not.
	s.idx.mu.Lock()
	delete(s.idx.data, other.id.String())
	s.idx.mu.Unlock()
	s.idx.lru.Remove(other.id.String())
	require.False(t, s.idx.has(other.id.String()))

	assert.Equal(t, expected, s.Score(other.id), "an evicted subject must reload from the datastore")
	assert.True(t, s.idx.has(other.id.String()), "and re-enter the index")
}

func TestUnobservedSubjectIsNotRequeriedForever(t *testing.T) {
	self := newIdentity(t)
	ghost := newIdentity(t)
	clock := newClock()
	s := newTestStore(t, self, newMemStore(), clock)

	require.Equal(t, MaxScore, s.Score(ghost.id))
	assert.True(t, s.idx.has(ghost.id.String()),
		"a peer nobody observed is marked present so it is not re-queried on every request")
}

func TestNopRaterNeverPenalises(t *testing.T) {
	var r Rater = Nop{}
	id := warpnet.FromStringToPeerID("12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo")
	r.Observe(id, KindBadSignature)
	assert.Equal(t, MaxScore, r.Score(id))
	assert.Equal(t, BandTrusted, r.Band(id))
}

func TestNilStoreIsSafe(t *testing.T) {
	var s *Store
	id := warpnet.FromStringToPeerID("12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo")
	assert.NotPanics(t, func() {
		s.Observe(id, KindBadSignature)
		assert.Equal(t, MaxScore, s.Score(id))
		assert.Equal(t, BandTrusted, s.Band(id))
		assert.Equal(t, ModeShadow, s.Mode())
		_ = s.Own()
		_ = s.Close()
	})
}

var _ = context.Background

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package crdt

import (
	"context"
	"sync"
	"testing"
	"time"

	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	datastore "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLiveRatingStore(t *testing.T) (*CRDTRatingStore, *silentBroadcaster) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	bc := &silentBroadcaster{}
	store, err := NewCRDTRatingStore(
		ctx,
		bc,
		dssync.MutexWrap(datastore.NewMapDatastore()),
		newStatsHost(t),
		noProviderRouter{},
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	return store, bc
}

func ratingRecord(observer, peerId, dimension string, bucket int64) domain.RatingRecord {
	return domain.RatingRecord{
		PeerId:     peerId,
		ObserverId: observer,
		Dimension:  dimension,
		Bucket:     bucket,
		Generation: "00112233445566778899aabbccddeeff",
		Offences:   []domain.OffenceCount{{Kind: "bad_signature", Count: 1}},
		UpdatedAt:  time.Unix(bucket*3600, 0).UTC(),
		Signature:  "not-checked-here",
	}
}

// putReplicated writes a record under its own key the way the DAG would
// deliver a foreign node's record: bypassing the own-records guard of Put.
func putReplicated(t *testing.T, s *CRDTRatingStore, rec domain.RatingRecord) {
	t.Helper()
	key, err := recordKey(rec)
	require.NoError(t, err)
	value, err := json.Marshal(rec)
	require.NoError(t, err)
	require.NoError(t, s.crdt.Put(context.Background(), key, value))
}

type recordLog struct {
	mu   sync.Mutex
	recs []domain.RatingRecord
}

func (l *recordLog) hook(rec domain.RatingRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.recs = append(l.recs, rec)
}

func (l *recordLog) snapshot() []domain.RatingRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]domain.RatingRecord(nil), l.recs...)
}

func TestCRDTRating_PutThenListRoundTrip(t *testing.T) {
	store, _ := newLiveRatingStore(t)

	aNet := ratingRecord(store.nodeID, "peer-a", "net", 10)
	aApp := ratingRecord(store.nodeID, "peer-a", "app", 10)
	bNet := ratingRecord(store.nodeID, "peer-b", "net", 10)
	for _, rec := range []domain.RatingRecord{aNet, aApp, bNet} {
		require.NoError(t, store.Put(rec))
	}

	got, err := store.List("peer-a")
	require.NoError(t, err)
	assert.ElementsMatch(t, []domain.RatingRecord{aNet, aApp}, got, "peer-b's record must not leak into peer-a's list")

	got, err = store.List("peer-b")
	require.NoError(t, err)
	assert.Equal(t, []domain.RatingRecord{bNet}, got)

	got, err = store.List("peer-nobody-observed")
	require.NoError(t, err)
	assert.Empty(t, got, "an unobserved peer is an empty list, not an error")
}

func TestCRDTRating_PutOverwritesTheSameKey(t *testing.T) {
	store, _ := newLiveRatingStore(t)

	rec := ratingRecord(store.nodeID, "peer-a", "net", 10)
	require.NoError(t, store.Put(rec))
	rec.Offences[0].Count = 7
	require.NoError(t, store.Put(rec))

	got, err := store.List("peer-a")
	require.NoError(t, err)
	require.Len(t, got, 1, "one (peer, observer, dimension, bucket, generation) tuple is one key")
	assert.EqualValues(t, 7, got[0].Offences[0].Count)
}

func TestCRDTRating_PutRefusesForeignRecords(t *testing.T) {
	store, bc := newLiveRatingStore(t)

	err := store.Put(ratingRecord("some-other-node", "peer-a", "net", 10))
	assert.ErrorIs(t, err, ErrForeignRatingRecord)

	got, err := store.List("peer-a")
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Zero(t, bc.count(), "a refused write must not reach the network")
}

func TestCRDTRating_PutRefusesMalformedKeyParts(t *testing.T) {
	store, _ := newLiveRatingStore(t)

	noDimension := ratingRecord(store.nodeID, "peer-a", "", 10)
	assert.ErrorIs(t, store.Put(noDimension), ErrMalformedRatingRecord)

	slashed := ratingRecord(store.nodeID, "peer-a", "net", 10)
	slashed.Generation = "gen/with/slashes"
	assert.ErrorIs(t, store.Put(slashed), ErrMalformedRatingRecord)

	_, err := store.List("")
	assert.ErrorIs(t, err, ErrMalformedRatingRecord)
}

func TestCRDTRating_HooksSeeLocalPutsAndDeletes(t *testing.T) {
	store, _ := newLiveRatingStore(t)

	puts, deletes := &recordLog{}, &recordLog{}
	store.OnPut(puts.hook)
	store.OnDelete(deletes.hook)

	rec := ratingRecord(store.nodeID, "peer-a", "net", 10)
	require.NoError(t, store.Put(rec))

	assert.Eventually(t, func() bool { return len(puts.snapshot()) == 1 }, time.Second, 10*time.Millisecond)
	assert.Equal(t, rec, puts.snapshot()[0], "the put hook carries the whole decoded record")

	require.NoError(t, store.DeleteExpired("net", 11))

	assert.Eventually(t, func() bool { return len(deletes.snapshot()) == 1 }, time.Second, 10*time.Millisecond)
	want := domain.RatingRecord{
		PeerId: rec.PeerId, ObserverId: rec.ObserverId, Dimension: rec.Dimension,
		Bucket: rec.Bucket, Generation: rec.Generation,
	}
	assert.Equal(t, want, deletes.snapshot()[0], "the delete hook carries the key fields only")
}

func TestCRDTRating_HooksSeeReplicatedRecords(t *testing.T) {
	store, _ := newLiveRatingStore(t)

	puts := &recordLog{}
	store.OnPut(puts.hook)

	foreign := ratingRecord("some-other-node", "peer-a", "net", 10)
	putReplicated(t, store, foreign)

	assert.Eventually(t, func() bool { return len(puts.snapshot()) == 1 }, time.Second, 10*time.Millisecond)
	assert.Equal(t, foreign, puts.snapshot()[0])

	got, err := store.List("peer-a")
	require.NoError(t, err)
	assert.Equal(t, []domain.RatingRecord{foreign}, got, "foreign records are readable, just not writable")
}

func TestCRDTRating_DeleteExpiredRemovesOnlyOwnOlderRecordsOfThatDimension(t *testing.T) {
	store, _ := newLiveRatingStore(t)

	oldNet := ratingRecord(store.nodeID, "peer-a", "net", 10)
	newNet := ratingRecord(store.nodeID, "peer-a", "net", 20)
	oldApp := ratingRecord(store.nodeID, "peer-a", "app", 10)
	for _, rec := range []domain.RatingRecord{oldNet, newNet, oldApp} {
		require.NoError(t, store.Put(rec))
	}
	foreignOldNet := ratingRecord("some-other-node", "peer-a", "net", 10)
	putReplicated(t, store, foreignOldNet)

	require.NoError(t, store.DeleteExpired("net", 15))

	got, err := store.List("peer-a")
	require.NoError(t, err)
	assert.ElementsMatch(t, []domain.RatingRecord{newNet, oldApp, foreignOldNet}, got,
		"only our own network record from before the cutoff may go; another node's evidence is never ours to prune")
}

func TestCRDTRating_ListDropsRecordsStoredUnderAForeignKey(t *testing.T) {
	store, _ := newLiveRatingStore(t)

	aboutA := ratingRecord("some-other-node", "peer-a", "net", 10)
	underB := ratingRecord("some-other-node", "peer-b", "net", 10)
	key, err := recordKey(underB)
	require.NoError(t, err)
	value, err := json.Marshal(aboutA)
	require.NoError(t, err)
	require.NoError(t, store.crdt.Put(context.Background(), key, value))

	got, err := store.List("peer-b")
	require.NoError(t, err)
	assert.Empty(t, got, "a record about peer-a filed under peer-b's key is not evidence about peer-b")

	got, err = store.List("peer-a")
	require.NoError(t, err)
	assert.Empty(t, got, "and it never entered peer-a's history either")
}

func TestCRDTRating_KeyRoundTrip(t *testing.T) {
	rec := ratingRecord("observer", "peer", "mod", 4711)
	key, err := recordKey(rec)
	require.NoError(t, err)
	assert.Equal(t, "/RATING/record/peer/observer/mod/4711/"+rec.Generation, key.String())

	parsed, ok := parseRecordKey(key.String())
	require.True(t, ok)
	assert.Equal(t, domain.RatingRecord{
		PeerId: "peer", ObserverId: "observer", Dimension: "mod", Bucket: 4711, Generation: rec.Generation,
	}, parsed)

	for _, foreign := range []string{
		"/STATS/incr/whatever/node/gen",
		"/RATING/record/too/few/parts",
		"/RATING/record/peer/observer/net/not-a-number/gen",
		"/RATING/record/peer//net/1/gen",
	} {
		_, ok := parseRecordKey(foreign)
		assert.False(t, ok, "key %q must not parse", foreign)
	}
}

func TestCRDTRating_EveryWriteIsBroadcast(t *testing.T) {
	store, bc := newLiveRatingStore(t)

	require.NoError(t, store.Put(ratingRecord(store.nodeID, "peer-a", "net", 10)))
	require.NoError(t, store.Put(ratingRecord(store.nodeID, "peer-b", "net", 10)))

	assert.Positive(t, bc.count(), "local-only records would never reach the peers that must weigh them")
}

func TestCRDTRating_CloseIsSafeOnNil(t *testing.T) {
	assert.NoError(t, (*CRDTRatingStore)(nil).Close())

	store, _ := newLiveRatingStore(t)
	assert.NoError(t, store.Close())
}

var _ = ds.NewKey

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package rating

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	datastore "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/ipfs/go-ds-crdt/pb"
	"github.com/libp2p/go-libp2p"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type silentBroadcaster struct {
	mu        sync.Mutex
	published [][]byte
}

func (b *silentBroadcaster) Broadcast(_ context.Context, data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, append([]byte(nil), data...))
	return nil
}

func (b *silentBroadcaster) Next(ctx context.Context) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *silentBroadcaster) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.published)
}

type idleRouter struct{}

func (idleRouter) FindProvidersAsync(context.Context, warpnet.WarpCID, int) <-chan warpnet.WarpAddrInfo {
	ch := make(chan warpnet.WarpAddrInfo)
	close(ch)
	return ch
}

func newHost(t *testing.T) warpnet.P2PNode {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func newLiveStore(t *testing.T) (*Store, *silentBroadcaster) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	bc := &silentBroadcaster{}
	store, err := New(
		ctx,
		bc,
		dssync.MutexWrap(datastore.NewMapDatastore()),
		newHost(t),
		idleRouter{},
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	return store, bc
}

func ratingRecord(observer, peerID, dimension string, bucket int64) domain.RatingRecord {
	return domain.RatingRecord{
		PeerID:     peerID,
		ObserverID: observer,
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
func putReplicated(t *testing.T, s *Store, rec domain.RatingRecord) {
	t.Helper()
	key, err := recordKey(rec)
	require.NoError(t, err)
	value, err := json.Marshal(rec)
	require.NoError(t, err)
	publishInEpoch(t, s, key.String(), value, epochDAGName(rec.Dimension, rec.Bucket))
}

func publishInEpoch(t *testing.T, s *Store, key string, value []byte, dagName string) {
	t.Helper()
	delta, err := s.crdt.Set().Add(context.Background(), key, value)
	require.NoError(t, err)
	delta.SetDagName(dagName)
	_, err = s.crdt.Publish(context.Background(), delta)
	require.NoError(t, err)
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

func TestStore_PutThenListRoundTrip(t *testing.T) {
	store, _ := newLiveStore(t)

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

func TestStore_PutOverwritesTheSameKey(t *testing.T) {
	store, _ := newLiveStore(t)

	rec := ratingRecord(store.nodeID, "peer-a", "net", 10)
	require.NoError(t, store.Put(rec))
	rec.Offences[0].Count = 7
	require.NoError(t, store.Put(rec))

	got, err := store.List("peer-a")
	require.NoError(t, err)
	require.Len(t, got, 1, "one (peer, observer, dimension, bucket, generation) tuple is one key")
	assert.EqualValues(t, 7, got[0].Offences[0].Count)
}

func TestStore_PutWritesOneBlockForManyRecords(t *testing.T) {
	store, bc := newLiveStore(t)
	ctx := context.Background()

	recs := make([]domain.RatingRecord, 0, 50)
	for i := range 50 {
		recs = append(recs, ratingRecord(store.nodeID, fmt.Sprintf("peer-%d", i), "net", 10))
	}
	require.NoError(t, store.Put(recs...))
	assert.EqualValues(t, 1, store.crdt.InternalStats(ctx).MaxHeight, "the whole write is one DAG block")
	assert.Equal(t, 1, bc.count())

	ids, err := store.PeerIDs()
	require.NoError(t, err)
	assert.Len(t, ids, 50)

	require.NoError(t, store.DeleteExpired("net", 24))
	assert.Empty(t, store.crdt.InternalStats(ctx).Heads, "the expired day goes as a whole")
	ids, err = store.PeerIDs()
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestStore_PutSplitsBlocksAtMaxDeltaSize(t *testing.T) {
	store, _ := newLiveStore(t)

	recs := make([]domain.RatingRecord, 0, 10)
	for i := range 10 {
		rec := ratingRecord(store.nodeID, fmt.Sprintf("peer-%d", i), "net", 10)
		rec.Signature = strings.Repeat("s", 100<<10)
		recs = append(recs, rec)
	}
	require.NoError(t, store.Put(recs...))

	height := store.crdt.InternalStats(context.Background()).MaxHeight
	assert.EqualValues(t, 5, height, "1 MiB of records is five blocks of at most 256 KiB")
	ids, err := store.PeerIDs()
	require.NoError(t, err)
	assert.Len(t, ids, 10)
}

func TestStore_PutRefusesTheWholeCallOnOneBadRecord(t *testing.T) {
	store, bc := newLiveStore(t)

	good := ratingRecord(store.nodeID, "peer-a", "net", 10)
	foreign := ratingRecord("some-other-node", "peer-b", "net", 10)
	assert.ErrorIs(t, store.Put(good, foreign), ErrForeignRecord)
	assert.Zero(t, bc.count(), "nothing of a refused call reaches the network")
}

func TestStore_PutRefusesForeignRecords(t *testing.T) {
	store, bc := newLiveStore(t)

	err := store.Put(ratingRecord("some-other-node", "peer-a", "net", 10))
	assert.ErrorIs(t, err, ErrForeignRecord)

	got, err := store.List("peer-a")
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Zero(t, bc.count(), "a refused write must not reach the network")
}

func TestStore_PutRefusesMalformedKeyParts(t *testing.T) {
	store, _ := newLiveStore(t)

	noDimension := ratingRecord(store.nodeID, "peer-a", "", 10)
	assert.ErrorIs(t, store.Put(noDimension), ErrMalformedRecord)

	slashed := ratingRecord(store.nodeID, "peer-a", "net", 10)
	slashed.Generation = "gen/with/slashes"
	assert.ErrorIs(t, store.Put(slashed), ErrMalformedRecord)

	_, err := store.List("")
	assert.ErrorIs(t, err, ErrMalformedRecord)
}

func TestStore_HooksSeeLocalPutsAndDeletes(t *testing.T) {
	store, _ := newLiveStore(t)

	puts, deletes := &recordLog{}, &recordLog{}
	store.OnPut(puts.hook)
	store.OnDelete(deletes.hook)

	rec := ratingRecord(store.nodeID, "peer-a", "net", 10)
	require.NoError(t, store.Put(rec))

	assert.Eventually(t, func() bool { return len(puts.snapshot()) == 1 }, time.Second, 10*time.Millisecond)
	assert.Equal(t, rec, puts.snapshot()[0], "the put hook carries the whole decoded record")

	require.NoError(t, store.DeleteExpired("net", 24))

	assert.Eventually(t, func() bool { return len(deletes.snapshot()) == 1 }, time.Second, 10*time.Millisecond)
	want := domain.RatingRecord{
		PeerID: rec.PeerID, ObserverID: rec.ObserverID, Dimension: rec.Dimension,
		Bucket: rec.Bucket, Generation: rec.Generation,
	}
	assert.Equal(t, want, deletes.snapshot()[0], "the delete hook carries the key fields only")
}

func TestStore_HooksSeeReplicatedRecords(t *testing.T) {
	store, _ := newLiveStore(t)

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

func TestStore_DeleteExpiredDropsWholeDaysOfThatDimension(t *testing.T) {
	store, _ := newLiveStore(t)

	oldNet := ratingRecord(store.nodeID, "peer-a", "net", 10)
	lateNet := ratingRecord(store.nodeID, "peer-a", "net", 23)
	newNet := ratingRecord(store.nodeID, "peer-a", "net", 24)
	oldApp := ratingRecord(store.nodeID, "peer-a", "app", 10)
	require.NoError(t, store.Put(oldNet, lateNet, newNet, oldApp))
	foreignOldNet := ratingRecord("some-other-node", "peer-a", "net", 10)
	putReplicated(t, store, foreignOldNet)

	require.NoError(t, store.DeleteExpired("net", 23))
	got, err := store.List("peer-a")
	require.NoError(t, err)
	assert.Len(t, got, 5, "a day goes only once all of its buckets are past the cutoff")

	require.NoError(t, store.DeleteExpired("net", 24))
	got, err = store.List("peer-a")
	require.NoError(t, err)
	assert.ElementsMatch(t, []domain.RatingRecord{newNet, oldApp}, got,
		"the expired network day goes whole, foreign evidence included; other days and dimensions stay")
}

func TestStore_BroadcastsCarryOnlyLiveDays(t *testing.T) {
	store, _ := newLiveStore(t)
	require.NoError(t, store.DeleteExpired("net", 48))

	encode := func(names ...string) []byte {
		msg := &pb.CRDTBroadcast{}
		for i, name := range names {
			msg.Heads = append(msg.Heads, &pb.Head{Cid: []byte{byte(i)}, DagName: name})
		}
		data, err := proto.Marshal(msg)
		require.NoError(t, err)
		return data
	}
	names := func(data []byte) []string {
		var msg pb.CRDTBroadcast
		require.NoError(t, proto.Unmarshal(data, &msg))
		var out []string
		for _, h := range msg.Heads {
			out = append(out, h.GetDagName())
		}
		return out
	}

	live, ok := store.liveHeads(encode("", "net-1", "net-2", "app-1", "bogus", "net-x"))
	require.True(t, ok)
	assert.Equal(t, []string{"net-2", "app-1"}, names(live),
		"the legacy DAG, expired days and malformed names never travel")

	_, ok = store.liveHeads(encode("", "net-0"))
	assert.False(t, ok, "a broadcast left without heads is dropped")
}

func TestStore_StartupPurgesTheLegacyDAG(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	data := dssync.MutexWrap(datastore.NewMapDatastore())

	first, err := New(ctx, &silentBroadcaster{}, data, newHost(t), idleRouter{})
	require.NoError(t, err)
	legacy := ratingRecord(first.nodeID, "peer-a", "net", currentBucket())
	key, err := recordKey(legacy)
	require.NoError(t, err)
	value, err := json.Marshal(legacy)
	require.NoError(t, err)
	require.NoError(t, first.crdt.Put(ctx, key, value))
	require.NoError(t, first.Close())

	second, err := New(ctx, &silentBroadcaster{}, data, newHost(t), idleRouter{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })
	require.Eventually(t, func() bool {
		got, err := second.List("peer-a")
		return err == nil && len(got) == 0 && len(second.crdt.InternalStats(ctx).Heads) == 0
	}, 5*time.Second, 10*time.Millisecond, "the history from before epochs is dropped on start")
}

func TestStore_ListDropsRecordsStoredUnderAForeignKey(t *testing.T) {
	store, _ := newLiveStore(t)

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

func TestStore_KeyRoundTrip(t *testing.T) {
	rec := ratingRecord("observer", "peer", "mod", 4711)
	key, err := recordKey(rec)
	require.NoError(t, err)
	assert.Equal(t, "/record/peer/observer/mod/4711/"+rec.Generation, key.String())

	parsed, ok := parseRecordKey(key.String())
	require.True(t, ok)
	assert.Equal(t, domain.RatingRecord{
		PeerID: "peer", ObserverID: "observer", Dimension: "mod", Bucket: 4711, Generation: rec.Generation,
	}, parsed)

	for _, foreign := range []string{
		"/STATS/incr/whatever/node/gen",
		"/record/too/few/parts",
		"/record/peer/observer/net/not-a-number/gen",
		"/record/peer//net/1/gen",
	} {
		_, ok := parseRecordKey(foreign)
		assert.False(t, ok, "key %q must not parse", foreign)
	}
}

func TestStore_EveryWriteIsBroadcast(t *testing.T) {
	store, bc := newLiveStore(t)

	require.NoError(t, store.Put(ratingRecord(store.nodeID, "peer-a", "net", 10)))
	require.NoError(t, store.Put(ratingRecord(store.nodeID, "peer-b", "net", 10)))

	assert.Positive(t, bc.count(), "local-only records would never reach the peers that must weigh them")
}

func TestStore_CloseIsSafeOnNil(t *testing.T) {
	assert.NoError(t, (*Store)(nil).Close())

	store, _ := newLiveStore(t)
	assert.NoError(t, store.Close())
}

func TestStore_PeerIDsListsEveryPeerOnRecordOnce(t *testing.T) {
	store, _ := newLiveStore(t)

	for _, rec := range []domain.RatingRecord{
		ratingRecord(store.nodeID, "peer-a", "net", 10),
		ratingRecord(store.nodeID, "peer-a", "app", 10),
		ratingRecord(store.nodeID, "peer-b", "net", 11),
	} {
		require.NoError(t, store.Put(rec))
	}
	putReplicated(t, store, ratingRecord("observer-c", "peer-c", "net", 10))

	got, err := store.PeerIDs()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"peer-a", "peer-b", "peer-c"}, got, "own and foreign records alike, each peer once")
}

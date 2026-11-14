// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	dag "github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/go-cid"
	datastore "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/ipfs/go-ds-crdt/pb"
	"github.com/libp2p/go-libp2p"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type noRouter struct{}

func (noRouter) FindProvidersAsync(context.Context, warpnet.WarpCID, int) <-chan warpnet.WarpAddrInfo {
	ch := make(chan warpnet.WarpAddrInfo)
	close(ch)
	return ch
}

func newTestCRDT(t *testing.T) *MerkleCRDT {
	t.Helper()
	return newHookedCRDT(t, nil)
}

func newHookedCRDT(t *testing.T, putHook func(datastore.Key, []byte)) *MerkleCRDT {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	host, err := libp2p.New(libp2p.NoListenAddrs)
	require.NoError(t, err)
	t.Cleanup(func() { _ = host.Close() })

	m, err := New(ctx, Config{
		Broadcaster: idleBroadcaster{},
		Datastore:   dssync.MutexWrap(datastore.NewMapDatastore()),
		Node:        host,
		Router:      noRouter{},
		Prefix:      "/test",
		PutHook:     putHook,
		Validate:    func(string, string, []byte) error { return nil },
		Compare: func(value, current []byte) int {
			return cmp.Compare(binary.BigEndian.Uint64(value), binary.BigEndian.Uint64(current))
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func total(n uint64) []byte { return binary.BigEndian.AppendUint64(nil, n) }

// received reads a block as go-ds-crdt would: through the store's delta.
func received(t *testing.T, m *MerkleCRDT, value, priority uint64) *checkedDelta {
	t.Helper()
	data, err := proto.Marshal(&pb.Delta{
		Priority: priority,
		DagName:  DayAt(time.Now()).DAG("test"),
		Elements: []*pb.Element{{Key: "/k", Value: total(value)}},
	})
	require.NoError(t, err)
	d := &checkedDelta{Delta: &pb.Delta{}, crdt: m}
	require.NoError(t, d.Unmarshal(data))
	return d
}

func merge(t *testing.T, m *MerkleCRDT, d *checkedDelta, block string) {
	t.Helper()
	require.NoError(t, m.Set().Merge(context.Background(), d, block))
}

func chosen(t *testing.T, m *MerkleCRDT) []byte {
	t.Helper()
	value, err := m.Get(context.Background(), datastore.NewKey("/k"))
	require.NoError(t, err)
	return value
}

func TestVersionedStore_AReplayDecodedBeforeANewerValueCannotWinAfterIt(t *testing.T) {
	m := newTestCRDT(t)
	merge(t, m, received(t, m, 1, 1), "first")

	replay := received(t, m, 1, 100)
	newer := received(t, m, 2, 2)
	merge(t, m, newer, "newer")
	merge(t, m, replay, "replay")

	assert.Equal(t, total(2), chosen(t, m), "the version is decided where the set writes it, not when a block is decoded")
}

func TestVersionedStore_AnUnsignedPriorityCannotLockTheKey(t *testing.T) {
	m := newTestCRDT(t)
	merge(t, m, received(t, m, 1, 1), "first")
	merge(t, m, received(t, m, 1, math.MaxUint64), "replay")
	merge(t, m, received(t, m, 2, 2), "newer")

	assert.Equal(t, total(2), chosen(t, m), "a block's priority is not signed and does not rank its writes")
}

func TestVersionedStore_AnOlderValueIsNeverChosen(t *testing.T) {
	m := newTestCRDT(t)
	merge(t, m, received(t, m, 5, 1), "first")
	merge(t, m, received(t, m, 3, 1000), "older")

	assert.Equal(t, total(5), chosen(t, m))
}

func TestVersionedStore_ARefusedValueReachesNoHook(t *testing.T) {
	var hooked [][]byte
	m := newHookedCRDT(t, func(_ datastore.Key, v []byte) { hooked = append(hooked, v) })
	merge(t, m, received(t, m, 5, 1), "first")
	merge(t, m, received(t, m, 3, 1000), "older")

	assert.Equal(t, [][]byte{total(5)}, hooked, "a value the store did not keep is not reported as merged")
}

func TestLineage_ABlockOfUnknownRootsMayClaimAnyDAG(t *testing.T) {
	l := newLineage(nil)
	assert.True(t, l.isRootedIn([]byte("never fetched"), "net-1"), "a block reached without an announcement, as the startup repair does, is not refused")
}

func TestLineage_ABlockMustBeFiledUnderTheDAGItWasReachedFrom(t *testing.T) {
	l := newLineage(nil)
	child := dag.NodeWithData([]byte("child delta"))
	head := dag.NodeWithData([]byte("head delta"))
	require.NoError(t, head.AddNodeLink("", child))

	l.announce(head.Cid(), "net-2")
	l.fetched(head)
	l.fetched(child)

	assert.True(t, l.isRootedIn(head.Data(), "net-2"))
	assert.False(t, l.isRootedIn(head.Data(), "net-1"), "the head was announced under another day")
	assert.True(t, l.isRootedIn(child.Data(), "net-2"), "a child inherits its head's DAG")
	assert.False(t, l.isRootedIn(child.Data(), "net-1"))
}

func TestLineage_ABlockReachedFromTwoHeadsMayClaimEither(t *testing.T) {
	l := newLineage(nil)
	shared := dag.NodeWithData([]byte("shared"))
	l.announce(shared.Cid(), "net-1")
	l.announce(shared.Cid(), "net-2")
	l.announce(shared.Cid(), "net-2")
	l.fetched(shared)

	assert.True(t, l.isRootedIn(shared.Data(), "net-1"))
	assert.True(t, l.isRootedIn(shared.Data(), "net-2"))
	names, _ := l.byCID.Get(shared.Cid())
	assert.Len(t, names, 2, "announcing a root twice records it once")
}

func TestLineage_EveryFetchPathIsFollowed(t *testing.T) {
	m := newTestCRDT(t)
	ctx := context.Background()
	nodes := make([]*dag.ProtoNode, 3)
	for i := range nodes {
		nodes[i] = dag.NodeWithData([]byte("fetched " + strconv.Itoa(i)))
		require.NoError(t, m.lineage.Add(ctx, nodes[i]))
		m.lineage.announce(nodes[i].Cid(), "net-9")
	}

	_, err := m.lineage.Get(ctx, nodes[0].Cid())
	require.NoError(t, err)
	for opt := range m.lineage.GetMany(ctx, []cid.Cid{nodes[1].Cid()}) {
		require.NoError(t, opt.Err)
	}
	_, err = m.lineage.Session(ctx).Get(ctx, nodes[2].Cid())
	require.NoError(t, err)

	for i, nd := range nodes {
		assert.False(t, m.lineage.isRootedIn(nd.Data(), "net-1"), "path %d did not record the block's root", i)
	}
}

func TestCheckedDelta_ABlockUnderAnotherRootIsRefused(t *testing.T) {
	m := newTestCRDT(t)
	data, err := proto.Marshal(&pb.Delta{Priority: 1, DagName: "test-1", Elements: []*pb.Element{{Key: "/k", Value: total(1)}}})
	require.NoError(t, err)
	block := dag.NodeWithData(data)
	m.lineage.announce(block.Cid(), "test-2")
	m.lineage.fetched(block)

	d := &checkedDelta{Delta: &pb.Delta{}, crdt: m}
	assert.ErrorIs(t, d.Unmarshal(data), ErrForeignRoot)
}

func TestCheckedDelta_KeepsOnlyValidWritesAndNoTombstones(t *testing.T) {
	m := newTestCRDT(t)
	m.validate = func(_, key string, _ []byte) error {
		if key == "/bad" {
			return errors.New("refused")
		}
		return nil
	}
	data, err := proto.Marshal(&pb.Delta{
		Priority:   1,
		DagName:    "test-1",
		Elements:   []*pb.Element{{Key: "/good", Value: total(1)}, {Key: "/bad", Value: total(1)}},
		Tombstones: []*pb.Element{{Key: "/good", Id: "any"}},
	})
	require.NoError(t, err)

	d := &checkedDelta{Delta: &pb.Delta{}, crdt: m}
	require.NoError(t, d.Unmarshal(data))
	require.Len(t, d.Elements, 1)
	assert.Equal(t, "/good", d.Elements[0].GetKey())
	assert.Empty(t, d.Tombstones, "these stores never delete, so a received tombstone is never honest")
	assert.False(t, d.IsEmpty())

	round, err := d.Marshal()
	require.NoError(t, err)
	again := &checkedDelta{Delta: &pb.Delta{}, crdt: m}
	require.NoError(t, again.Unmarshal(round))
	assert.Len(t, again.Elements, 1, "a checked delta survives its own round trip")
}

func TestCheckedDelta_AGarbageBlockIsAnError(t *testing.T) {
	m := newTestCRDT(t)
	d := &checkedDelta{Delta: &pb.Delta{}, crdt: m}
	assert.Error(t, d.Unmarshal([]byte{0xff, 0xff, 0xff}))
}

func TestVersionedStore_TheSameValueAndANewerOneAreKept(t *testing.T) {
	m := newTestCRDT(t)
	merge(t, m, received(t, m, 5, 1), "first")
	merge(t, m, received(t, m, 5, 7), "again")
	assert.Equal(t, total(5), chosen(t, m))
	merge(t, m, received(t, m, 6, 1), "newer at a lower priority")
	assert.Equal(t, total(6), chosen(t, m), "a newer version wins whatever priority its block claims")
}

func TestVersionedStore_KeysOutsideTheSetPassThrough(t *testing.T) {
	inner := dssync.MutexWrap(datastore.NewMapDatastore())
	v := versionedStore{Datastore: inner, compare: func([]byte, []byte) int { return -1 }}
	ctx := context.Background()

	other := datastore.NewKey("/h/somehead/p")
	require.NoError(t, v.Put(ctx, other, []byte("x")))
	require.NoError(t, v.Put(ctx, other, []byte("y")))
	got, err := v.Get(ctx, other)
	require.NoError(t, err)
	assert.Equal(t, []byte("y"), got, "only the set's value keys are versioned")

	prio := datastore.NewKey(setValuePrefix + "/k" + setPrioSuffix)
	require.NoError(t, inner.Put(ctx, prio, binary.AppendUvarint(nil, math.MaxUint64)))
	got, err = v.Get(ctx, prio)
	require.NoError(t, err)
	assert.Equal(t, unrankedPriority, got, "a stored priority always reads as the lowest")

	value := setValueKey("/k")
	require.NoError(t, v.Put(ctx, value, total(2)))
	require.NoError(t, v.Put(ctx, value, total(9)))
	got, err = v.Get(ctx, value)
	require.NoError(t, err)
	assert.Equal(t, total(2), got, "a value compare puts below the current one is not written")
}

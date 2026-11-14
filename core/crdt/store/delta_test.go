// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"cmp"
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
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

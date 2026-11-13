// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"cmp"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	datastore "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	crdt "github.com/ipfs/go-ds-crdt"
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
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	host, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = host.Close() })

	m, err := New(ctx, Config{
		Broadcaster: idleBroadcaster{},
		Datastore:   dssync.MutexWrap(datastore.NewMapDatastore()),
		Node:        host,
		Router:      noRouter{},
		Prefix:      "/test",
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

func TestCheckedDelta_TheSetNamespaceIsFound(t *testing.T) {
	m := newTestCRDT(t)
	ns := m.setNamespace.Load()
	require.NotNil(t, ns, "reflection must find go-ds-crdt's set namespace")
	assert.Equal(t, "/s", ns.String())
}

func TestCheckedDelta_ANewerValueOutranksTheCurrentOneAndAnOlderIsDropped(t *testing.T) {
	m := newTestCRDT(t)
	ctx := context.Background()
	dagName := DayAt(time.Now()).DAG("test")

	for i, key := range []string{"/a", "/b", "/k"} {
		delta, err := m.Set().Add(ctx, key, total(uint64(5+i)))
		require.NoError(t, err)
		delta.SetDagName(dagName)
		require.NoError(t, m.Publish(ctx, delta))
	}
	current, currentPriority, ok := m.current("/k")
	require.True(t, ok)
	require.Equal(t, total(7), current)

	received := func(value uint64) *checkedDelta {
		data, err := proto.Marshal(&pb.Delta{
			Priority: 1,
			DagName:  dagName,
			Elements: []*pb.Element{{Key: "/k", Value: total(value)}},
		})
		require.NoError(t, err)
		d := &checkedDelta{Delta: &pb.Delta{}, crdt: m}
		require.NoError(t, d.Unmarshal(data))
		return d
	}

	newer := received(9)
	assert.Len(t, newer.Elements, 1)
	assert.Equal(t, currentPriority+1, newer.Priority, "a newer value is raised above the replay that holds the key")

	older := received(3)
	assert.Empty(t, older.Elements, "an older value never takes part in choosing the key's value")
}

func TestCheckedDelta_ALayoutSurpriseIsRecovered(t *testing.T) {
	m := &MerkleCRDT{MerkleCRDT: &crdt.MerkleCRDT{}}
	var (
		ok  bool
		err any
	)
	func() {
		defer func() { err = recover() }()
		_, ok = m.readSetNamespace()
	}()
	assert.Nil(t, err, "a reflection failure must not escape")
	assert.False(t, ok, "and must turn the stale-version check off")
}

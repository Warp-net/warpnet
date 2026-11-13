// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"testing"
	"time"

	"github.com/ipfs/go-ds-crdt/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func encodeHeads(t *testing.T, names ...string) []byte {
	t.Helper()
	msg := &pb.CRDTBroadcast{}
	for i, name := range names {
		msg.Heads = append(msg.Heads, &pb.Head{Cid: []byte{byte(i)}, DagName: name})
	}
	data, err := proto.Marshal(msg)
	require.NoError(t, err)
	return data
}

func headNames(t *testing.T, data []byte) []string {
	t.Helper()
	var msg pb.CRDTBroadcast
	require.NoError(t, proto.Unmarshal(data, &msg))
	out := make([]string, 0, len(msg.GetHeads()))
	for _, h := range msg.GetHeads() {
		out = append(out, h.GetDagName())
	}
	return out
}

func TestEpochs_BroadcastsCarryOnlyLiveDays(t *testing.T) {
	e := NewEpochs()
	e.Expire("net", 2)

	live, ok := e.liveHeads(encodeHeads(t, "", "net-1", "net-2", "app-1", "bogus", "net-x"))
	require.True(t, ok)
	assert.Equal(t, []string{"net-2", "app-1"}, headNames(t, live),
		"the legacy DAG, expired days and malformed names never travel")

	_, ok = e.liveHeads(encodeHeads(t, "", "net-0"))
	assert.False(t, ok, "a broadcast left without heads is dropped")
}

func TestEpochs_RefuseDaysFromTheFuture(t *testing.T) {
	e := NewEpochs()
	today := DayOf(time.Now())

	assert.True(t, e.IsLive(DayDAG("stats", today+1)), "one day of clock skew is tolerated")
	assert.False(t, e.IsLive(DayDAG("stats", today+2)), "a day ahead of the clock would never expire")
}

func TestEpochs_KnownPrefixesOnly(t *testing.T) {
	e := NewEpochs("stats")
	today := DayOf(time.Now())

	assert.True(t, e.IsLive(DayDAG("stats", today)))
	assert.False(t, e.IsLive(DayDAG("net", today)), "another store's DAG is not this store's")
}

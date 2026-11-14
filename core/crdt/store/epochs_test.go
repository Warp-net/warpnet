// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"strconv"
	"testing"
	"time"

	datastore "github.com/ipfs/go-datastore"
	crdt "github.com/ipfs/go-ds-crdt"
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

func newEpochs(prefixes ...string) *MerkleCRDT {
	m := &MerkleCRDT{firstLive: make(map[string]Day), lineage: newLineage(nil)}
	if len(prefixes) > 0 {
		m.prefixes = make(map[string]struct{}, len(prefixes))
		for _, p := range prefixes {
			m.prefixes[p] = struct{}{}
		}
	}
	return m
}

func TestEpochs_BroadcastsCarryOnlyLiveDays(t *testing.T) {
	e := newEpochs()
	e.Expire("net", 2)

	live, ok := e.liveHeads(encodeHeads(t, "", "net-1", "net-2", "app-1", "bogus", "net-x"))
	require.True(t, ok)
	assert.Equal(t, []string{"net-2", "app-1"}, headNames(t, live),
		"the legacy DAG, expired days and malformed names never travel")

	_, ok = e.liveHeads(encodeHeads(t, "", "net-0"))
	assert.False(t, ok, "a broadcast left without heads is dropped")
}

func TestEpochs_RefuseDaysFromTheFuture(t *testing.T) {
	e := newEpochs()
	today := DayAt(time.Now())

	assert.True(t, e.IsLive((today + 1).DAG("stats")), "one day of clock skew is tolerated")
	assert.False(t, e.IsLive((today + 2).DAG("stats")), "a day ahead of the clock would never expire")
}

func TestEpochs_AllowedPrefixesOnly(t *testing.T) {
	e := newEpochs("stats")
	today := DayAt(time.Now())

	assert.True(t, e.IsLive(today.DAG("stats")))
	assert.False(t, e.IsLive(today.DAG("net")), "another store's DAG is not this store's")
}

func TestEpochs_DayAtIsTheUTCDayNumber(t *testing.T) {
	midnight := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	day := DayAt(midnight)

	assert.Equal(t, day, DayAt(midnight.Add(23*time.Hour+59*time.Minute)), "the whole UTC day is one day")
	assert.Equal(t, day-1, DayAt(midnight.Add(-time.Second)), "a second before midnight is the day before")
	assert.Equal(t, day, DayAt(midnight.In(time.FixedZone("UTC+4", 4*3600))), "the zone of the clock does not matter")
	assert.Equal(t, "stats-20770", Day(20770).DAG("stats"))
}

func TestEpochs_MalformedDAGNamesAreNeverLive(t *testing.T) {
	e := newEpochs()
	for _, name := range []string{"", "stats", "-1", "stats-", "stats-x", "stats-1.5", "stats-+1"} {
		assert.False(t, e.IsLive(name), "%q", name)
	}
	assert.True(t, e.IsLive("net-app-1"), "a prefix may contain dashes; the day is after the last one")
}

func TestEpochs_ExpireKeepsTheFirstLiveDay(t *testing.T) {
	e := newEpochs()
	today := DayAt(time.Now())

	assert.True(t, e.IsLive((today - 100).DAG("net")), "a prefix with no cutoff yet keeps every day")
	e.Expire("net", today-4)
	assert.True(t, e.IsLive((today - 4).DAG("net")), "the first live day is live")
	assert.False(t, e.IsLive((today - 5).DAG("net")), "the day before it is not")
	assert.True(t, e.IsLive((today - 100).DAG("app")), "a cutoff applies to its own prefix only")

	e.Expire("net", today-10)
	assert.True(t, e.IsLive((today - 5).DAG("net")), "a later cutoff replaces the earlier one")
}

func TestEpochs_OnlyAllowedPrefixesAreLive(t *testing.T) {
	e := newEpochs("net", "app")
	today := DayAt(time.Now())

	assert.True(t, e.IsLive(today.DAG("net")))
	assert.True(t, e.IsLive(today.DAG("app")))
	assert.False(t, e.IsLive(today.DAG("bogus")), "a prefix no store expires would keep its writes forever")
	assert.False(t, e.IsLive(today.DAG("net-")), "nor may a near-copy of a real prefix")
}

func TestEpochs_GarbageBroadcastsAreDropped(t *testing.T) {
	e := newEpochs()
	_, ok := e.liveHeads([]byte("not a protobuf \xff\xff"))
	assert.False(t, ok)
	_, ok = e.liveHeads(encodeHeads(t))
	assert.False(t, ok, "a broadcast with no heads carries nothing")
}

func TestEpochs_ALiveBroadcastIsPassedOnUnchanged(t *testing.T) {
	e := newEpochs()
	today := DayAt(time.Now())
	data := encodeHeads(t, today.DAG("net"), today.DAG("app"))

	live, ok := e.liveHeads(data)
	require.True(t, ok)
	assert.Equal(t, data, live, "nothing to drop means the same bytes go on")
}

type recordingBroadcaster struct {
	sent  [][]byte
	inbox [][]byte
}

func (b *recordingBroadcaster) Broadcast(_ context.Context, data []byte) error {
	b.sent = append(b.sent, data)
	return nil
}

func (b *recordingBroadcaster) Next(ctx context.Context) ([]byte, error) {
	if len(b.inbox) == 0 {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	next := b.inbox[0]
	b.inbox = b.inbox[1:]
	return next, nil
}

func TestEpochs_TheBroadcasterFiltersBothWays(t *testing.T) {
	e := newEpochs()
	e.Expire("net", 5)
	today := DayAt(time.Now())
	inner := &recordingBroadcaster{inbox: [][]byte{
		encodeHeads(t, "net-1"),
		[]byte("garbage"),
		encodeHeads(t, "net-1", today.DAG("net")),
	}}
	b := liveBroadcaster{Broadcaster: inner, crdt: e}

	require.NoError(t, b.Broadcast(context.Background(), encodeHeads(t, "net-1")))
	assert.Empty(t, inner.sent, "a broadcast of expired days only is not sent")
	require.NoError(t, b.Broadcast(context.Background(), encodeHeads(t, "net-2", today.DAG("net"))))
	require.Len(t, inner.sent, 1)
	assert.Equal(t, []string{today.DAG("net")}, headNames(t, inner.sent[0]))

	got, err := b.Next(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{today.DAG("net")}, headNames(t, got), "dead and garbage broadcasts are skipped on receipt")
}

func TestEpochs_PurgeExpiredDropsOnlyDAGsThatAreNotLive(t *testing.T) {
	m := newTestCRDT(t)
	ctx := context.Background()
	today := DayAt(time.Now())
	for i, day := range []Day{today - 10, today} {
		delta, err := m.Set().Add(ctx, "/k"+strconv.Itoa(i), total(1))
		require.NoError(t, err)
		delta.SetDagName(day.DAG("test"))
		require.NoError(t, m.Publish(ctx, delta))
	}

	m.Expire("test", today-1)
	require.NoError(t, m.PurgeExpired(ctx))

	heads := m.InternalStats(ctx).Heads
	require.Len(t, heads, 1)
	assert.Equal(t, today.DAG("test"), heads[0].DAGName)
	has, err := m.Has(ctx, datastore.NewKey("/k0"))
	require.NoError(t, err)
	assert.False(t, has, "the expired day's writes go with it")
	has, err = m.Has(ctx, datastore.NewKey("/k1"))
	require.NoError(t, err)
	assert.True(t, has)
}

func TestEpochs_PublishWritesOneBlockPerDAG(t *testing.T) {
	m := newTestCRDT(t)
	ctx := context.Background()
	today := DayAt(time.Now())

	require.NoError(t, m.Publish(ctx), "nothing to publish is not an error")
	assert.Empty(t, m.InternalStats(ctx).Heads)

	days := []Day{today, today - 1, today, today - 1}
	deltas := make([]crdt.Delta, 0, len(days))
	for i, day := range days {
		delta, err := m.Set().Add(ctx, "/p"+strconv.Itoa(i), total(1))
		require.NoError(t, err)
		delta.SetDagName(day.DAG("test"))
		deltas = append(deltas, delta)
	}
	require.NoError(t, m.Publish(ctx, deltas...))

	heads := m.InternalStats(ctx).Heads
	require.Len(t, heads, 2, "writes of two days are two blocks, never one")
	for _, h := range heads {
		assert.EqualValues(t, 1, h.Height)
	}
}

func TestEpochs_PublishSplitsABlockAtTheSizeCap(t *testing.T) {
	m := newTestCRDT(t)
	ctx := context.Background()
	dagName := DayAt(time.Now()).DAG("test")

	deltas := make([]crdt.Delta, 0, 5)
	for i := range 5 {
		delta, err := m.Set().Add(ctx, "/big"+strconv.Itoa(i), make([]byte, maxDeltaSize/3))
		require.NoError(t, err)
		delta.SetDagName(dagName)
		deltas = append(deltas, delta)
	}
	require.NoError(t, m.Publish(ctx, deltas...))

	assert.EqualValues(t, 3, m.InternalStats(ctx).MaxHeight, "two third-cap writes fit a block, three do not")
}

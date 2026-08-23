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
	datastore "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLiveDatastore(t *testing.T) *Datastore {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	d, err := NewDatastore(
		ctx,
		&silentBroadcaster{},
		dssync.MutexWrap(datastore.NewMapDatastore()),
		newStatsHost(t),
		noProviderRouter{},
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	return d
}

type hookLog struct {
	mu   sync.Mutex
	puts []string
	dels []string
}

func (h *hookLog) onPut(k ds.Key, _ []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.puts = append(h.puts, k.String())
}

func (h *hookLog) onDelete(k ds.Key) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dels = append(h.dels, k.String())
}

func (h *hookLog) snapshot() ([]string, []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.puts...), append([]string(nil), h.dels...)
}

// go-ds-crdt has room for exactly one PutHook and one DeleteHook, and a
// node has exactly one datastore — so every store sharing it has to be
// able to subscribe, not just the first one to ask.
func TestDatastore_HooksFanOutToEveryTenant(t *testing.T) {
	d := newLiveDatastore(t)

	stats, rating := &hookLog{}, &hookLog{}
	d.OnPut(stats.onPut)
	d.OnDelete(stats.onDelete)
	d.OnPut(rating.onPut)
	d.OnDelete(rating.onDelete)

	key := datastore.NewKey("/RATING/obs/subject/observer/network/1/gen")
	require.NoError(t, d.Put(context.Background(), key, []byte("record")))

	assert.Eventually(t, func() bool {
		sp, _ := stats.snapshot()
		rp, _ := rating.snapshot()
		return len(sp) == 1 && len(rp) == 1
	}, time.Second, 10*time.Millisecond, "both tenants must see the put")

	require.NoError(t, d.Delete(context.Background(), key))

	assert.Eventually(t, func() bool {
		_, sd := stats.snapshot()
		_, rd := rating.snapshot()
		return len(sd) == 1 && len(rd) == 1
	}, time.Second, 10*time.Millisecond, "both tenants must see the delete")
}

// Tenants share the datastore but not the keyspace: a prefix query must
// return one tenant's records and none of the other's.
func TestDatastore_TenantsAreSeparatedByPrefix(t *testing.T) {
	d := newLiveDatastore(t)
	ctx := context.Background()

	require.NoError(t, d.Put(ctx, datastore.NewKey("/STATS/incr/likes/node/gen"), []byte("1")))
	require.NoError(t, d.Put(ctx, datastore.NewKey("/RATING/obs/a/b/network/1/gen"), []byte("r")))

	for prefix, want := range map[string]int{"/STATS": 1, "/RATING/obs": 1} {
		results, err := d.Query(ctx, ds.Query{Prefix: prefix})
		require.NoError(t, err)
		var got int
		for r := range results.Next() {
			require.NoError(t, r.Error)
			got++
		}
		_ = results.Close()
		assert.Equal(t, want, got, "prefix %s", prefix)
	}
}

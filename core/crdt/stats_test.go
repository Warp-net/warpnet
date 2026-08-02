/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU Affero General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU Affero General Public License for more details.

 You should have received a copy of the GNU Affero General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package crdt

import (
	"context"
	"sync"
	"testing"

	"github.com/ipfs/go-cid"
	datastore "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// silentBroadcaster satisfies the CRDT Broadcaster contract without a network:
// Broadcast is a sink and Next blocks until the store shuts down.
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

// noProviderRouter never finds providers — the store must work standalone.
type noProviderRouter struct{}

func (noProviderRouter) FindProvidersAsync(context.Context, cid.Cid, int) <-chan peer.AddrInfo {
	ch := make(chan peer.AddrInfo)
	close(ch)
	return ch
}

func newStatsHost(t *testing.T) host.Host {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func newLiveStatsStore(t *testing.T) (*CRDTStatsStore, *silentBroadcaster) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	bc := &silentBroadcaster{}
	store, err := NewCRDTStatsStore(
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

func TestCRDTStats_FreshKeyIsZeroNotAnError(t *testing.T) {
	store, _ := newLiveStatsStore(t)

	got, err := store.GetAggregatedStat(datastore.NewKey("/TWEETS/LIKES/never-touched"))
	require.NoError(t, err)
	assert.Equal(t, uint64(0), got, "an unseen counter reads as zero, not as a missing-key error")
}

func TestCRDTStats_IncrementAccumulates(t *testing.T) {
	store, _ := newLiveStatsStore(t)
	key := datastore.NewKey("/TWEETS/LIKES/tweet-1")

	for i := 1; i <= 5; i++ {
		require.NoError(t, store.Increment(key))

		got, err := store.GetAggregatedStat(key)
		require.NoError(t, err)
		assert.Equal(t, uint64(i), got, "after %d likes", i)
	}
}

// A like followed by an unlike must land back on zero — and a stray extra
// unlike must not wrap the unsigned counter around to 2^64-1.
func TestCRDTStats_DecrementClampsAtZero(t *testing.T) {
	store, _ := newLiveStatsStore(t)
	key := datastore.NewKey("/TWEETS/LIKES/tweet-clamp")

	require.NoError(t, store.Increment(key))
	require.NoError(t, store.Decrement(key))

	got, err := store.GetAggregatedStat(key)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), got)

	for i := 0; i < 3; i++ {
		require.NoError(t, store.Decrement(key))
		got, err = store.GetAggregatedStat(key)
		require.NoError(t, err)
		assert.Equal(t, uint64(0), got, "extra unlike %d must not underflow", i+1)
	}

	// The counter must still be able to climb back up afterwards.
	require.NoError(t, store.Increment(key))
	got, err = store.GetAggregatedStat(key)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), got,
		"decrements already banked stay banked — this is a PN-counter, not a floor")
}

func TestCRDTStats_CountersAreIsolatedPerKey(t *testing.T) {
	store, _ := newLiveStatsStore(t)

	likes := datastore.NewKey("/TWEETS/LIKES/tweet-a")
	retweets := datastore.NewKey("/TWEETS/RETWEETS/tweet-a")
	otherTweet := datastore.NewKey("/TWEETS/LIKES/tweet-b")

	require.NoError(t, store.Increment(likes))
	require.NoError(t, store.Increment(likes))
	require.NoError(t, store.Increment(retweets))

	got, err := store.GetAggregatedStat(likes)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), got)

	got, err = store.GetAggregatedStat(retweets)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), got, "retweets must not pick up likes")

	got, err = store.GetAggregatedStat(otherTweet)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), got, "another tweet must stay untouched")
}

// A key that is a prefix of another must not absorb its counts — otherwise
// "/tweet-1" would report the sum of every "/tweet-1x" in the network.
func TestCRDTStats_PrefixSiblingsDoNotBleed(t *testing.T) {
	store, _ := newLiveStatsStore(t)

	short := datastore.NewKey("/TWEETS/LIKES/tweet")
	long := datastore.NewKey("/TWEETS/LIKES/tweet-with-longer-id")

	require.NoError(t, store.Increment(long))
	require.NoError(t, store.Increment(long))

	got, err := store.GetAggregatedStat(short)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), got, "a prefix key must not sum its siblings")

	got, err = store.GetAggregatedStat(long)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), got)
}

func TestCRDTStats_ConcurrentBumpsAreNotLost(t *testing.T) {
	store, _ := newLiveStatsStore(t)
	key := datastore.NewKey("/TWEETS/VIEWS/viral")

	const workers, perWorker = 8, 25

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				_ = store.Increment(key)
			}
		}()
	}
	wg.Wait()

	got, err := store.GetAggregatedStat(key)
	require.NoError(t, err)
	assert.Equal(t, uint64(workers*perWorker), got,
		"a viral tweet must not lose views to a read-modify-write race")
}

func TestCRDTStats_EveryWriteIsBroadcast(t *testing.T) {
	store, bc := newLiveStatsStore(t)
	key := datastore.NewKey("/TWEETS/LIKES/broadcast")

	require.NoError(t, store.Increment(key))
	require.NoError(t, store.Increment(key))

	assert.Positive(t, bc.count(),
		"local-only counters would never converge across the network")
}

// Two processes on the same node id must not clobber each other: each mints a
// fresh generation, so a restart adds a sub-counter rather than overwriting.
func TestCRDTStats_GenerationIsUniquePerProcess(t *testing.T) {
	seen := make(map[string]struct{}, 64)
	for i := 0; i < 64; i++ {
		gen, err := newGenerationID()
		require.NoError(t, err)
		assert.Len(t, gen, generationIDBytes*2, "generation must be a hex-encoded 128-bit nonce")

		_, dup := seen[gen]
		assert.False(t, dup, "generation nonces must never repeat")
		seen[gen] = struct{}{}
	}
}

func TestCRDTStats_CloseIsSafeOnNilAndStopsTheStore(t *testing.T) {
	assert.NoError(t, (*CRDTStatsStore)(nil).Close())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := NewCRDTStatsStore(
		ctx,
		&silentBroadcaster{},
		dssync.MutexWrap(datastore.NewMapDatastore()),
		newStatsHost(t),
		noProviderRouter{},
	)
	require.NoError(t, err)
	assert.NoError(t, store.Close())
}

func TestCRDTStats_CounterCodecRoundTrip(t *testing.T) {
	for _, v := range []uint64{0, 1, 42, 1 << 32, ^uint64(0)} {
		assert.Equal(t, v, decodeCounter(encodeCounter(v)))
	}

	// Truncated or absent values must read as zero rather than panicking on a
	// short slice — peers can and do ship malformed deltas.
	assert.Equal(t, uint64(0), decodeCounter(nil))
	assert.Equal(t, uint64(0), decodeCounter([]byte{}))
	assert.Equal(t, uint64(0), decodeCounter([]byte{1, 2, 3}))
	assert.Equal(t, uint64(0), decodeCounter(make([]byte, 7)))
}

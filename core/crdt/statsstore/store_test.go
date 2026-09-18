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
package statsstore

import (
	"context"
	"sync"
	"testing"
	"time"

	datastore "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func newStatsHost(t *testing.T) host.Host {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func newLiveStatsStore(t *testing.T) (*Store, *silentBroadcaster) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	bc := &silentBroadcaster{}
	store, err := New(
		ctx,
		bc,
		dssync.MutexWrap(datastore.NewMapDatastore()),
		newStatsHost(t),
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

func TestCRDTStats_AFlushIsBroadcastOnce(t *testing.T) {
	store, bc := newLiveStatsStore(t)
	likes := datastore.NewKey("/TWEETS/LIKES/broadcast")
	views := datastore.NewKey("/TWEETS/VIEWS/broadcast")

	for i := 0; i < 5; i++ {
		require.NoError(t, store.Increment(likes))
	}
	require.NoError(t, store.Increment(views))

	assert.Zero(t, bc.count(), "a bump on its own must not reach the network")

	require.NoError(t, store.flush())
	assert.Equal(t, 1, bc.count(),
		"every buffered counter goes out as one delta, not one per bump")

	require.NoError(t, store.flush())
	assert.Equal(t, 1, bc.count(), "a flush with nothing new must stay quiet")
}

func TestCRDTStats_FlushedCountersSurviveTheBuffer(t *testing.T) {
	store, _ := newLiveStatsStore(t)
	key := datastore.NewKey("/TWEETS/LIKES/flushed")

	require.NoError(t, store.Increment(key))
	require.NoError(t, store.Increment(key))
	require.NoError(t, store.flush())
	require.NoError(t, store.Increment(key))

	got, err := store.GetAggregatedStat(key)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), got,
		"a read must sum what the CRDT holds and what is still buffered")
}

type pairBroadcaster struct {
	inbox chan []byte
	peer  *pairBroadcaster
}

func newBroadcasterPair() (*pairBroadcaster, *pairBroadcaster) {
	a := &pairBroadcaster{inbox: make(chan []byte, 64)}
	b := &pairBroadcaster{inbox: make(chan []byte, 64)}
	a.peer, b.peer = b, a
	return a, b
}

func (p *pairBroadcaster) Broadcast(_ context.Context, data []byte) error {
	select {
	case p.peer.inbox <- append([]byte(nil), data...):
	default:
	}
	return nil
}

func (p *pairBroadcaster) Next(ctx context.Context) ([]byte, error) {
	select {
	case data := <-p.inbox:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestCRDTStats_AFlushReachesTheOtherReplica(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	author, reader := newStatsHost(t), newStatsHost(t)
	require.NoError(t, author.Connect(ctx, peer.AddrInfo{
		ID: reader.ID(), Addrs: reader.Addrs(),
	}))

	authorBc, readerBc := newBroadcasterPair()
	authorStore, err := New(
		ctx, authorBc, dssync.MutexWrap(datastore.NewMapDatastore()), author,
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = authorStore.Close() })

	readerStore, err := New(
		ctx, readerBc, dssync.MutexWrap(datastore.NewMapDatastore()), reader,
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = readerStore.Close() })

	likes := datastore.NewKey("/TWEETS/LIKES/converge")
	views := datastore.NewKey("/TWEETS/VIEWS/converge")
	for i := 0; i < 3; i++ {
		require.NoError(t, authorStore.Increment(likes))
	}
	require.NoError(t, authorStore.Increment(views))
	require.NoError(t, authorStore.flush())

	require.Eventually(t, func() bool {
		gotLikes, err := readerStore.GetAggregatedStat(likes)
		if err != nil || gotLikes != 3 {
			return false
		}
		gotViews, err := readerStore.GetAggregatedStat(views)
		return err == nil && gotViews == 1
	}, 20*time.Second, 100*time.Millisecond,
		"counters batched into one delta must still arrive at their own keys")
}

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
	assert.NoError(t, (*Store)(nil).Close())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := New(
		ctx,
		&silentBroadcaster{},
		dssync.MutexWrap(datastore.NewMapDatastore()),
		newStatsHost(t),
	)
	require.NoError(t, err)
	assert.NoError(t, store.Close())
	assert.NotPanics(t, func() { _ = store.Close() },
		"a second Close must not close the stop channel twice")
}

func TestCRDTStats_CloseStopsTheFlushWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bc := &silentBroadcaster{}
	store, err := New(
		ctx,
		bc,
		dssync.MutexWrap(datastore.NewMapDatastore()),
		newStatsHost(t),
	)
	require.NoError(t, err)

	require.NoError(t, store.Increment(datastore.NewKey("/TWEETS/LIKES/closing")))

	closed := make(chan error, 1)
	go func() { closed <- store.Close() }()
	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Close waits on a flush worker that only its context can stop")
	}

	assert.Equal(t, 1, bc.count(), "Close flushes what is buffered before it stops")
}

func TestCRDTStats_CounterCodecRoundTrip(t *testing.T) {
	for _, v := range []uint64{0, 1, 42, 1 << 32, ^uint64(0)} {
		assert.Equal(t, v, decodeCounter(encodeCounter(v)))
	}

	assert.Equal(t, uint64(0), decodeCounter(nil))
	assert.Equal(t, uint64(0), decodeCounter([]byte{}))
	assert.Equal(t, uint64(0), decodeCounter([]byte{1, 2, 3}))
	assert.Equal(t, uint64(0), decodeCounter(make([]byte, 7)))
}

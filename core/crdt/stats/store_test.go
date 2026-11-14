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
package stats

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/crdt/store"
	"github.com/Warp-net/warpnet/core/warpnet"
	datastore "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/ipfs/go-ds-crdt/pb"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

var testTweet = ulid.Make().String()

// tweetKey is a counter key of the test tweet: stats finds the tweet by
// the ULID among the key's parts.
func tweetKey(path string) datastore.Key {
	return datastore.NewKey(path + "/" + testTweet)
}

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
		idleRouter{},
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	return store, bc
}

func TestCRDTStats_FreshKeyIsZeroNotAnError(t *testing.T) {
	store, _ := newLiveStatsStore(t)

	got, err := store.GetAggregatedStat(tweetKey("/TWEETS/LIKES/never-touched"), 0)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), got, "an unseen counter reads as zero, not as a missing-key error")
}

func TestCRDTStats_IncrementAccumulates(t *testing.T) {
	store, _ := newLiveStatsStore(t)
	key := tweetKey("/TWEETS/LIKES/tweet-1")

	for i := 1; i <= 5; i++ {
		require.NoError(t, store.Increment(key))

		got, err := store.GetAggregatedStat(key, 0)
		require.NoError(t, err)
		assert.Equal(t, uint64(i), got, "after %d likes", i)
	}
}

func TestCRDTStats_DecrementClampsAtZero(t *testing.T) {
	store, _ := newLiveStatsStore(t)
	key := tweetKey("/TWEETS/LIKES/tweet-clamp")

	require.NoError(t, store.Increment(key))
	require.NoError(t, store.Decrement(key))

	got, err := store.GetAggregatedStat(key, 0)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), got)

	for i := 0; i < 3; i++ {
		require.NoError(t, store.Decrement(key))
		got, err = store.GetAggregatedStat(key, 0)
		require.NoError(t, err)
		assert.Equal(t, uint64(0), got, "extra unlike %d must not underflow", i+1)
	}

	require.NoError(t, store.Increment(key))
	got, err = store.GetAggregatedStat(key, 0)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), got,
		"decrements already banked stay banked — this is a PN-counter, not a floor")
}

func TestCRDTStats_CountersAreIsolatedPerKey(t *testing.T) {
	store, _ := newLiveStatsStore(t)

	likes := tweetKey("/TWEETS/LIKES/tweet-a")
	retweets := tweetKey("/TWEETS/RETWEETS/tweet-a")
	otherTweet := tweetKey("/TWEETS/LIKES/tweet-b")

	require.NoError(t, store.Increment(likes))
	require.NoError(t, store.Increment(likes))
	require.NoError(t, store.Increment(retweets))

	got, err := store.GetAggregatedStat(likes, 0)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), got)

	got, err = store.GetAggregatedStat(retweets, 0)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), got, "retweets must not pick up likes")

	got, err = store.GetAggregatedStat(otherTweet, 0)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), got, "another tweet must stay untouched")
}

func TestCRDTStats_PrefixSiblingsDoNotBleed(t *testing.T) {
	store, _ := newLiveStatsStore(t)

	short := tweetKey("/TWEETS/LIKES/tweet")
	long := tweetKey("/TWEETS/LIKES/tweet-with-longer-id")

	require.NoError(t, store.Increment(long))
	require.NoError(t, store.Increment(long))

	got, err := store.GetAggregatedStat(short, 0)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), got, "a prefix key must not sum its siblings")

	got, err = store.GetAggregatedStat(long, 0)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), got)
}

func TestCRDTStats_ConcurrentBumpsAreNotLost(t *testing.T) {
	store, _ := newLiveStatsStore(t)
	key := tweetKey("/TWEETS/VIEWS/viral")

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

	got, err := store.GetAggregatedStat(key, 0)
	require.NoError(t, err)
	assert.Equal(t, uint64(workers*perWorker), got,
		"a viral tweet must not lose views to a read-modify-write race")
}

func TestCRDTStats_AFlushIsBroadcastOnce(t *testing.T) {
	store, bc := newLiveStatsStore(t)
	likes := tweetKey("/TWEETS/LIKES/broadcast")
	views := tweetKey("/TWEETS/VIEWS/broadcast")

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
	key := tweetKey("/TWEETS/LIKES/flushed")

	require.NoError(t, store.Increment(key))
	require.NoError(t, store.Increment(key))
	require.NoError(t, store.flush())
	require.NoError(t, store.Increment(key))

	got, err := store.GetAggregatedStat(key, 0)
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
		ctx, authorBc, dssync.MutexWrap(datastore.NewMapDatastore()), author, idleRouter{},
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = authorStore.Close() })

	readerStore, err := New(
		ctx, readerBc, dssync.MutexWrap(datastore.NewMapDatastore()), reader, idleRouter{},
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = readerStore.Close() })

	likes := tweetKey("/TWEETS/LIKES/converge")
	views := tweetKey("/TWEETS/VIEWS/converge")
	for i := 0; i < 3; i++ {
		require.NoError(t, authorStore.Increment(likes))
	}
	require.NoError(t, authorStore.Increment(views))
	require.NoError(t, authorStore.flush())

	require.Eventually(t, func() bool {
		gotLikes, err := readerStore.GetAggregatedStat(likes, 0)
		if err != nil || gotLikes != 3 {
			return false
		}
		gotViews, err := readerStore.GetAggregatedStat(views, 0)
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
		idleRouter{},
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
		idleRouter{},
	)
	require.NoError(t, err)

	require.NoError(t, store.Increment(tweetKey("/TWEETS/LIKES/closing")))

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

func newStatsPair(t *testing.T) (*Store, *Store) {
	t.Helper()
	a, b, _ := newStatsPairWire(t)
	return a, b
}

// newStatsPairWire also hands back the inbox of the first store, to deliver
// a broadcast the second store would never send itself.
func newStatsPairWire(t *testing.T) (*Store, *Store, chan<- []byte) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	a, b := newStatsHost(t), newStatsHost(t)
	require.NoError(t, a.Connect(ctx, peer.AddrInfo{ID: b.ID(), Addrs: b.Addrs()}))
	aBc, bBc := newBroadcasterPair()

	aStore, err := New(ctx, aBc, dssync.MutexWrap(datastore.NewMapDatastore()), a, idleRouter{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = aStore.Close() })
	bStore, err := New(ctx, bBc, dssync.MutexWrap(datastore.NewMapDatastore()), b, idleRouter{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = bStore.Close() })
	return aStore, bStore, aBc.inbox
}

func tweetAt(t time.Time) string {
	return ulid.MustNew(ulid.Timestamp(t), rand.Reader).String()
}

func publishRaw(t *testing.T, s *Store, key string, value []byte, dag string) {
	t.Helper()
	delta, err := s.crdt.Set().Add(context.Background(), key, value)
	require.NoError(t, err)
	delta.SetDagName(dag)
	require.NoError(t, s.crdt.Publish(context.Background(), delta))
}

func TestCRDTStats_ACounterNotSignedByItsNodeIsIgnored(t *testing.T) {
	honest, attacker := newStatsPair(t)
	likes := tweetKey("/REACTIONS/INCR/forged")
	day := store.DayAt(time.Now()).DAG(dagPrefix)

	require.NoError(t, honest.Increment(likes))
	require.NoError(t, honest.flush())

	base := counterPrefix + "/" + incrNamespace + likes.String()
	unsigned := binary.BigEndian.AppendUint64(nil, 1_000_000)
	publishRaw(t, attacker, base+"/"+attacker.nodeID+"/unsigned", unsigned, day)
	impersonated := base + "/" + honest.nodeID + "/stolen"
	publishRaw(t, attacker, impersonated, attacker.signedValue(impersonated, 1_000_000), day)
	signed := base + "/" + attacker.nodeID + "/own"
	publishRaw(t, attacker, signed, attacker.signedValue(signed, 1), day)

	require.Eventually(t, func() bool {
		got, err := honest.GetAggregatedStat(likes, 0)
		return err == nil && got == 2
	}, 20*time.Second, 100*time.Millisecond,
		"only the honest like and the attacker's own signed one count; unsigned and impersonated values do not")
}

func TestCRDTStats_ANodeCannotLowerAnotherNodesCount(t *testing.T) {
	honest, attacker := newStatsPair(t)
	likes := tweetKey("/REACTIONS/INCR/zeroing")

	require.NoError(t, honest.Increment(likes))
	require.NoError(t, honest.Increment(likes))
	require.NoError(t, honest.flush())
	for range 100 {
		require.NoError(t, attacker.Decrement(likes))
	}
	require.NoError(t, attacker.flush())

	require.Eventually(t, func() bool {
		got, err := attacker.GetAggregatedStat(likes, 0)
		return err == nil && got == 2
	}, 20*time.Second, 100*time.Millisecond)
	got, err := honest.GetAggregatedStat(likes, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 2, got, "a node's decrements only cancel its own increments")
}

func TestCRDTStats_OldAndNonULIDTweetsStayLocal(t *testing.T) {
	store, bc := newLiveStatsStore(t)
	old := datastore.NewKey("/REACTIONS/INCR/" + tweetAt(time.Now().Add(-(retentionDays+1)*24*time.Hour)))
	fediverse := datastore.NewKey("/REACTIONS/INCR/mastodon-note-1")

	require.NoError(t, store.Increment(old))
	require.NoError(t, store.Increment(fediverse))
	require.NoError(t, store.flush())

	for _, key := range []datastore.Key{old, fediverse} {
		got, err := store.GetAggregatedStat(key, 0)
		require.NoError(t, err)
		assert.Zero(t, got)
	}
	assert.Zero(t, bc.count(), "nothing about such tweets reaches the network")
}

func TestCRDTStats_CountersGoToTheDAGOfTheTweetsDay(t *testing.T) {
	s, _ := newLiveStatsStore(t)
	created := time.Now().Add(-3 * 24 * time.Hour)
	likes := datastore.NewKey("/REACTIONS/INCR/" + tweetAt(created))

	require.NoError(t, s.Increment(likes))
	require.NoError(t, s.flush())

	heads := s.crdt.InternalStats(context.Background()).Heads
	require.Len(t, heads, 1)
	assert.Equal(t, store.DayAt(created).DAG(dagPrefix), heads[0].DAGName)
}

func TestCRDTStats_PurgeDropsTweetsPastRetention(t *testing.T) {
	s, _ := newLiveStatsStore(t)
	likes := datastore.NewKey("/REACTIONS/INCR/" + tweetAt(time.Now().Add(-(retentionDays-1)*24*time.Hour)))

	require.NoError(t, s.Increment(likes))
	require.NoError(t, s.flush())
	got, err := s.GetAggregatedStat(likes, 0)
	require.NoError(t, err)
	require.EqualValues(t, 1, got)

	require.NoError(t, s.purge(time.Now().Add(2*24*time.Hour)))

	assert.Empty(t, s.crdt.InternalStats(context.Background()).Heads)
	got, err = s.GetAggregatedStat(likes, 0)
	require.NoError(t, err)
	assert.Zero(t, got)
	s.mu.Lock()
	assert.Empty(t, s.counters, "flushed counters of an expired day are forgotten")
	s.mu.Unlock()
}

func TestCRDTStats_ANodeCountsOnceWhereAnOwnerActsOnce(t *testing.T) {
	honest, attacker := newStatsPair(t)
	likes := tweetKey("/REACTIONS/INCR/capped")

	require.NoError(t, honest.Increment(likes))
	require.NoError(t, honest.flush())
	for range 1000 {
		require.NoError(t, attacker.Increment(likes))
	}
	require.NoError(t, attacker.flush())

	require.Eventually(t, func() bool {
		got, err := honest.GetAggregatedStat(likes, 0)
		return err == nil && got == 1001
	}, 20*time.Second, 100*time.Millisecond)
	got, err := honest.GetAggregatedStat(likes, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 2, got, "a thousand self-signed likes from one node are one like")
}

func TestCRDTStats_AnInvalidValueCannotDisplaceAnHonestOne(t *testing.T) {
	honest, attacker := newStatsPair(t)
	likes := tweetKey("/REACTIONS/INCR/displaced")
	dag, ok := tweetDAG(likes)
	require.True(t, ok)

	require.NoError(t, honest.Increment(likes))
	require.NoError(t, honest.flush())
	require.Eventually(t, func() bool {
		got, err := attacker.GetAggregatedStat(likes, 1)
		return err == nil && got == 1
	}, 20*time.Second, 100*time.Millisecond)

	honestKey := honest.counterKey(incrNamespace, likes).String()
	publishRaw(t, attacker, honestKey, make([]byte, signedValueSize), dag)
	marker := tweetKey("/REACTIONS/INCR/marker")
	require.NoError(t, attacker.Increment(marker))
	require.NoError(t, attacker.flush())
	require.Eventually(t, func() bool {
		got, err := honest.GetAggregatedStat(marker, 1)
		return err == nil && got == 1
	}, 20*time.Second, 100*time.Millisecond)

	got, err := honest.GetAggregatedStat(likes, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 1, got, "a value its node did not sign is refused before it can win the key")
}

func TestCRDTStats_ACounterOutsideItsTweetsDayIsRefused(t *testing.T) {
	honest, attacker := newStatsPair(t)
	old := datastore.NewKey("/REACTIONS/INCR/" + tweetAt(time.Now().Add(-60*24*time.Hour)))
	today := store.DayAt(time.Now()).DAG(dagPrefix)

	oldKey := attacker.counterKey(incrNamespace, old).String()
	publishRaw(t, attacker, oldKey, attacker.signedValue(oldKey, 1), today)
	marker := tweetKey("/REACTIONS/INCR/marker")
	require.NoError(t, attacker.Increment(marker))
	require.NoError(t, attacker.flush())
	require.Eventually(t, func() bool {
		got, err := honest.GetAggregatedStat(marker, 1)
		return err == nil && got == 1
	}, 20*time.Second, 100*time.Millisecond)

	has, err := honest.crdt.Has(context.Background(), datastore.NewKey(oldKey))
	require.NoError(t, err)
	assert.False(t, has, "a signed counter filed under another day's DAG would outlive its tweet")
	require.NoError(t, honest.purge(time.Now()))
	got, err := honest.GetAggregatedStat(old, 1)
	require.NoError(t, err)
	assert.Zero(t, got)
}

func TestCRDTStats_ABlockAnnouncedUnderAnotherDayIsRefused(t *testing.T) {
	honest, attacker, honestInbox := newStatsPairWire(t)
	created := time.Now().Add(-60 * 24 * time.Hour)
	old := datastore.NewKey("/REACTIONS/INCR/" + tweetAt(created))
	oldDAG := store.DayAt(created).DAG(dagPrefix)

	oldKey := attacker.counterKey(incrNamespace, old).String()
	publishRaw(t, attacker, oldKey, attacker.signedValue(oldKey, 1), oldDAG)
	var block []byte
	for _, head := range attacker.crdt.InternalStats(context.Background()).Heads {
		if head.DAGName == oldDAG {
			block = head.Cid.Bytes()
		}
	}
	require.NotNil(t, block)
	today := store.DayAt(time.Now()).DAG(dagPrefix)
	announced, err := proto.Marshal(&pb.CRDTBroadcast{Heads: []*pb.Head{{Cid: block, DagName: today}}})
	require.NoError(t, err)
	honestInbox <- announced

	marker := tweetKey("/REACTIONS/INCR/marker")
	require.NoError(t, attacker.Increment(marker))
	require.NoError(t, attacker.flush())
	require.Eventually(t, func() bool {
		got, err := honest.GetAggregatedStat(marker, 1)
		return err == nil && got == 1
	}, 20*time.Second, 100*time.Millisecond)

	has, err := honest.crdt.Has(context.Background(), datastore.NewKey(oldKey))
	require.NoError(t, err)
	assert.False(t, has, "a block filed under an expired day and announced under today's is refused")
}

func TestCRDTStats_AReplayedOlderTotalCannotWinTheKey(t *testing.T) {
	honest, attacker := newStatsPair(t)
	likes := tweetKey("/REACTIONS/INCR/replayed")
	dag, ok := tweetDAG(likes)
	require.True(t, ok)
	honestKey := honest.counterKey(incrNamespace, likes).String()

	require.NoError(t, honest.Increment(likes))
	require.NoError(t, honest.flush())
	replay := honest.signedValue(honestKey, 1)
	require.NoError(t, honest.Increment(likes))
	require.NoError(t, honest.flush())
	require.Eventually(t, func() bool {
		got, err := attacker.GetAggregatedStat(likes, 0)
		return err == nil && got == 2
	}, 20*time.Second, 100*time.Millisecond)

	publishRaw(t, attacker, honestKey, replay, dag)
	marker := tweetKey("/REACTIONS/INCR/marker")
	require.NoError(t, attacker.Increment(marker))
	require.NoError(t, attacker.flush())
	require.Eventually(t, func() bool {
		got, err := honest.GetAggregatedStat(marker, 1)
		return err == nil && got == 1
	}, 20*time.Second, 100*time.Millisecond)

	got, err := honest.GetAggregatedStat(likes, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 2, got, "the node's own older total, replayed at a higher priority, does not roll the count back")
}

func TestCRDTStats_ValidateCounter(t *testing.T) {
	s, _ := newLiveStatsStore(t)
	likes := tweetKey("/REACTIONS/INCR/validated")
	dag, ok := tweetDAG(likes)
	require.True(t, ok)
	key := s.counterKey(incrNamespace, likes).String()
	signed := s.signedValue(key, 3)

	assert.NoError(t, validateCounter(dag, key, signed))

	assert.ErrorIs(t, validateCounter(store.DayAt(time.Now().Add(-48*time.Hour)).DAG(dagPrefix), key, signed), ErrForeignDAG,
		"a counter filed under another day")
	noULID := counterPrefix + "/" + incrNamespace + "/REACTIONS/INCR/mastodon/" + s.nodeID + "/" + s.generation
	assert.ErrorIs(t, validateCounter(dag, noULID, s.signedValue(noULID, 1)), ErrForeignDAG, "a key with no tweet id has no day")
	assert.ErrorIs(t, validateCounter(dag, key, signed[:counterSize]), ErrBadSignature, "an unsigned total")
	assert.ErrorIs(t, validateCounter(dag, key, signed[:3]), ErrBadSignature, "a value too short to hold a total")
	assert.ErrorIs(t, validateCounter(dag, key, append(slices.Clone(signed), 0)), ErrBadSignature, "a value of the wrong size")

	tampered := slices.Clone(signed)
	tampered[counterSize-1]++
	assert.ErrorIs(t, validateCounter(dag, key, tampered), ErrBadSignature, "a total the signature does not cover")
	other := s.counterKey(incrNamespace, tweetKey("/REACTIONS/INCR/other")).String()
	assert.ErrorIs(t, validateCounter(dag, other, signed), ErrBadSignature, "a signature moved to another key")
	badNode := strings.Replace(key, s.nodeID, "not-a-peer-id", 1)
	assert.ErrorIs(t, validateCounter(dag, badNode, signed), ErrBadSignature, "a key naming no node")
}

func TestCRDTStats_CompareTotals(t *testing.T) {
	assert.Negative(t, compareTotals(binary.BigEndian.AppendUint64(nil, 1), binary.BigEndian.AppendUint64(nil, 2)))
	assert.Zero(t, compareTotals(binary.BigEndian.AppendUint64(nil, 2), binary.BigEndian.AppendUint64(nil, 2)))
	assert.Positive(t, compareTotals(binary.BigEndian.AppendUint64(nil, 3), binary.BigEndian.AppendUint64(nil, 2)))
	assert.Zero(t, compareTotals([]byte{1}, binary.BigEndian.AppendUint64(nil, 2)), "a value too short to read is not ordered")
}

func TestCRDTStats_TweetDAGTakesTheULIDInTheKey(t *testing.T) {
	created := time.Now().Add(-5 * 24 * time.Hour)
	id := tweetAt(created)

	for _, key := range []string{"/REACTIONS/INCR/" + id, "/TWEETS/VIEWS/" + id + "/extra", "/POLL/VOTES/INCR/" + id + "/2"} {
		dag, ok := tweetDAG(datastore.NewKey(key))
		require.True(t, ok, key)
		assert.Equal(t, store.DayAt(created).DAG(dagPrefix), dag, key)
	}
	_, ok := tweetDAG(datastore.NewKey("/REACTIONS/INCR/https:--mastodon.example-notes-1"))
	assert.False(t, ok)
}

func TestCRDTStats_ACapOfZeroCountsEveryEvent(t *testing.T) {
	s, _ := newLiveStatsStore(t)
	replies := tweetKey("/TWEETS/REPLIES/uncapped")
	for range 4 {
		require.NoError(t, s.Increment(replies))
	}
	got, err := s.GetAggregatedStat(replies, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 4, got)
	got, err = s.GetAggregatedStat(replies, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 1, got)
}

func TestCRDTStats_ANodeBelowZeroTakesNothingFromOthers(t *testing.T) {
	honest, other := newStatsPair(t)
	likes := tweetKey("/REACTIONS/INCR/netted")

	require.NoError(t, honest.Increment(likes))
	require.NoError(t, honest.flush())
	require.NoError(t, other.Increment(likes))
	for range 5 {
		require.NoError(t, other.Decrement(likes))
	}
	require.NoError(t, other.flush())

	require.Eventually(t, func() bool {
		got, err := honest.GetAggregatedStat(likes, 0)
		return err == nil && got == 1
	}, 20*time.Second, 100*time.Millisecond, "a node with more unlikes than likes adds zero, not a negative")
}

func TestCRDTStats_PurgeKeepsCountersNotYetFlushed(t *testing.T) {
	s, _ := newLiveStatsStore(t)
	aging := datastore.NewKey("/REACTIONS/INCR/" + tweetAt(time.Now().Add(-(retentionDays-1)*24*time.Hour)))
	require.NoError(t, s.Increment(aging))

	require.NoError(t, s.purge(time.Now().Add(2*24*time.Hour)))

	s.mu.Lock()
	defer s.mu.Unlock()
	assert.Len(t, s.counters, 1, "an unflushed count is never dropped by a purge")
}

func TestCRDTStats_ANodeWithoutAnEd25519KeyIsRefused(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h, err := libp2p.New(libp2p.NoListenAddrs, libp2p.Identity(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	h.Peerstore().RemovePeer(h.ID())

	_, err = New(ctx, &silentBroadcaster{}, dssync.MutexWrap(datastore.NewMapDatastore()), h, idleRouter{})
	assert.ErrorIs(t, err, ErrNoSigningKey)
}

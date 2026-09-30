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

 WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
 Use at your own risk. The maintainers shall not be liable for any damages or data loss
 resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"math/rand/v2"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Everything the engine does at once — charging, flushing, merging,
// scoring, rating and collecting — within one hour loses no count.
func TestConcurrentUseLosesNoCount(t *testing.T) {
	const (
		peers   = 40
		writers = 16
		charges = 3000
	)
	self := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	ratings := NewPeersRatings()
	e := newRatingEngine(t, self, store, clock, ratings)

	targets := make([]identity, peers)
	for i := range targets {
		targets[i] = newIdentity(t)
	}
	observers := []identity{newIdentity(t), newIdentity(t), newIdentity(t)}
	kinds := []Kind{KindBadSignature, KindRateLimitHit, KindMalformedFrame, KindWriteFlood, KindModerationUpheld}

	var charged [peers]atomic.Int64
	var writersWG, helpersWG sync.WaitGroup
	stop := make(chan struct{})

	for w := range writers {
		writersWG.Go(func() {
			rng := rand.New(rand.NewPCG(uint64(w), 1))
			for range charges {
				i := rng.IntN(peers)
				if err := e.record(targets[i].id, kinds[rng.IntN(len(kinds))]); err == nil {
					charged[i].Add(1)
				}
			}
		})
	}
	helper := func(fn func(rng *rand.Rand)) {
		seed := uint64(len(observers)) + 100
		helpersWG.Go(func() {
			rng := rand.New(rand.NewPCG(seed, 2))
			for {
				select {
				case <-stop:
					return
				default:
					fn(rng)
				}
			}
		})
	}
	helper(func(*rand.Rand) { _ = e.flush() })
	helper(func(*rand.Rand) { _ = e.ratePeers() })
	helper(func(*rand.Rand) { e.gc() })
	helper(func(rng *rand.Rand) {
		id := targets[rng.IntN(peers)].id
		_ = e.Score(id)
		_, _ = e.View(id)
		_ = ratings.Tier(id)
	})
	helper(func(rng *rand.Rand) {
		o := observers[rng.IntN(len(observers))]
		store.merge(signedRecord(o, targets[rng.IntN(peers)].id, Network, bucketAt(clock.Now()),
			genB, kindCount{KindDialFailure, uint32(1 + rng.IntN(5))}))
	})
	helper(func(rng *rand.Rand) {
		if rng.IntN(50) == 0 {
			e.index.forget(targets[rng.IntN(peers)].id.String()) // an eviction
		}
	})

	writersWG.Wait()
	close(stop)
	helpersWG.Wait()
	flushNow(t, e)

	stored := make(map[string]int64)
	for _, rec := range store.records() {
		if rec.ObserverID != self.id.String() {
			continue
		}
		for _, o := range rec.Offences {
			stored[rec.PeerID] += int64(o.Count)
		}
	}
	for i, target := range targets {
		assert.Equal(t, charged[i].Load(), stored[target.id.String()], "peer %d", i)
	}
	for _, target := range targets {
		assert.Less(t, e.Score(target.id), MaxScore, "every peer was charged")
	}
}

// goroutinesIn counts the live goroutines whose stack passes through fn.
func goroutinesIn(fn string) int {
	buf := make([]byte, 1<<22)
	buf = buf[:runtime.Stack(buf, true)]
	n := 0
	for g := range strings.SplitSeq(string(buf), "\n\n") {
		if strings.Contains(g, fn) {
			n++
		}
	}
	return n
}

// Closing the engine while every fan-out is still flooding it returns in
// time, and its flush loop and every listener exit.
func TestCloseUnderLoadStopsEverything(t *testing.T) {
	self := newIdentity(t)
	clock := newClock()
	store := newFakeStore(self.id)
	e, err := NewEngine(t.Context(), store, acquainted(clock), self.priv, warpnet.MemberNode,
		WithClock(clock.Now), WithFlushInterval(5*time.Millisecond), WithRatings(NewPeersRatings()))
	require.NoError(t, err)

	emitters := make([]warpnet.PeerEmitter, 4)
	sources := make([]<-chan warpnet.PeerEvent, len(emitters))
	for i := range emitters {
		emitters[i] = warpnet.NewPeerEmitter()
		sources[i] = emitters[i]
	}
	e.Listen(sources...)
	require.Eventually(t, func() bool { return goroutinesIn("rating.(*Engine)") == 1+len(emitters) },
		5*time.Second, 5*time.Millisecond, "a flush loop and a listener per fan-out")

	peer := newIdentity(t)
	stop := make(chan struct{})
	var floodWG sync.WaitGroup
	for _, em := range emitters {
		floodWG.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					em.Emit(warpnet.PeerEvent{PeerID: peer.id.String(), Type: warpnet.PeerBadSignature})
				}
			}
		})
	}

	require.Eventually(t, func() bool { return e.Score(peer.id) == MinScore }, 10*time.Second, 5*time.Millisecond,
		"the flood reaches the engine and floors the peer")

	closed := make(chan struct{})
	go func() {
		_ = e.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(closeTimeout + 2*time.Second):
		t.Fatal("Close did not return while the fan-outs were flooding")
	}
	close(stop)
	floodWG.Wait()

	require.NotZero(t, store.len(), "the final flush wrote what was charged")
	assert.Eventually(t, func() bool { return goroutinesIn("rating.(*Engine)") == 0 },
		5*time.Second, 5*time.Millisecond, "the flush loop and every listener exit")
}

// The read-model the modules hit on every message stays consistent under
// parallel writes and reads.
func TestPeersRatingsUnderParallelUse(t *testing.T) {
	r := NewPeersRatings()
	ids := make([]warpnet.WarpPeerID, 64)
	for i := range ids {
		ids[i] = newIdentity(t).id
	}
	tiers := []Tier{TierTrusted, TierWatched, TierDegraded, TierFloor}

	var wg sync.WaitGroup
	for w := range 8 {
		wg.Go(func() {
			rng := rand.New(rand.NewPCG(uint64(w), 3))
			for range 20_000 {
				id := ids[rng.IntN(len(ids))]
				if rng.IntN(4) == 0 {
					r.Rate(id, tiers[rng.IntN(len(tiers))])
					continue
				}
				tier := r.Tier(id)
				require.Contains(t, tiers, tier)
				require.Equal(t, tier.RateMultiplier() > 0, r.RateMultiplier(id) > 0)
			}
		})
	}
	wg.Wait()

	for _, tier := range tiers {
		r.Rate(ids[0], tier)
		assert.Equal(t, tier, r.Tier(ids[0]), "the last word wins")
	}
}

// countingStore counts the full-history loads the engine asks for.
type countingStore struct {
	*fakeStore

	loads atomic.Int64
}

func (s *countingStore) List(peerID string) ([]domain.RatingRecord, error) {
	s.loads.Add(1)
	return s.fakeStore.List(peerID)
}

// Four and a half days of steady traffic on a simulated clock, past the
// network retention: the engine's own memory stays bounded by the last two
// hours, and its records by retention.
func TestDaysOfTrafficStayBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("simulates days of traffic")
	}
	self := newIdentity(t)
	clock := newClock()
	store := &countingStore{fakeStore: newFakeStore(self.id)}
	e := newRatingEngine(t, self, store, clock, NewPeersRatings())

	pool := make([]identity, 40)
	for i := range pool {
		pool[i] = newIdentity(t)
	}
	rng := rand.New(rand.NewPCG(21, 22))
	const step = 15 * time.Minute
	steps := int(108 * time.Hour / step)
	for i := range steps {
		for range 10 {
			require.NoError(t, e.record(pool[rng.IntN(len(pool))].id, KindRateLimitHit))
		}
		clock.advance(step)
		flushNow(t, e)
		if i%4 == 0 {
			e.gc()
			require.NoError(t, e.ratePeers())
		}

		e.mu.Lock()
		counters, dirty := len(e.counters), len(e.dirty)
		e.mu.Unlock()
		require.Zero(t, dirty, "a flush leaves nothing dirty")
		require.LessOrEqual(t, counters, 2*len(pool), "only this hour and the last are held in memory")
	}

	oldest := bucketAt(clock.Now().Add(-Network.Retention())) - 1
	for _, rec := range store.records() {
		require.GreaterOrEqual(t, rec.Bucket, int64(oldest), "gc keeps own records within retention")
	}
	assert.LessOrEqual(t, store.len(), len(pool)*int(Network.Retention()/time.Hour+2))
	assert.LessOrEqual(t, e.index.peers.Len(), len(pool))
	t.Logf("%d full-history loads over %d flushes of %d peers", store.loads.Load(), steps, len(pool))
}

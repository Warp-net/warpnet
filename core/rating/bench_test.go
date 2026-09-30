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
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	log "github.com/sirupsen/logrus"
)

func benchIdentity(b *testing.B) identity {
	b.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		b.Fatal(err)
	}
	id, err := warpnet.IDFromPublicKey(pub)
	if err != nil {
		b.Fatal(err)
	}
	return identity{id: id, priv: priv}
}

func benchEngine(b *testing.B, opts ...Option) (*Engine, *fakeStore, *fixedClock) {
	b.Helper()
	level := log.GetLevel()
	log.SetLevel(log.ErrorLevel)
	b.Cleanup(func() { log.SetLevel(level) })

	self := benchIdentity(b)
	clock := newClock()
	store := newFakeStore(self.id)
	e, err := NewEngine(b.Context(), store, acquainted(clock), self.priv, warpnet.MemberNode,
		append([]Option{WithClock(clock.Now), WithFlushInterval(time.Hour)}, opts...)...)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = e.Close() })
	return e, store, clock
}

// history seeds a peer with records from observers over the given hours.
func history(b *testing.B, store *fakeStore, peer identity, clock *fixedClock, observers, hours int) {
	b.Helper()
	for range observers {
		o := benchIdentity(b)
		for h := range hours {
			store.seed(signedRecord(o, peer.id, Network, bucketAt(clock.Now().Add(-time.Duration(h)*time.Hour)),
				genA, kindCount{KindRateLimitHit, 3}, kindCount{KindDialFailure, 1}))
		}
	}
}

// BenchmarkRecord is the cost every module pays per observation.
func BenchmarkRecord(b *testing.B) {
	e, _, _ := benchEngine(b)
	peers := make([]warpnet.WarpPeerID, 256)
	for i := range peers {
		peers[i] = benchIdentity(b).id
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = e.record(peers[i%len(peers)], KindRateLimitHit)
			i++
		}
	})
}

// BenchmarkObserveThroughListen is how many observations a second one
// fan-out can push through the engine without dropping any.
func BenchmarkObserveThroughListen(b *testing.B) {
	e, _, _ := benchEngine(b)
	peer := benchIdentity(b).id.String()
	events := make(chan warpnet.PeerEvent)
	e.Listen(events)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		events <- warpnet.PeerEvent{PeerID: peer, Type: warpnet.PeerBadSignature}
	}
}

// BenchmarkFlush signs and writes one record per dirty bucket.
func BenchmarkFlush(b *testing.B) {
	for _, dirty := range []int{10, 1000} {
		b.Run(fmt.Sprintf("dirty=%d", dirty), func(b *testing.B) {
			e, _, _ := benchEngine(b)
			peers := make([]warpnet.WarpPeerID, dirty)
			for i := range peers {
				peers[i] = benchIdentity(b).id
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				b.StopTimer()
				for _, p := range peers {
					_ = e.record(p, KindBadSignature)
				}
				b.StartTimer()
				if err := e.flush(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*dirty), "ns/record")
		})
	}
}

// BenchmarkScore is what a rating pass costs per peer: memoised, and
// recomputed after the peer's records changed.
func BenchmarkScore(b *testing.B) {
	for _, shape := range []struct{ observers, hours int }{{1, 1}, {10, 24}, {50, 96}} {
		name := fmt.Sprintf("observers=%d/hours=%d", shape.observers, shape.hours)
		b.Run("cached/"+name, func(b *testing.B) {
			e, store, clock := benchEngine(b)
			peer := benchIdentity(b)
			history(b, store, peer, clock, shape.observers, shape.hours)
			_ = e.Score(peer.id)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_ = e.Score(peer.id)
			}
		})
		b.Run("recomputed/"+name, func(b *testing.B) {
			e, store, clock := benchEngine(b)
			peer := benchIdentity(b)
			history(b, store, peer, clock, shape.observers, shape.hours)
			p, err := e.peer(peer.id.String())
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				p.set(slot{observer: "bench", dim: Network, bucket: bucket(i % 3), generation: genB}, nil)
				_ = e.Score(peer.id)
			}
		})
		b.Run("cold/"+name, func(b *testing.B) {
			e, store, clock := benchEngine(b)
			peer := benchIdentity(b)
			history(b, store, peer, clock, shape.observers, shape.hours)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				e.index.forget(peer.id.String())
				_ = e.Score(peer.id)
			}
		})
	}
}

// BenchmarkView is the public route's cost for a well-observed peer.
func BenchmarkView(b *testing.B) {
	e, store, clock := benchEngine(b)
	peer := benchIdentity(b)
	history(b, store, peer, clock, 50, 96)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := e.View(peer.id); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAuthenticate is the cost of every record merged from the network.
func BenchmarkAuthenticate(b *testing.B) {
	e, _, clock := benchEngine(b)
	observer := benchIdentity(b)
	rec := signedRecord(observer, benchIdentity(b).id, Network, bucketAt(clock.Now()), genA,
		kindCount{KindRateLimitHit, 3}, kindCount{KindBadSignature, 1})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := e.authenticate(rec); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRatePeers is one pass of the flush loop over a full index.
func BenchmarkRatePeers(b *testing.B) {
	for _, n := range []int{100, 5000} {
		b.Run(fmt.Sprintf("peers=%d", n), func(b *testing.B) {
			e, _, _ := benchEngine(b, WithRatings(NewPeersRatings()))
			for range n {
				_ = e.record(benchIdentity(b).id, KindDialFailure)
			}
			if err := e.flush(); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := e.ratePeers(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkPeersRatingsTier is the read gossipsub and the rate limiter do
// on every message and every stream.
func BenchmarkPeersRatingsTier(b *testing.B) {
	r := NewPeersRatings()
	peers := make([]warpnet.WarpPeerID, 1024)
	for i := range peers {
		peers[i] = benchIdentity(b).id
		r.Rate(peers[i], Tier(i%4))
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = r.GossipScore(peers[i%len(peers)])
			i++
		}
	})
}

// BenchmarkRatePeersAsTheFlushLoopDoes is a rating pass every flush
// interval over peers that others keep writing about: most passes find
// nothing new about most peers.
func BenchmarkRatePeersAsTheFlushLoopDoes(b *testing.B) {
	e, store, clock := benchEngine(b, WithRatings(NewPeersRatings()))
	peers := make([]identity, 100)
	for i := range peers {
		peers[i] = benchIdentity(b)
		history(b, store, peers[i], clock, 10, 12)
		_ = e.Score(peers[i].id)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		clock.advance(defaultFlushInterval)
		if err := e.ratePeers(); err != nil {
			b.Fatal(err)
		}
	}
}

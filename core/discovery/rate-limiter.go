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

package discovery

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

// perPeerCapacity is how many discovery events one peer may spend in
// the shared budget before it is throttled on its own. The global
// bucket alone could not tell "twelve new peers" from "one peer twelve
// times", so a single chatty gossiper starved discovery for everyone.
const (
	perPeerCapacity      = 4
	perPeerLeakPer10Sec  = 1
	perPeerCacheSize     = 1024
	perPeerCacheLifetime = 10 * time.Minute
)

type leakyBucketRateLimiter struct {
	capacity     *atomic.Int64
	remaining    *atomic.Int64
	lastLeak     *atomic.Int64
	leakInterval time.Duration
}

func newRateLimiter(capacity int, leakPer10Sec int) *leakyBucketRateLimiter {
	if leakPer10Sec <= 0 {
		leakPer10Sec = 1
	}
	atomicCap := new(atomic.Int64)
	atomicCap.Store(int64(capacity))
	atomicRemain := new(atomic.Int64)
	atomicRemain.Store(0)
	atomicLastLeak := new(atomic.Int64)
	atomicLastLeak.Store(time.Now().UnixMilli())
	return &leakyBucketRateLimiter{
		capacity:     atomicCap,
		remaining:    atomicRemain,
		leakInterval: (time.Second * 10) / time.Duration(leakPer10Sec),
		lastLeak:     atomicLastLeak,
	}
}

func (b *leakyBucketRateLimiter) Allow() bool {
	now := time.Now().UnixMilli()
	elapsed := now - b.lastLeak.Load()
	leaks := elapsed / b.leakInterval.Milliseconds()
	if leaks > 0 {
		b.remaining.Add(-leaks)
		if b.remaining.Load() < 0 {
			b.remaining.Store(0)
		}
		b.lastLeak.Store(b.lastLeak.Load() + leaks*b.leakInterval.Milliseconds())
	}

	rem := b.remaining.Load()
	if rem < b.capacity.Load() {
		b.remaining.Add(1)
		return true
	}

	return false
}

// peerLimiter throttles discovery per source peer on top of the global
// budget, and scales a peer's allowance down with its standing so an
// offender's entries are the first dropped under pressure.
type peerLimiter struct {
	mx      sync.Mutex
	buckets *expirable.LRU[string, *leakyBucketRateLimiter]
	band    func(warpnet.WarpPeerID) rating.Band
}

func newPeerLimiter(band func(warpnet.WarpPeerID) rating.Band) *peerLimiter {
	return &peerLimiter{
		buckets: expirable.NewLRU[string, *leakyBucketRateLimiter](
			perPeerCacheSize, nil, perPeerCacheLifetime,
		),
		band: band,
	}
}

func (p *peerLimiter) Allow(id warpnet.WarpPeerID) bool {
	if p == nil {
		return true
	}
	key := id.String()

	p.mx.Lock()
	bucket, ok := p.buckets.Get(key)
	if !ok {
		capacity := perPeerCapacity
		if p.band != nil {
			scaled := int(float64(capacity) * rating.LimitMultiplier(p.band(id)))
			if scaled < 1 {
				scaled = 1
			}
			capacity = scaled
		}
		bucket = newRateLimiter(capacity, perPeerLeakPer10Sec)
		p.buckets.Add(key, bucket)
	}
	p.mx.Unlock()

	return bucket.Allow()
}

func (p *peerLimiter) Close() {
	if p == nil || p.buckets == nil {
		return
	}
	p.buckets.Purge()
}

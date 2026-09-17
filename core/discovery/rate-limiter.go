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

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

const (
	maxIPBuckets = 4096
	ipBucketTTL  = time.Minute * 5
)

// ipRateLimiter keeps a leaky bucket per remote IP, so that a peer flooding
// discoveries cannot shed the peers everyone else announces.
type ipRateLimiter struct {
	buckets      *expirable.LRU[string, *leakyBucketRateLimiter]
	capacity     int
	leakPer10Sec int
	mx           sync.Mutex
}

func newIPRateLimiter(capacity int, leakPer10Sec int) *ipRateLimiter {
	return &ipRateLimiter{
		buckets:      expirable.NewLRU[string, *leakyBucketRateLimiter](maxIPBuckets, nil, ipBucketTTL),
		capacity:     capacity,
		leakPer10Sec: leakPer10Sec,
	}
}

// allow charges the bucket of the IP the peer is reachable at. Peers announced
// without an address share a single bucket.
func (l *ipRateLimiter) allow(addrs []warpnet.WarpAddress) bool {
	var ip string
	for _, addr := range addrs {
		if parsed := warpnet.MultiAddressIP(addr); parsed != nil {
			ip = parsed.String()
			break
		}
	}
	return l.bucket(ip).allow()
}

func (l *ipRateLimiter) bucket(ip string) *leakyBucketRateLimiter {
	l.mx.Lock()
	defer l.mx.Unlock()

	bucket, ok := l.buckets.Get(ip)
	if !ok {
		bucket = newRateLimiter(l.capacity, l.leakPer10Sec)
		l.buckets.Add(ip, bucket)
	}
	return bucket
}

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

func (b *leakyBucketRateLimiter) allow() bool {
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

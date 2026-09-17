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

package ratelimit

import (
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

type Limit struct {
	Burst    int64
	Interval time.Duration
}

func PerMinute(burst, perMinute int64) Limit {
	return Limit{Burst: max(1, burst), Interval: time.Minute / time.Duration(max(1, perMinute))}
}

func PerTenSeconds(burst, perTenSec int64) Limit {
	return Limit{Burst: max(1, burst), Interval: 10 * time.Second / time.Duration(max(1, perTenSec))}
}

// MultipliedBy is what a caller on this multiplier may spend of a limit, never
// below one: a peer the rating thinks little of is slowed down, never starved.
func (l Limit) MultipliedBy(multiplier float64) Limit {
	if multiplier >= 1 {
		return l
	}
	return Limit{
		Burst:    max(1, int64(float64(l.Burst)*multiplier)),
		Interval: time.Duration(max(1, int64(float64(l.Interval)/multiplier))),
	}
}

func (l Limit) PerMinute() int64 {
	if l.Interval <= 0 {
		return 0
	}
	return int64(time.Minute / l.Interval)
}

type Bucket struct {
	mx       sync.Mutex
	limit    Limit
	filled   int64
	lastLeak time.Time
}

func NewBucket(l Limit) *Bucket {
	if l.Burst <= 0 {
		l.Burst = 1
	}
	if l.Interval <= 0 {
		l.Interval = time.Minute
	}
	return &Bucket{limit: l, lastLeak: time.Now()}
}

func (b *Bucket) Allow() bool {
	b.mx.Lock()
	defer b.mx.Unlock()

	if leaks := int64(time.Since(b.lastLeak) / b.limit.Interval); leaks > 0 {
		b.filled -= leaks
		if b.filled < 0 {
			b.filled = 0
		}
		b.lastLeak = b.lastLeak.Add(time.Duration(leaks) * b.limit.Interval)
	}

	if b.filled >= b.limit.Burst {
		return false
	}
	b.filled++
	return true
}

// Buckets is one leaky bucket per caller, dropped once a caller goes quiet
// for the whole TTL.
type Buckets struct {
	mx      sync.Mutex
	buckets *lru.LRU[string, *Bucket]
}

func NewBuckets(size int, ttl time.Duration) *Buckets {
	return &Buckets{buckets: lru.NewLRU[string, *Bucket](size, nil, ttl)}
}

func (b *Buckets) Allow(key string, l Limit) bool {
	if b == nil || b.buckets == nil {
		return true
	}

	b.mx.Lock()
	bucket, ok := b.buckets.Get(key)
	if !ok {
		bucket = NewBucket(l)
		b.buckets.Add(key, bucket)
	}
	b.mx.Unlock()

	return bucket.Allow()
}

func (b *Buckets) Close() {
	if b == nil || b.buckets == nil {
		return
	}

	b.mx.Lock()
	defer b.mx.Unlock()

	b.buckets.Purge()
	CloseExpirableLRU(b.buckets)
}

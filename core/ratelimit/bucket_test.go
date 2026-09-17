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
package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBucketAdmitsBurstThenLeaks(t *testing.T) {
	b := NewBucket(PerMinute(3, 60_000))

	for i := range 3 {
		if !b.Allow() {
			t.Fatalf("request %d of the burst must be admitted", i+1)
		}
	}
	if b.Allow() {
		t.Fatal("expected the request past the burst to be refused")
	}

	time.Sleep(5 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("expected the bucket to admit again after leaking")
	}
}

func TestBucketZeroLimitFallsBackToOne(t *testing.T) {
	b := NewBucket(Limit{})
	assert.True(t, b.Allow(), "the first request must be admitted")
	assert.False(t, b.Allow(), "the second request must be refused")
}

func TestPerTenSecondsLeaksOverTime(t *testing.T) {
	b := NewBucket(PerTenSeconds(2, 100))
	assert.True(t, b.Allow())
	assert.True(t, b.Allow())
	assert.False(t, b.Allow(), "capacity is spent")

	b.lastLeak = b.lastLeak.Add(-time.Second * 11)
	assert.True(t, b.Allow(), "the bucket leaks with time")
}

func TestPerTenSecondsGuardsAZeroRate(t *testing.T) {
	assert.Equal(t, time.Second*10, PerTenSeconds(10, 0).Interval)
	assert.Equal(t, time.Second*10, PerTenSeconds(10, -5).Interval)
	assert.Equal(t, int64(1), PerTenSeconds(0, 1).Burst)
}

func TestLimitMultipliedBy(t *testing.T) {
	limit := PerMinute(60, 300)

	assert.Equal(t, limit, limit.MultipliedBy(1), "a full share leaves the limit alone")
	assert.Equal(t, limit, limit.MultipliedBy(2))

	half := limit.MultipliedBy(0.5)
	assert.Equal(t, int64(30), half.Burst)
	assert.Equal(t, int64(150), half.PerMinute())

	starved := PerMinute(1, 1).MultipliedBy(0.01)
	assert.Equal(t, int64(1), starved.Burst, "a peer is slowed down, never starved")
}

func TestBucketsAreKeptPerKey(t *testing.T) {
	buckets := NewBuckets(16, time.Minute)
	t.Cleanup(buckets.Close)

	limit := PerMinute(2, 60)
	assert.True(t, buckets.Allow("one", limit))
	assert.True(t, buckets.Allow("one", limit))
	assert.False(t, buckets.Allow("one", limit), "a spent key is limited")
	assert.True(t, buckets.Allow("two", limit), "one key's bucket must not limit another")
}

func TestBucketsAreNilSafe(t *testing.T) {
	var buckets *Buckets
	require.True(t, buckets.Allow("key", PerMinute(1, 1)))
	require.NotPanics(t, buckets.Close)
}

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

package middleware

import (
	"bytes"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

// newCacheForTest creates a cache and registers Close so the library's
// background goroutine doesn't outlive the test.
func newCacheForTest(t *testing.T, ttl time.Duration) *idempotencyCache {
	t.Helper()
	c := newIdempotencyCache(ttl)
	t.Cleanup(c.Close)
	return c
}

func TestIdempotencyCache_HitReturnsCachedResponse(t *testing.T) {
	c := newCacheForTest(t, time.Minute)
	key := idempotencyKey("/private/post/tweet/0.0.0", "peer-1", "msg-1")
	resp := []byte(`{"id":"abc"}`)

	if _, ok := c.get(key); ok {
		t.Fatal("expected cache miss before set")
	}
	c.set(key, resp)

	got, ok := c.get(key)
	if !ok {
		t.Fatal("expected cache hit after set")
	}
	if !bytes.Equal(got, resp) {
		t.Fatalf("expected %s, got %s", resp, got)
	}
}

func TestIdempotencyCache_DistinctKeysIsolated(t *testing.T) {
	c := newCacheForTest(t, time.Minute)
	c.set(idempotencyKey("/private/post/tweet/0.0.0", "peer-1", "msg-1"), []byte("a"))
	c.set(idempotencyKey("/private/post/tweet/0.0.0", "peer-1", "msg-2"), []byte("b"))

	if v, _ := c.get(idempotencyKey("/private/post/tweet/0.0.0", "peer-1", "msg-1")); !bytes.Equal(v, []byte("a")) {
		t.Fatalf("unexpected value for msg-1: %s", v)
	}
	if v, _ := c.get(idempotencyKey("/private/post/tweet/0.0.0", "peer-1", "msg-2")); !bytes.Equal(v, []byte("b")) {
		t.Fatalf("unexpected value for msg-2: %s", v)
	}
	if _, ok := c.get(idempotencyKey("/public/post/like/0.0.0", "peer-1", "msg-1")); ok {
		t.Fatal("expected miss for different protocol")
	}
	if _, ok := c.get(idempotencyKey("/private/post/tweet/0.0.0", "peer-2", "msg-1")); ok {
		t.Fatal("expected miss for different peer (cross-peer isolation)")
	}
}

func TestIdempotencyCache_Expires(t *testing.T) {
	c := newCacheForTest(t, 20*time.Millisecond)
	key := idempotencyKey("/private/post/tweet/0.0.0", "peer-1", "msg-1")
	c.set(key, []byte("payload"))

	if _, ok := c.get(key); !ok {
		t.Fatal("expected hit before expiration")
	}

	time.Sleep(60 * time.Millisecond)
	if _, ok := c.get(key); ok {
		t.Fatal("expected miss after expiration")
	}
}

func TestIdempotencyCache_EmptyResponseNotStored(t *testing.T) {
	c := newCacheForTest(t, time.Minute)
	key := idempotencyKey("/private/post/tweet/0.0.0", "peer-1", "msg-1")
	c.set(key, nil)
	if _, ok := c.get(key); ok {
		t.Fatal("expected nil response not to be cached")
	}
}

func TestIsIdempotencyApplicable(t *testing.T) {
	cases := map[string]bool{
		"/private/post/tweet/0.0.0":   true,
		"/public/post/like/0.0.0":     true,
		"/public/get/tweet/0.0.0":     false,
		"/private/delete/tweet/0.0.0": false,
	}
	for path, want := range cases {
		if got := isIdempotencyApplicable(path); got != want {
			t.Fatalf("%s: expected %v, got %v", path, want, got)
		}
	}
}

func TestIdempotencyCache_CloseStopsLibraryGoroutine(t *testing.T) {
	c := newIdempotencyCache(time.Minute)
	// This test owns Close() — don't register cleanup.

	// Reach into the library struct and verify the `done` channel is open
	// before Close, then closed afterwards. If the field disappears in a
	// future library version, the test makes the regression explicit.
	field := reflect.ValueOf(c.cache).Elem().FieldByName("done")
	if !field.IsValid() || field.Kind() != reflect.Chan {
		t.Fatal("expirable LRU layout changed: missing `done` chan")
	}
	doneCh := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface().(chan struct{})

	select {
	case <-doneCh:
		t.Fatal("done channel unexpectedly closed before Close()")
	default:
	}

	c.Close()

	select {
	case <-doneCh:
	case <-time.After(time.Second):
		t.Fatal("done channel was not closed after Close()")
	}

	// Idempotent: a second Close must not panic.
	c.Close()
}

func TestIdempotencyCache_SetCopiesPayload(t *testing.T) {
	c := newCacheForTest(t, time.Minute)
	key := idempotencyKey("/private/post/tweet/0.0.0", "peer-1", "msg-1")
	payload := []byte("original")
	c.set(key, payload)
	payload[0] = 'X'

	got, ok := c.get(key)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if !bytes.Equal(got, []byte("original")) {
		t.Fatalf("cache should not be mutated by caller: got %s", got)
	}
}

// TestIdempotencyCache_DoCollapsesConcurrent verifies that concurrent
// callers with the same key share a single compute invocation. Uses a
// deterministic barrier on the cache's follower count so the test does
// not depend on wall-clock timing.
func TestIdempotencyCache_DoCollapsesConcurrent(t *testing.T) {
	c := newCacheForTest(t, time.Minute)
	key := idempotencyKey("/private/post/tweet/0.0.0", "peer-1", "msg-1")

	var calls atomic.Int32
	release := make(chan struct{})
	compute := func() ([]byte, bool, error) { //nolint:unparam // signature matches do(); error path covered elsewhere
		calls.Add(1)
		<-release // hold the leader inside compute until all followers are queued
		return []byte("payload-1"), true, nil
	}

	const N = 8
	var wg sync.WaitGroup
	results := make([][]byte, N)
	for i := range N {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			payload, err := c.do(key, compute)
			if err != nil {
				t.Errorf("do err: %v", err)
				return
			}
			results[i] = payload
		}(i)
	}

	// Wait until all N-1 followers have registered on the leader's
	// inflight call. Polling the cache's own bookkeeping makes this
	// barrier deterministic instead of relying on a fixed sleep.
	deadline := time.Now().Add(2 * time.Second)
	for c.followerCount(key) < N-1 {
		if time.Now().After(deadline) {
			t.Fatalf("only %d/%d followers registered before deadline",
				c.followerCount(key), N-1)
		}
		runtime.Gosched()
	}
	close(release)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected compute to run exactly once, ran %d times", got)
	}
	for i, r := range results {
		if !bytes.Equal(r, []byte("payload-1")) {
			t.Fatalf("caller %d got unexpected payload: %s", i, r)
		}
	}
}

// TestIdempotencyCache_GetReturnsCopy ensures callers cannot mutate the
// cached value through the returned slice.
func TestIdempotencyCache_GetReturnsCopy(t *testing.T) {
	c := newCacheForTest(t, time.Minute)
	key := idempotencyKey("/private/post/tweet/0.0.0", "peer-1", "msg-1")
	c.set(key, []byte("payload"))

	got, ok := c.get(key)
	if !ok {
		t.Fatal("expected cache hit")
	}
	got[0] = 'X'

	again, _ := c.get(key)
	if !bytes.Equal(again, []byte("payload")) {
		t.Fatalf("cache mutated via returned slice: got %s", again)
	}
}

// TestIdempotencyCache_DropsOversizedPayload ensures payloads larger than
// idempotencyMaxPayloadBytes are not cached, bounding memory usage.
func TestIdempotencyCache_DropsOversizedPayload(t *testing.T) {
	c := newCacheForTest(t, time.Minute)
	key := idempotencyKey("/private/post/tweet/0.0.0", "peer-1", "msg-1")
	big := make([]byte, idempotencyMaxPayloadBytes+1)
	c.set(key, big)
	if _, ok := c.get(key); ok {
		t.Fatal("oversized payload should not be cached")
	}

	// Boundary: exactly maxPayloadBytes is still cached.
	atLimit := make([]byte, idempotencyMaxPayloadBytes)
	c.set(key, atLimit)
	if _, ok := c.get(key); !ok {
		t.Fatal("payload at exact size limit should be cached")
	}
}

// TestIdempotencyCache_DoSkipsCacheWhenNotCacheable verifies that error
// responses are still returned to callers but not stored in the cache.
func TestIdempotencyCache_DoSkipsCacheWhenNotCacheable(t *testing.T) {
	c := newCacheForTest(t, time.Minute)
	key := idempotencyKey("/private/post/tweet/0.0.0", "peer-1", "msg-1")

	payload, err := c.do(key, func() ([]byte, bool, error) {
		return []byte(`{"error":"boom"}`), false, nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !bytes.Equal(payload, []byte(`{"error":"boom"}`)) {
		t.Fatalf("unexpected payload: %s", payload)
	}
	if _, ok := c.get(key); ok {
		t.Fatal("error response should not be cached")
	}
}

// TestIdempotencyCache_DoReturnsCachedOnHit verifies the fast path.
func TestIdempotencyCache_DoReturnsCachedOnHit(t *testing.T) {
	c := newCacheForTest(t, time.Minute)
	key := idempotencyKey("/private/post/tweet/0.0.0", "peer-1", "msg-1")
	c.set(key, []byte("cached"))

	called := false
	payload, err := c.do(key, func() ([]byte, bool, error) {
		called = true
		return []byte("ignored"), true, nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if called {
		t.Fatal("compute should not run on cache hit")
	}
	if !bytes.Equal(payload, []byte("cached")) {
		t.Fatalf("expected cached payload, got %s", payload)
	}
}

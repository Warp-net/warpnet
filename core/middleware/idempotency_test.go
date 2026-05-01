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
	"testing"
	"time"
)

func newTestCache(ttl time.Duration, now func() time.Time) *idempotencyCache {
	c := &idempotencyCache{
		entries: make(map[string]idempotencyEntry),
		ttl:     ttl,
		now:     now,
	}
	return c
}

func TestIdempotencyCache_HitReturnsCachedResponse(t *testing.T) {
	c := newTestCache(time.Minute, time.Now)
	key := idempotencyKey("/private/post/tweet/0.0.0", "msg-1")
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
	c := newTestCache(time.Minute, time.Now)
	c.set(idempotencyKey("/private/post/tweet/0.0.0", "msg-1"), []byte("a"))
	c.set(idempotencyKey("/private/post/tweet/0.0.0", "msg-2"), []byte("b"))

	if v, _ := c.get(idempotencyKey("/private/post/tweet/0.0.0", "msg-1")); !bytes.Equal(v, []byte("a")) {
		t.Fatalf("unexpected value for msg-1: %s", v)
	}
	if v, _ := c.get(idempotencyKey("/private/post/tweet/0.0.0", "msg-2")); !bytes.Equal(v, []byte("b")) {
		t.Fatalf("unexpected value for msg-2: %s", v)
	}
	if _, ok := c.get(idempotencyKey("/public/post/like/0.0.0", "msg-1")); ok {
		t.Fatal("expected miss for different protocol")
	}
}

func TestIdempotencyCache_Expires(t *testing.T) {
	current := time.Unix(0, 0)
	c := newTestCache(time.Minute, func() time.Time { return current })
	key := idempotencyKey("/private/post/tweet/0.0.0", "msg-1")
	c.set(key, []byte("payload"))

	current = current.Add(30 * time.Second)
	if _, ok := c.get(key); !ok {
		t.Fatal("expected hit before expiration")
	}

	current = current.Add(2 * time.Minute)
	if _, ok := c.get(key); ok {
		t.Fatal("expected miss after expiration")
	}
}

func TestIdempotencyCache_EmptyResponseNotStored(t *testing.T) {
	c := newTestCache(time.Minute, time.Now)
	key := idempotencyKey("/private/post/tweet/0.0.0", "msg-1")
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

func TestIdempotencyCache_SetCopiesPayload(t *testing.T) {
	c := newTestCache(time.Minute, time.Now)
	key := idempotencyKey("/private/post/tweet/0.0.0", "msg-1")
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

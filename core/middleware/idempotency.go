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
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
	log "github.com/sirupsen/logrus"
)

const (
	idempotencyTTL = 10 * time.Minute
	// idempotencySize bounds the cache by entry count. Combined with
	// idempotencyMaxPayloadBytes below, worst-case memory is bounded
	// at ~(size * maxPayloadBytes).
	idempotencySize = 1024
	// idempotencyMaxPayloadBytes drops oversized payloads instead of
	// caching them. Entry-count LRU alone offers no protection against
	// a few large responses dominating memory; this guard keeps the
	// cache cheap.
	idempotencyMaxPayloadBytes = 64 * 1024 // 64 KiB
)

// idempotencyCache stores responses keyed by (protocol + peer + message id)
// so that duplicate POST requests retried by clients (double-clicks, network
// retries) return the original response without re-executing the side effect.
// It also collapses concurrent same-key requests via an in-flight wait map,
// so simultaneous retries share a single handler invocation.
type idempotencyCache struct {
	cache  *lru.LRU[string, []byte]
	closed sync.Once

	inflightMu sync.Mutex
	inflight   map[string]*inflightCall
}

// inflightCall is the shared rendezvous for concurrent callers waiting on
// the same idempotency key. The leader runs the compute function; followers
// wait on `done` and read `payload` / `err`. `followers` is incremented
// while holding inflightMu and is used only for testing/observability.
type inflightCall struct {
	done      chan struct{}
	payload   []byte
	err       error
	followers int
}

func newIdempotencyCache(ttl time.Duration) *idempotencyCache {
	c := &idempotencyCache{
		cache:    lru.NewLRU[string, []byte](idempotencySize, nil, ttl),
		inflight: make(map[string]*inflightCall),
	}
	// The library spawns a deleteExpired goroutine whose `done` channel is
	// never closed by its public API, so the goroutine would normally outlive
	// the cache. Set a finalizer that closes the channel via reflect+unsafe
	// when the wrapper becomes unreachable, so the goroutine exits and the
	// underlying LRU can be GC'd. The library goroutine holds the LRU
	// strongly but does not reference this wrapper, so the wrapper itself
	// remains finalizable independently.
	runtime.SetFinalizer(c, func(c *idempotencyCache) { c.Close() })
	return c
}

// get returns a defensive copy so callers can't mutate the cached value.
func (c *idempotencyCache) get(key string) ([]byte, bool) {
	v, ok := c.cache.Get(key)
	if !ok {
		return nil, false
	}
	return cloneBytes(v), true
}

// set stores a defensive copy of the payload. Empty payloads and payloads
// larger than idempotencyMaxPayloadBytes are dropped to bound memory.
func (c *idempotencyCache) set(key string, response []byte) {
	if len(response) == 0 || len(response) > idempotencyMaxPayloadBytes {
		return
	}
	c.cache.Add(key, cloneBytes(response))
}

func cloneBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp
}

// do returns the cached payload for `key` if present; otherwise it runs
// `compute` exactly once for any set of concurrent callers sharing the same
// key. Followers wait for the leader and receive a fresh copy of the
// payload. The returned payload is stored in the cache only when compute
// reports it cacheable (so error responses don't poison the cache) and
// when its size is within idempotencyMaxPayloadBytes.
func (c *idempotencyCache) do(
	key string,
	compute func() (payload []byte, cacheable bool, err error),
) ([]byte, error) {
	if v, ok := c.get(key); ok {
		log.Debugf("middleware: idempotent replay (cache hit) for %s", key)
		return v, nil
	}

	c.inflightMu.Lock()
	if call, ok := c.inflight[key]; ok {
		call.followers++
		c.inflightMu.Unlock()
		<-call.done
		log.Debugf("middleware: idempotent replay (in-flight follower) for %s", key)
		return cloneBytes(call.payload), call.err
	}
	call := &inflightCall{done: make(chan struct{})}
	c.inflight[key] = call
	c.inflightMu.Unlock()

	defer func() {
		c.inflightMu.Lock()
		delete(c.inflight, key)
		c.inflightMu.Unlock()
		close(call.done)
	}()

	// Re-check the cache under leadership: a previous leader may have
	// completed and populated it between our miss and our claim.
	if v, ok := c.get(key); ok {
		call.payload = cloneBytes(v) // owned copy for any racing followers
		return v, nil
	}

	payload, cacheable, err := compute()
	// Take an owned copy of the leader's payload before publishing it via
	// `call.payload`, so handler-owned slices can't be mutated under
	// followers after the leader returns.
	call.payload = cloneBytes(payload)
	call.err = err
	if err == nil && cacheable {
		c.set(key, payload)
	}
	return payload, err
}

// Close stops the library's background deleteExpired goroutine by closing
// its unexported `done` channel via reflect+unsafe. Safe to call multiple
// times. No-op if the library's internal layout changes.
func (c *idempotencyCache) Close() {
	c.closed.Do(func() {
		closeExpirableLRU(c.cache)
	})
}

func closeExpirableLRU(cache any) {
	defer func() {
		if r := recover(); r != nil {
			log.Debugf("middleware: idempotency: closeExpirableLRU recovered: %v", r)
		}
	}()
	v := reflect.ValueOf(cache)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return
	}
	field := v.Elem().FieldByName("done")
	if !field.IsValid() || field.Kind() != reflect.Chan {
		return
	}
	// FieldByName on an unexported field returns a Value flagged as
	// read-only, so reflect.Value.Close() would panic. Rebuild a settable
	// Value pointing at the same memory to bypass the export check.
	//#nosec G103 // intentional: bypass reflect's exported-field check to close the library's `done` chan
	settable := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	// Closing an already-closed channel panics; rely on the recover above.
	settable.Close()
}

// isIdempotencyApplicable reports whether the given protocol path is a POST
// route that should be guarded by the idempotency cache.
func isIdempotencyApplicable(protocol string) bool {
	return strings.Contains(protocol, "/post/")
}

func idempotencyKey(protocol, peerID, messageID string) string {
	return protocol + "|" + peerID + "|" + messageID
}

// followerCount reports the number of in-flight followers waiting on the
// leader for `key` (excluding the leader itself). Used by tests to build
// a deterministic barrier without time.Sleep.
func (c *idempotencyCache) followerCount(key string) int {
	c.inflightMu.Lock()
	defer c.inflightMu.Unlock()
	call, ok := c.inflight[key]
	if !ok {
		return 0
	}
	return call.followers
}

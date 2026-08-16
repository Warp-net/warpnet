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

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
	log "github.com/sirupsen/logrus"
)

const (
	idempotencyTTL             = 10 * time.Minute
	idempotencySize            = 1024
	idempotencyMaxPayloadBytes = 64 * 1024 // 64 KiB
)

// IdempotencyMiddleware deduplicates POST requests retried with the same
// message id (double-clicks, network retries): the first request runs
// downstream, replays are answered from the cache without re-executing the
// side effect, and concurrent same-key requests share a single downstream
// invocation. The handler's return value is memoized as-is — whoever ends
// the chain decides how it goes out on the wire.
func (p *WarpMiddleware) IdempotencyMiddleware(next warpnet.WarpHandlerFunc) warpnet.WarpHandlerFunc {
	return func(data []byte, s warpnet.WarpStream) (any, error) {
		typedStream, ok := s.(*warpnet.WarpStreamBody)
		if !ok || p.idempotency == nil || typedStream.MessageId == "" ||
			!isIdempotencyApplicable(string(s.Protocol())) {
			return next(data, s)
		}

		// Scope the key by authenticated remote peer so two peers
		// can't collide on the same message id within the TTL window.
		var peerID string
		if conn := s.Conn(); conn != nil {
			peerID = conn.RemotePeer().String()
		}
		cacheKey := idempotencyKey(string(s.Protocol()), peerID, typedStream.MessageId)

		return p.idempotency.do(cacheKey, func() (any, bool, error) {
			response, err := next(data, s)
			return response, isCacheableResponse(response, err), err
		})
	}
}

// isCacheableResponse reports whether a reply may be replayed for a retry:
// failures, empty replies and error envelopes must not be pinned in the
// cache for the whole TTL.
func isCacheableResponse(response any, err error) bool {
	if err != nil || response == nil {
		return false
	}
	if _, ok := response.(event.ResponseError); ok {
		return false
	}
	return true
}

type idempotencyCache struct {
	cache  *lru.LRU[string, any]
	closed sync.Once

	inflightMu sync.Mutex
	inflight   map[string]*inflightCall
}

type inflightCall struct {
	done      chan struct{}
	response  any
	err       error
	followers int
}

func newIdempotencyCache(ttl time.Duration) *idempotencyCache {
	c := &idempotencyCache{
		cache:    lru.NewLRU[string, any](idempotencySize, nil, ttl),
		inflight: make(map[string]*inflightCall),
	}
	runtime.SetFinalizer(c, func(c *idempotencyCache) { c.Close() })
	return c
}

func (c *idempotencyCache) get(key string) (any, bool) {
	v, ok := c.cache.Get(key)
	if !ok {
		return nil, false
	}
	return cloneResponse(v), true
}

// set stores a reply for replays. Byte and string replies larger than
// idempotencyMaxPayloadBytes are dropped to bound memory.
func (c *idempotencyCache) set(key string, response any) {
	switch typed := response.(type) {
	case nil:
		return
	case []byte:
		if len(typed) == 0 || len(typed) > idempotencyMaxPayloadBytes {
			return
		}
	case string:
		if typed == "" || len(typed) > idempotencyMaxPayloadBytes {
			return
		}
	}
	c.cache.Add(key, cloneResponse(response))
}

// cloneResponse copies byte slices so neither cache nor callers can mutate
// each other's data; other reply kinds are treated as read-only values.
func cloneResponse(response any) any {
	if bt, ok := response.([]byte); ok {
		return cloneBytes(bt)
	}
	return response
}

func cloneBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp
}

func (c *idempotencyCache) do(
	key string,
	compute func() (response any, cacheable bool, err error),
) (any, error) {
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
		return cloneResponse(call.response), call.err
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
		call.response = cloneResponse(v) // owned copy for any racing followers
		return v, nil
	}

	response, cacheable, err := compute()
	// Publish an owned copy, so handler-owned slices can't be mutated under
	// followers after the leader returns.
	call.response = cloneResponse(response)
	call.err = err
	if err == nil && cacheable {
		c.set(key, response)
	}
	return response, err
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

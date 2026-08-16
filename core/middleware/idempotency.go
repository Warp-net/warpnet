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
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
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
// downstream and its reply is cached, replays are answered from the cache
// without re-executing the side effect, and concurrent same-key requests
// share a single downstream invocation.
func (p *WarpMiddleware) IdempotencyMiddleware(next warpnet.StreamHandler) warpnet.StreamHandler {
	return func(s warpnet.WarpStream) {
		typedStream, ok := s.(*warpnet.WarpStreamBody)
		if !ok || p.idempotency == nil || typedStream.MessageId == "" ||
			!isIdempotencyApplicable(string(s.Protocol())) {
			next(s)
			return
		}

		// Downstream only writes to the recorder, so the reply to this
		// stream (and its closing) is owned here.
		defer func() {
			_ = s.Close()
		}()

		// Scope the key by authenticated remote peer so two peers
		// can't collide on the same message id within the TTL window.
		var peerID string
		if conn := s.Conn(); conn != nil {
			peerID = conn.RemotePeer().String()
		}
		cacheKey := idempotencyKey(string(s.Protocol()), peerID, typedStream.MessageId)

		payload, err := p.idempotency.do(cacheKey, func() ([]byte, bool, error) {
			recorder := &responseRecorder{WarpStream: typedStream.WarpStream}
			next(&warpnet.WarpStreamBody{
				WarpStream: recorder,
				Body:       typedStream.Body,
				MessageId:  typedStream.MessageId,
			})
			response := recorder.buf.Bytes()
			return response, isCacheableResponse(response), nil
		})
		if err != nil {
			log.Errorf("middleware: idempotency: %v", err)
		}
		if len(payload) == 0 {
			return
		}
		if _, werr := s.Write(payload); werr != nil {
			log.Errorf("middleware: idempotency: writing response to stream: %v", werr)
		}
	}
}

// responseRecorder captures downstream writes instead of forwarding them, so
// the recorded payload can be both cached and written back by the middleware.
// Close is swallowed too: the wrapped stream must stay writable after the
// downstream handler finishes.
type responseRecorder struct {
	warpnet.WarpStream

	buf bytes.Buffer
}

func (r *responseRecorder) Write(p []byte) (int, error) { return r.buf.Write(p) }
func (r *responseRecorder) Close() error                { return nil }
func (r *responseRecorder) CloseWrite() error           { return nil }

// isCacheableResponse reports whether payload is a successful reply. Error
// envelopes (non-zero code, or the empty-response fallback) must not be
// pinned in the cache: a transient failure would otherwise replay for the
// whole TTL.
func isCacheableResponse(payload []byte) bool {
	if len(payload) == 0 {
		return false
	}
	var respErr event.ResponseError
	if err := json.Unmarshal(payload, &respErr); err != nil {
		return true // not an error envelope — a raw or custom payload
	}
	return respErr.Code == 0 && respErr.Message != EmptyResponseMessage
}

type idempotencyCache struct {
	cache  *lru.LRU[string, []byte]
	closed sync.Once

	inflightMu sync.Mutex
	inflight   map[string]*inflightCall
}

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
	runtime.SetFinalizer(c, func(c *idempotencyCache) { c.Close() })
	return c
}

func (c *idempotencyCache) get(key string) ([]byte, bool) {
	v, ok := c.cache.Get(key)
	if !ok {
		return nil, false
	}
	return cloneBytes(v), true
}

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

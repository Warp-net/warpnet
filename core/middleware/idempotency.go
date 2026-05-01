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
	idempotencyTTL  = 10 * time.Minute
	idempotencySize = 4096
)

// idempotencyCache stores responses keyed by (protocol + peer + message id)
// so that duplicate POST requests retried by clients (double-clicks, network
// retries) return the original response without re-executing the side effect.
type idempotencyCache struct {
	cache  *lru.LRU[string, []byte]
	closed sync.Once
}

func newIdempotencyCache(ttl time.Duration) *idempotencyCache {
	c := &idempotencyCache{
		cache: lru.NewLRU[string, []byte](idempotencySize, nil, ttl),
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

func (c *idempotencyCache) get(key string) ([]byte, bool) {
	return c.cache.Get(key)
}

func (c *idempotencyCache) set(key string, response []byte) {
	if len(response) == 0 {
		return
	}
	cp := make([]byte, len(response))
	copy(cp, response)
	c.cache.Add(key, cp)
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

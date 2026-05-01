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
	"strings"
	"sync"
	"time"
)

const (
	idempotencyTTL             = 10 * time.Minute
	idempotencyJanitorInterval = time.Minute
)

type idempotencyEntry struct {
	response  []byte
	expiresAt time.Time
}

// idempotencyCache stores responses keyed by (protocol + message id) so that
// duplicate POST requests retried by clients (double-clicks, network retries)
// return the original response without re-executing the side effect.
type idempotencyCache struct {
	mu      sync.Mutex
	entries map[string]idempotencyEntry
	ttl     time.Duration
	now     func() time.Time
}

func newIdempotencyCache(ttl time.Duration) *idempotencyCache {
	c := &idempotencyCache{
		entries: make(map[string]idempotencyEntry),
		ttl:     ttl,
		now:     time.Now,
	}
	go c.janitor()
	return c
}

func (c *idempotencyCache) janitor() {
	ticker := time.NewTicker(idempotencyJanitorInterval)
	defer ticker.Stop()
	for range ticker.C {
		c.evictExpired()
	}
}

func (c *idempotencyCache) evictExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for k, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, k)
		}
	}
}

func (c *idempotencyCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if c.now().After(e.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return e.response, true
}

func (c *idempotencyCache) set(key string, response []byte) {
	if len(response) == 0 {
		return
	}
	cp := make([]byte, len(response))
	copy(cp, response)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = idempotencyEntry{
		response:  cp,
		expiresAt: c.now().Add(c.ttl),
	}
}

// isIdempotencyApplicable reports whether the given protocol path is a POST
// route that should be guarded by the idempotency cache.
func isIdempotencyApplicable(protocol string) bool {
	return strings.Contains(protocol, "/post/")
}

func idempotencyKey(protocol, messageID string) string {
	return protocol + "|" + messageID
}

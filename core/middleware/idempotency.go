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
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

const (
	idempotencyTTL  = 10 * time.Minute
	idempotencySize = 4096
)

// idempotencyCache stores responses keyed by (protocol + message id) so that
// duplicate POST requests retried by clients (double-clicks, network retries)
// return the original response without re-executing the side effect.
type idempotencyCache struct {
	cache *lru.LRU[string, []byte]
}

func newIdempotencyCache(ttl time.Duration) *idempotencyCache {
	return &idempotencyCache{
		cache: lru.NewLRU[string, []byte](idempotencySize, nil, ttl),
	}
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

// isIdempotencyApplicable reports whether the given protocol path is a POST
// route that should be guarded by the idempotency cache.
func isIdempotencyApplicable(protocol string) bool {
	return strings.Contains(protocol, "/post/")
}

func idempotencyKey(protocol, messageID string) string {
	return protocol + "|" + messageID
}

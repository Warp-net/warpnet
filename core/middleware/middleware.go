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
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/docker/go-units"
)

type middlewareError string

func (e middlewareError) Error() string {
	return string(e)
}
func (e middlewareError) Bytes() []byte {
	return []byte(e)
}

const (
	ErrUnknownClientPeer middlewareError = `["middleware: auth: unknown client peer"]`
	ErrStreamReadError   middlewareError = `["middleware: stream: reading failed"]`
	ErrInternalNodeError middlewareError = `["middleware: internal node error"]`
)

const (
	MaxLimit = units.MiB * 5 // TODO size limit???
	// ImportMaxLimit is the inbound cap for the Twitter archive import route,
	// whose request body carries the whole archive as base64 (~33% larger
	// than the raw .zip). Kept modest: it's read fully into memory, and the
	// browser-upload path is meant for typical archives — very large ones
	// should use the desktop (read-off-disk) path. Private, signature-gated,
	// owner-only, so not a remote-DoS vector.
	ImportMaxLimit        = units.MiB * 64
	InternalNodeErrorCode = 5000
)

type WarpMiddleware struct {
	idempotency *idempotencyCache
}

func NewWarpMiddleware(ownNodeId warpnet.WarpPeerID) *WarpMiddleware {
	wm := &WarpMiddleware{
		idempotency: newIdempotencyCache(idempotencyTTL),
	}
	return wm
}

// Close releases background resources owned by the middleware (currently
// the idempotency cache's expirable-LRU janitor goroutine). Safe to call
// multiple times.
func (p *WarpMiddleware) Close() {
	if p.idempotency != nil {
		p.idempotency.Close()
	}
}

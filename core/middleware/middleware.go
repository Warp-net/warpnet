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
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/docker/go-units"
)

type middlewareError string

func (e middlewareError) Error() string {
	return string(e)
}

const (
	ErrUnknownClientPeer middlewareError = "middleware: auth: unknown client peer"
	ErrStreamReadError   middlewareError = "middleware: stream: reading failed"
	ErrInternalNodeError middlewareError = "middleware: internal node error"
	ErrStaleMessage      middlewareError = "middleware: auth: stale or replayed message"
)

// messageFreshnessWindow caps how far a signed timestamp may drift from now.
const messageFreshnessWindow = 5 * time.Minute

const (
	MaxLimit              = units.MiB * 50
	InternalNodeErrorCode = 5000
)

type AliasPairer interface {
	GetNodeIDs() (ids []string, err error)
}

type WarpMiddleware struct {
	idempotency     *idempotencyCache
	freshnessWindow time.Duration
	ownNodeId       warpnet.WarpPeerID
	aliases         AliasPairer
}

func NewWarpMiddleware(ownNodeId warpnet.WarpPeerID, aliases AliasPairer) *WarpMiddleware {
	wm := &WarpMiddleware{
		idempotency:     newIdempotencyCache(idempotencyTTL),
		freshnessWindow: messageFreshnessWindow,
		ownNodeId:       ownNodeId,
		aliases:         aliases,
	}
	return wm
}

func (p *WarpMiddleware) Close() {
	if p.idempotency != nil {
		p.idempotency.Close()
	}
}


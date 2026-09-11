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
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
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
	ErrRateLimited       middlewareError = "middleware: too many requests for this route"
)

// messageFreshnessWindow caps how far a signed timestamp may drift from now.
const messageFreshnessWindow = 5 * time.Minute

const (
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

	rateLimitersMx sync.Mutex
	rateLimiters   *lru.LRU[string, *leakyBucketRateLimiter]

	events chan domain.PeerEvent
}

// eventsBuffer bounds what the fan-out holds for a rating that is not
// reading fast enough; past it the oldest observation is simply lost.
const eventsBuffer = 256

func NewWarpMiddleware(ownNodeId warpnet.WarpPeerID, aliases AliasPairer) *WarpMiddleware {
	wm := &WarpMiddleware{
		idempotency:     newIdempotencyCache(idempotencyTTL),
		freshnessWindow: messageFreshnessWindow,
		ownNodeId:       ownNodeId,
		aliases:         aliases,
		rateLimiters:    newRateLimitersCache(),
		events:          make(chan domain.PeerEvent, eventsBuffer),
	}
	return wm
}

// Event is what the middlewares saw the peers do, for whoever rates them.
// The channel is never closed, and a slow reader is never waited for.
func (p *WarpMiddleware) Event() <-chan domain.PeerEvent {
	return p.events
}

// emit reports an observation about the stream's remote peer. A self-stream
// or an unidentified connection names nobody, so it reports nothing.
func (p *WarpMiddleware) emit(s warpnet.WarpStream, t domain.PeerEventType) {
	if p == nil || p.events == nil || s == nil || s.Conn() == nil {
		return
	}
	remote := s.Conn().RemotePeer()
	if remote == "" || remote == s.Conn().LocalPeer() || remote == p.ownNodeId {
		return
	}
	select {
	case p.events <- domain.PeerEvent{
		PeerID: remote.String(),
		Type:   t,
		Route:  string(s.Protocol()),
	}:
	default: // the rating is not worth stalling a request for
	}
}

func (p *WarpMiddleware) Close() {
	if p.idempotency != nil {
		p.idempotency.Close()
	}
	if p.rateLimiters != nil {
		closeExpirableLRU(p.rateLimiters)
	}
}

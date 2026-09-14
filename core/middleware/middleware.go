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

	events  warpnet.PeerEmitter
	ratings PeersRatings
}

// PeersRatings answers how much of a route's allowance a peer may spend,
// which is how a node serves a badly rated peer more slowly.
type PeersRatings interface {
	RateMultiplier(peerID warpnet.WarpPeerID) float64
}

func NewWarpMiddleware(
	ownNodeId warpnet.WarpPeerID, aliases AliasPairer, ratings PeersRatings,
) *WarpMiddleware {
	wm := &WarpMiddleware{
		idempotency:     newIdempotencyCache(idempotencyTTL),
		freshnessWindow: messageFreshnessWindow,
		ownNodeId:       ownNodeId,
		aliases:         aliases,
		rateLimiters:    newRateLimitersCache(),
		events:          warpnet.NewPeerEmitter(),
		ratings:         ratings,
	}
	return wm
}

// Event is what the middlewares saw the peers do. The channel is never closed.
func (p *WarpMiddleware) Event() <-chan warpnet.PeerEvent {
	return p.events
}

// emitStream reports an observation about the stream's remote peer. A
// self-stream names nobody, so it reports nothing.
func (p *WarpMiddleware) emitStream(s warpnet.WarpStream, t warpnet.PeerEventType) {
	if p == nil || s == nil || s.Conn() == nil {
		return
	}
	remote := s.Conn().RemotePeer()
	if remote == s.Conn().LocalPeer() || remote == p.ownNodeId {
		return
	}
	p.events.Emit(warpnet.PeerEvent{
		PeerID: remote.String(),
		Type:   t,
		Route:  string(s.Protocol()),
	})
}

func (p *WarpMiddleware) Close() {
	if p.idempotency != nil {
		p.idempotency.Close()
	}
	if p.rateLimiters != nil {
		closeExpirableLRU(p.rateLimiters)
	}
}

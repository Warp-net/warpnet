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
	"sync/atomic"
	"time"

	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/warpnet"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
	log "github.com/sirupsen/logrus"
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

	writeFloodMx sync.Mutex
	writeFlood   *lru.LRU[string, *atomic.Int64]

	rater rating.Rater
}

func NewWarpMiddleware(ownNodeId warpnet.WarpPeerID, aliases AliasPairer) *WarpMiddleware {
	wm := &WarpMiddleware{
		idempotency:     newIdempotencyCache(idempotencyTTL),
		freshnessWindow: messageFreshnessWindow,
		ownNodeId:       ownNodeId,
		aliases:         aliases,
		rateLimiters:    newRateLimitersCache(),
		writeFlood: lru.NewLRU[string, *atomic.Int64](
			writeFloodCacheSize, nil, writeFloodWindow,
		),
		rater: rating.Nop{},
	}
	return wm
}

func (p *WarpMiddleware) SetRating(r rating.Rater) {
	if p == nil || r == nil {
		return
	}
	p.rater = r
}

func (p *WarpMiddleware) record(s warpnet.WarpStream, kind rating.Kind) {
	if p == nil || p.rater == nil || s == nil || s.Conn() == nil {
		return
	}
	remote := s.Conn().RemotePeer()
	if remote == "" || remote == s.Conn().LocalPeer() || remote == p.ownNodeId {
		return
	}
	if err := p.rater.Record(remote, kind); err != nil {
		log.Warnf("middleware: rating %s for %s: %v", kind, remote, err)
	}
}

func (p *WarpMiddleware) band(remote warpnet.WarpPeerID) rating.Band {
	if p == nil || p.rater == nil {
		return rating.BandTrusted
	}
	b, err := p.rater.Band(remote)
	if err != nil {
		log.Warnf("middleware: reading standing of %s: %v", remote, err)
		return rating.BandTrusted
	}
	return b
}

func (p *WarpMiddleware) Close() {
	if p.idempotency != nil {
		p.idempotency.Close()
	}
	if p.rateLimiters != nil {
		closeExpirableLRU(p.rateLimiters)
	}
	if p.writeFlood != nil {
		closeExpirableLRU(p.writeFlood)
	}
}

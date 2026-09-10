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

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
	log "github.com/sirupsen/logrus"
)

const (
	rateLimiterCacheSize = 4096
	rateLimiterCacheTTL  = 10 * time.Minute
)

type routeLimit struct {
	burst     int64
	perMinute int64
}

var (
	limitDelivery  = routeLimit{burst: 200, perMinute: 1200}
	limitMedia     = routeLimit{burst: 150, perMinute: 600}
	limitRead      = routeLimit{burst: 60, perMinute: 300}
	limitMessaging = routeLimit{burst: 60, perMinute: 300}
	limitWrite     = routeLimit{burst: 30, perMinute: 120}
	limitUpload    = routeLimit{burst: 10, perMinute: 30}
	limitReport    = routeLimit{burst: 10, perMinute: 30}
	limitPairing   = routeLimit{burst: 5, perMinute: 15}
)

var routeLimits = map[string]routeLimit{
	event.PUBLIC_GET_IMAGE: limitMedia,
	event.PUBLIC_GET_VIDEO: limitMedia,

	event.PRIVATE_POST_UPLOAD_IMAGE: limitUpload,
	event.PRIVATE_POST_UPLOAD_VIDEO: limitUpload,

	event.PUBLIC_POST_TIMELINE:          limitDelivery,
	event.PUBLIC_POST_MODERATION_RESULT: limitDelivery,

	event.PUBLIC_POST_CHAT:    limitMessaging,
	event.PUBLIC_POST_MESSAGE: limitMessaging,

	event.PUBLIC_POST_IS_FOLLOWING:       limitRead,
	event.PUBLIC_POST_IS_FOLLOWER:        limitRead,
	event.PUBLIC_POST_VIEW:               limitRead,
	event.PRIVATE_POST_NOTIFICATION_READ: limitRead,

	event.PUBLIC_POST_REPORT: limitReport,

	event.PRIVATE_POST_PAIR:          limitPairing,
	event.PUBLIC_POST_NODE_CHALLENGE: limitPairing,
}

func limitForRoute(route stream.WarpRoute) routeLimit {
	if limit, ok := routeLimits[route.String()]; ok {
		return limit
	}
	if route.IsGet() {
		return limitRead
	}
	return limitWrite
}

func (p *WarpMiddleware) RateLimiterMiddleware(next warpnet.WarpHandlerFunc) warpnet.WarpHandlerFunc {
	return func(data []byte, s warpnet.WarpStream) (any, error) {
		conn := s.Conn()
		if p.rateLimiters == nil || conn == nil {
			return next(data, s)
		}

		remotePeer := conn.RemotePeer()
		if remotePeer == conn.LocalPeer() || remotePeer == p.ownNodeId {
			return next(data, s)
		}

		route := stream.FromPrIDToRoute(s.Protocol())
		if !p.bucket(route, remotePeer).Allow() {
			log.Infof("middleware: rate limiter: %s: limited peer %s", route, remotePeer)
			return event.ResponseError{
				Code: event.RateLimitErrorCode, Message: ErrRateLimited.Error(),
			}, nil
		}
		return next(data, s)
	}
}

func (p *WarpMiddleware) bucket(
	route stream.WarpRoute, remotePeer warpnet.WarpPeerID,
) *leakyBucketRateLimiter {
	key := route.String() + "|" + remotePeer.String()

	p.rateLimitersMx.Lock()
	defer p.rateLimitersMx.Unlock()

	if b, ok := p.rateLimiters.Get(key); ok {
		return b
	}
	b := newRateLimiter(limitForRoute(route))
	p.rateLimiters.Add(key, b)
	return b
}

type leakyBucketRateLimiter struct {
	mx           sync.Mutex
	capacity     int64
	filled       int64
	lastLeak     time.Time
	leakInterval time.Duration
}

func newRateLimiter(limit routeLimit) *leakyBucketRateLimiter {
	if limit.burst <= 0 {
		limit.burst = 1
	}
	if limit.perMinute <= 0 {
		limit.perMinute = 1
	}
	return &leakyBucketRateLimiter{
		capacity:     limit.burst,
		lastLeak:     time.Now(),
		leakInterval: time.Minute / time.Duration(limit.perMinute),
	}
}

func (b *leakyBucketRateLimiter) Allow() bool {
	b.mx.Lock()
	defer b.mx.Unlock()

	if leaks := int64(time.Since(b.lastLeak) / b.leakInterval); leaks > 0 {
		b.filled -= leaks
		if b.filled < 0 {
			b.filled = 0
		}
		b.lastLeak = b.lastLeak.Add(time.Duration(leaks) * b.leakInterval)
	}

	if b.filled >= b.capacity {
		return false
	}
	b.filled++
	return true
}

func newRateLimitersCache() *lru.LRU[string, *leakyBucketRateLimiter] {
	return lru.NewLRU[string, *leakyBucketRateLimiter](
		rateLimiterCacheSize, nil, rateLimiterCacheTTL,
	)
}

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
	"strconv"
	"time"

	"github.com/Warp-net/warpnet/core/fediverse"
	"github.com/Warp-net/warpnet/core/ratelimit"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	log "github.com/sirupsen/logrus"
)

const (
	bucketsCacheSize = 4096
	bucketsCacheTTL  = 10 * time.Minute
)

func (p *WarpMiddleware) RateLimiterMiddleware(next warpnet.WarpHandlerFunc) warpnet.WarpHandlerFunc {
	return func(data []byte, s warpnet.WarpStream) (any, error) {
		conn := s.Conn()
		if p.buckets == nil || conn == nil {
			return next(data, s)
		}

		remotePeer := conn.RemotePeer()
		if remotePeer == conn.LocalPeer() || remotePeer == p.ownNodeId {
			return next(data, s)
		}

		route := stream.FromPrIDToRoute(s.Protocol())
		if !p.allow(route, remotePeer) {
			log.Infof("middleware: rate limiter: %s: limited peer %s", route, remotePeer)
			p.emitStream(s, warpnet.PeerRateLimited)
			return event.ResponseError{
				Code: event.RateLimitErrorCode, Message: ErrRateLimited.Error(),
			}, nil
		}
		return next(data, s)
	}
}

func (p *WarpMiddleware) allow(route stream.WarpRoute, remotePeer warpnet.WarpPeerID) bool {
	multiplier := p.rateMultiplier(remotePeer)
	limit := p.limitForRoute(route, remotePeer).MultipliedBy(multiplier)
	if multiplier < 1 {
		log.Infof(
			"middleware: rate limiter: rating leaves %s %d calls per minute on %s",
			remotePeer, limit.PerMinute(), route,
		)
	}
	// A peer whose standing moved does not keep the bucket it filled under
	// the old one.
	key := route.String() + "|" + remotePeer.String() + "|" + strconv.FormatFloat(multiplier, 'f', 2, 64)
	return p.buckets.Allow(key, limit)
}

func (p *WarpMiddleware) limitForRoute(
	route stream.WarpRoute, remotePeer warpnet.WarpPeerID,
) ratelimit.Limit {
	if remotePeer.String() == fediverse.GatewayNodeID() {
		return p.limits.Gateway()
	}
	if limit, ok := p.limits.Route(route.String()); ok {
		return limit
	}
	if route.IsGet() {
		return p.limits.Read()
	}
	return p.limits.Write()
}

func (p *WarpMiddleware) rateMultiplier(peerID warpnet.WarpPeerID) float64 {
	if p == nil || p.ratings == nil {
		return 1
	}
	return p.ratings.RateMultiplier(peerID)
}

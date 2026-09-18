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
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	log "github.com/sirupsen/logrus"
)

// StreamLimiter answers whether a peer may still call a route.
type StreamLimiter interface {
	Allow(route stream.WarpRoute, remotePeer warpnet.WarpPeerID) bool
	Close()
}

func (p *WarpMiddleware) RateLimiterMiddleware(next warpnet.WarpHandlerFunc) warpnet.WarpHandlerFunc {
	return func(data []byte, s warpnet.WarpStream) (any, error) {
		conn := s.Conn()
		if p.limiter == nil || conn == nil {
			return next(data, s)
		}

		remotePeer := conn.RemotePeer()
		if remotePeer == conn.LocalPeer() || remotePeer == p.ownNodeId {
			return next(data, s)
		}

		route := stream.FromPrIDToRoute(s.Protocol())
		if !p.limiter.Allow(route, remotePeer) {
			log.Infof("middleware: rate limiter: %s: limited peer %s", route, remotePeer)
			p.emitStream(s, warpnet.PeerRateLimited)
			return event.ResponseError{
				Code: event.RateLimitErrorCode, Message: ErrRateLimited.Error(),
			}, nil
		}
		return next(data, s)
	}
}

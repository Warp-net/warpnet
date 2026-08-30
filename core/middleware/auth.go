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
	"slices"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
)

func (p *WarpMiddleware) AuthMiddleware(next warpnet.WarpHandlerFunc) warpnet.WarpHandlerFunc {
	return func(data []byte, s warpnet.WarpStream) (any, error) {
		if s.Conn() == nil {
			log.Errorf("middleware: auth: connection is not ready")
			return nil, ErrInternalNodeError
		}
		var (
			route      = stream.FromPrIDToRoute(s.Protocol())
			remotePeer = s.Conn().RemotePeer()
		)

		var msg event.Message
		if err := json.Unmarshal(data, &msg); err != nil || msg.MessageId == "" {
			log.Errorf("middleware: auth: unmarshaling data: %s %s %v", route, data, err)
			return nil, ErrInternalNodeError
		}

		if msg.Signature == "" {
			log.Errorf("middleware: auth: signature missing: %s", string(data))
			return nil, ErrInternalNodeError
		}
		if remotePeer.Size() == 0 {
			log.Errorf("middleware: auth: connection is not ready")
			return nil, ErrInternalNodeError
		}

		pubKey := warpnet.FromIDToPubKey(remotePeer)
		if err := security.VerifySignature(pubKey, msg.SigningBytes(), msg.Signature); err != nil {
			// Remote-side fault (foreign or outdated peer), not ours: warn, don't error.
			log.Warnf("middleware: auth: signature invalid: %v: route %s, peer %s", err, route, remotePeer)
			return nil, ErrInternalNodeError
		}

		// Freshness gate for remote peers only; loopback self-streams are exempt.
		if remotePeer != s.Conn().LocalPeer() && !p.isFresh(msg.Timestamp) {
			log.Errorf("middleware: auth: %s: stale/replayed message from %s ts=%s",
				route, remotePeer, msg.Timestamp)
			return nil, ErrStaleMessage
		}

		isPairedAlias := p.isPairedAlias(remotePeer, s.Conn().LocalPeer())

		if route.IsPrivate() && !p.isPrivateRouteAllowed(route, remotePeer, s.Conn().LocalPeer(), isPairedAlias) {
			log.Warnf("middleware: auth: %s: private route denied for peer %s", route, remotePeer)
			return nil, ErrUnknownClientPeer
		}

		return next(msg.Body, &warpnet.WarpStreamBody{
			WarpStream:  s,
			MessageId:   msg.MessageId,
			PairedAlias: isPairedAlias,
		})
	}
}

func (p *WarpMiddleware) isPrivateRouteAllowed(
	route stream.WarpRoute, remotePeer, localPeer warpnet.WarpPeerID, isPairedAlias bool,
) bool {
	if remotePeer == localPeer || remotePeer == p.ownNodeId {
		return true
	}
	if route.ProtocolID() == event.PRIVATE_POST_PAIR {
		return true
	}
	return isPairedAlias
}

func (p *WarpMiddleware) isPairedAlias(remotePeer, localPeer warpnet.WarpPeerID) bool {
	if p.aliases == nil || remotePeer == localPeer || remotePeer == p.ownNodeId {
		return false
	}

	ids, err := p.aliases.GetNodeIDs()
	if err != nil {
		log.Errorf("middleware: auth: paired devices: %v", err)
		return false
	}
	return slices.ContainsFunc(ids, func(id string) bool { return id == remotePeer.String() })
}

// isFresh reports whether ts is within the freshness window of now, either way.
func (p *WarpMiddleware) isFresh(ts time.Time) bool {
	if ts.IsZero() {
		return false
	}
	window := p.freshnessWindow
	if window <= 0 {
		window = messageFreshnessWindow
	}
	skew := time.Since(ts)
	if skew < 0 {
		skew = -skew
	}
	return skew <= window
}

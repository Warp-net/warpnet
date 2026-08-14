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
	"errors"
	"io"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
)

func (p *WarpMiddleware) AuthMiddleware(next warpnet.StreamHandler) warpnet.StreamHandler {
	return func(s warpnet.WarpStream) {
		var isAuthSuccess bool
		defer func() {
			if isAuthSuccess {
				return
			}
			_ = s.Close()
		}()
		if s.Conn() == nil {
			log.Errorf("middleware: auth: connection is not ready")
			_, _ = s.Write(ErrInternalNodeError.Bytes())
			return
		}
		var (
			route      = stream.FromPrIDToRoute(s.Protocol())
			remotePeer = s.Conn().RemotePeer()
		)

		limit := int64(MaxLimit)
		reader := io.LimitReader(s, limit+1)
		data, err := io.ReadAll(reader)
		if err != nil && !errors.Is(err, io.EOF) {
			log.Errorf("middleware: auth: reading from stream: %v", err)
			_, _ = s.Write(ErrInternalNodeError.Bytes())
			return
		}

		if int64(len(data)) > limit {
			log.Errorf(
				"middleware: auth: %s: payload exceeds the %d byte limit for this route",
				route, limit,
			)
			_ = s.Reset()
			return
		}

		var msg event.Message
		if err := json.Unmarshal(data, &msg); err != nil || msg.MessageId == "" {
			log.Errorf("middleware: auth: unmarshaling data: %s %s %v", route, data, err)
			_, _ = s.Write(ErrInternalNodeError.Bytes())
			return
		}

		if msg.Signature == "" {
			log.Errorf("middleware: auth: signature missing: %s", string(data))
			_, _ = s.Write(ErrInternalNodeError.Bytes())
			return
		}
		if remotePeer.Size() == 0 {
			log.Errorf("middleware: auth: connection is not ready")
			_, _ = s.Write(ErrInternalNodeError.Bytes())
			return
		}

		pubKey := warpnet.FromIDToPubKey(remotePeer)
		if err := security.VerifySignature(pubKey, msg.SigningBytes(), msg.Signature); err != nil {
			// Remote-side fault (foreign or outdated peer), not ours: warn, don't error.
			log.Warnf("middleware: auth: signature invalid: %v: route %s, peer %s", err, route, remotePeer)
			_, _ = s.Write(ErrInternalNodeError.Bytes())
			return
		}

		// Freshness gate for remote peers only; loopback self-streams are exempt.
		if remotePeer != s.Conn().LocalPeer() && !p.isFresh(msg.Timestamp) {
			log.Errorf("middleware: auth: %s: stale/replayed message from %s ts=%s",
				route, remotePeer, msg.Timestamp)
			_, _ = s.Write(ErrStaleMessage.Bytes())
			return
		}

		// Owner gate: /private/ routes read and write the owner's own data, so
		// only this node and the devices paired with it may reach them.
		if route.IsPrivate() && !p.isPrivateRouteAllowed(route, remotePeer, s.Conn().LocalPeer()) {
			log.Warnf("middleware: auth: %s: private route denied for peer %s", route, remotePeer)
			_, _ = s.Write(ErrUnknownClientPeer.Bytes())
			return
		}

		isAuthSuccess = true

		next(&warpnet.WarpStreamBody{
			WarpStream: s,
			Body:       msg.Body,
			MessageId:  string(msg.MessageId),
		})
	}
}

// isPrivateRouteAllowed reports whether remotePeer may use a /private/ route:
// the node itself (loopback self-streams), a device paired with it, or one of
// the routes below, which are private in name only.
func (p *WarpMiddleware) isPrivateRouteAllowed(
	route stream.WarpRoute, remotePeer, localPeer warpnet.WarpPeerID,
) bool {
	if remotePeer == localPeer || remotePeer == p.ownNodeId {
		return true
	}

	switch route.ProtocolID() {
	// Pairing carries its own authentication - the handler rejects anyone
	// who cannot present the owner's session token - and it is what puts a
	// device in the paired set, so gating it would leave that set empty.
	case event.PRIVATE_POST_PAIR:
		return true
	// Reply create/delete are forwarded to the node of the parent tweet's
	// author (handler.handleNewReply, handler.deleteReply), so they arrive
	// from a peer that owns nothing here. They are node-to-node writes that
	// happen to sit under /private/; moving them out needs a wire change.
	case event.PRIVATE_POST_TWEET, event.PRIVATE_DELETE_TWEET:
		return true
	}

	return p.isPaired(remotePeer)
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

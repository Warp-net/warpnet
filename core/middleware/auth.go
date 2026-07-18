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

		// The per-tweet streaming import route carries one tweet plus up to
		// four base64 photos; allow it a larger ceiling than other routes.
		limit := int64(MaxLimit)
		if string(route) == event.PRIVATE_POST_IMPORT_TWITTER_TWEET {
			limit = int64(ImportTweetMaxLimit)
		}
		reader := io.LimitReader(s, limit)
		data, err := io.ReadAll(reader)
		if err != nil && !errors.Is(err, io.EOF) {
			log.Errorf("middleware: auth: reading from stream: %v", err)
			_, _ = s.Write(ErrInternalNodeError.Bytes())
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

		// Verify over the body plus the timestamp exactly as it appears on the
		// wire (not the re-marshalled time.Time), so the check is byte-identical
		// across Go and the mobile client regardless of RFC3339 formatting.
		var rawTS struct {
			Timestamp string `json:"timestamp"`
		}
		_ = json.Unmarshal(data, &rawTS)
		signingInput := make([]byte, 0, len(msg.Body)+len(rawTS.Timestamp))
		signingInput = append(signingInput, msg.Body...)
		signingInput = append(signingInput, rawTS.Timestamp...)

		pubKey := warpnet.FromIDToPubKey(remotePeer)
		if err := security.VerifySignature(pubKey, signingInput, msg.Signature); err != nil {
			log.Errorf("middleware: auth: signature invalid: %v", err)
			_, _ = s.Write(ErrInternalNodeError.Bytes())
			return
		}

		// Freshness gate, only for genuinely remote peers. Loopback self-streams
		// (the local frontend/bridge path and gossip re-injection) carry a local
		// timestamp and must not be rejected for age.
		if remotePeer != s.Conn().LocalPeer() && !p.isFresh(msg.Timestamp) {
			log.Errorf("middleware: auth: %s: stale/replayed message from %s ts=%s",
				route, remotePeer, msg.Timestamp)
			_, _ = s.Write(ErrStaleMessage.Bytes())
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

// isFresh reports whether ts is within the configured freshness window of now,
// in either direction (covers both stale replays and future-dated clocks).
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

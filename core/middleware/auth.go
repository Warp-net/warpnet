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

		pubKey := warpnet.FromIDToPubKey(remotePeer)
		if err := security.VerifySignature(pubKey, msg.Body, msg.Signature); err != nil {
			log.Errorf("middleware: auth: signature invalid: %v", err)
			_, _ = s.Write(ErrInternalNodeError.Bytes())
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

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

package node

import (
	"errors"
	"io"
	"runtime/debug"

	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

// unwrap terminates the middleware chain: it reads the raw request from the
// stream, runs the composed handler, and — since no further middleware is
// left — writes the returned payload back. This is the only place a
// response is written to the stream.
func (n *WarpNode) unwrap(handler warpnet.WarpHandlerFunc) warpnet.StreamHandler {
	return func(s warpnet.WarpStream) {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("node: unwrap: panic: %v %s", r, debug.Stack())
			}
			_ = s.Close()
		}()

		limit := int64(middleware.MaxLimit)
		reader := io.LimitReader(s, limit+1)
		data, err := io.ReadAll(reader)
		if err != nil && !errors.Is(err, io.EOF) {
			log.Errorf("node: unwrap: reading from stream: %v", err)
			_ = json.NewEncoder(s).Encode(event.ResponseError{Message: middleware.ErrStreamReadError.Error()})
			return
		}
		if int64(len(data)) > limit {
			log.Errorf("node: unwrap: %s: payload exceeds the %d byte limit", s.Protocol(), limit)
			_ = s.Reset()
			return
		}

		log.Debugf(">>> STREAM REQUEST %s %s\n", string(s.Protocol()), string(data))

		response, herr := handler(data, s)
		if herr == nil && s.Protocol() == event.PRIVATE_POST_PAIR {
			log.Debugf("node: unwrap: paired alias: %s", s.Conn().RemotePeer())
		}

		payload, _, encErr := middleware.NormalizeResponse(response, herr, data, s)
		if encErr != nil {
			return
		}

		log.Debugf("<<< STREAM RESPONSE: %s %s\n", string(s.Protocol()), string(payload))
		if len(payload) == 0 {
			return
		}
		if _, werr := s.Write(payload); werr != nil {
			log.Errorf("node: unwrap: writing response to stream: %v", werr)
		}
	}
}

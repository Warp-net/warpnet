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
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/docker/go-units"
	log "github.com/sirupsen/logrus"
	"io"
	"runtime/debug"
	"time"
)

type middlewareError string

func (e middlewareError) Error() string {
	return string(e)
}
func (e middlewareError) Bytes() []byte {
	return []byte(e)
}

const (
	ErrUnknownClientPeer middlewareError = `["middleware: auth: unknown client peer"]`
	ErrStreamReadError   middlewareError = `["middleware: stream: reading failed"]`
	ErrInternalNodeError middlewareError = `["middleware: internal node error"]`
)

type WarpMiddleware struct {
	ownNodeID warpnet.WarpPeerID
}

func NewWarpMiddleware(ownNodeID warpnet.WarpPeerID) *WarpMiddleware {
	return &WarpMiddleware{ownNodeID}
}

func (p *WarpMiddleware) LoggingMiddleware(next warpnet.StreamHandler) warpnet.StreamHandler {
	return func(s warpnet.WarpStream) {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("middleware: panic: %v %s", r, debug.Stack())
			}
		}() //#nosec

		log.Debugf("middleware: server stream opened: %s %s\n", s.Protocol(), s.Conn().RemotePeer())
		before := time.Now()
		next(s)
		after := time.Now()
		log.Debugf(
			"middleware: server stream closed: %s %s, elapsed: %s\n",
			s.Protocol(),
			s.Conn().RemotePeer(),
			after.Sub(before).String(),
		)
	}
}

func (p *WarpMiddleware) AuthMiddleware(next warpnet.StreamHandler) warpnet.StreamHandler {
	return func(s warpnet.WarpStream) {
		var isAuthSuccess bool
		defer func() {
			if isAuthSuccess {
				return
			}
			_ = s.Close()
		}()
		var (
			route      = stream.FromPrIDToRoute(s.Protocol())
			remotePeer = s.Conn().RemotePeer()
		)

		switch {
		case route.IsPrivate() && p.ownNodeID == "":
			log.Errorf("middleware: auth: client peer ID not set, ignoring private route: %s", route)
			_, _ = s.Write(ErrUnknownClientPeer.Bytes())
			return
		case route.IsPrivate() && p.ownNodeID != "" && p.ownNodeID != remotePeer:
			log.Errorf("middleware: auth: client peer id mismatch: %s", remotePeer)
			_, _ = s.Write(ErrUnknownClientPeer.Bytes())
			return

		}

		reader := io.LimitReader(s, units.MiB*5) // TODO size limit???
		data, err := io.ReadAll(reader)
		if err != nil && err != io.EOF {
			log.Errorf("middleware: reading from stream: %v", err)
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
		if s.Conn() == nil || s.Conn().RemotePeer().Size() == 0 {
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
		})
	}
}

func (p *WarpMiddleware) UnwrapStreamMiddleware(handler warpnet.WarpHandlerFunc) warpnet.StreamHandler {
	return func(s warpnet.WarpStream) {
		defer s.Close()

		var (
			response any
			err      error
			encoder  = json.NewEncoder(s)
			data     []byte
		)

		switch s.(type) {
		case *warpnet.WarpStreamBody:
			data = s.(*warpnet.WarpStreamBody).Body
		default:
			reader := io.LimitReader(s, units.MiB*5) // TODO size limit???
			data, err = io.ReadAll(reader)
			if err != nil && err != io.EOF {
				log.Errorf("middleware: reading from stream: %v", err)
				response = event.ErrorResponse{Message: ErrStreamReadError.Error()}
			}
		}

		log.Debugf(">>> STREAM REQUEST %s %s\n", string(s.Protocol()), string(data))

		if response == nil {
			response, err = handler(data, s)
			if err != nil && !errors.Is(err, warpnet.ErrNodeIsOffline) {
				if len(data) > 500 {
					data = data[:500]
				}
				log.Errorf("middleware: handling of %s %s message: %s failed: %v\n", s.Protocol(), s.Conn().RemotePeer(), string(data), err)
				response = event.ErrorResponse{Code: 500, Message: err.Error()} // TODO errors ranking
			}
		}

		log.Debugf("<<< STREAM RESPONSE: %s %+v\n", string(s.Protocol()), response)
		if response == nil {
			response = event.ErrorResponse{Message: "empty response"}
		}

		switch response.(type) {
		case []byte:
			if _, err := s.Write(response.([]byte)); err != nil {
				log.Errorf("middleware: writing raw bytes to stream: %v", err)
			}
			return
		case string:
			if _, err := s.Write([]byte(response.(string))); err != nil {
				log.Errorf("middleware: writing string to stream: %v", err)
			}
			return
		default:
			if err := encoder.Encode(response); err != nil {
				log.Errorf("middleware: failed encoding generic response: %v %v", response, err)
			}
		}
	}
}

/*

	Warpnet - Decentralized Social Network
	Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
	<github.com.mecdy@passmail.net>

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

package middleware

import (
	"errors"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/docker/go-units"
	log "github.com/sirupsen/logrus"
	"io"
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
	ErrUnknownClientPeer middlewareError = "auth failed: unknown client peer"
	ErrStreamReadError   middlewareError = "stream reading failed"
	ErrInternalNodeError middlewareError = "internal node error"
)

type WarpHandler func(msg []byte, s warpnet.WarpStream) (any, error)

type WarpMiddleware struct {
	clientNodeID warpnet.WarpPeerID
}

func NewWarpMiddleware() *WarpMiddleware {
	return &WarpMiddleware{""}
}

func (p *WarpMiddleware) LoggingMiddleware(next warpnet.WarpStreamHandler) warpnet.WarpStreamHandler {
	return func(s warpnet.WarpStream) {
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

func (p *WarpMiddleware) AuthMiddleware(next warpnet.WarpStreamHandler) warpnet.WarpStreamHandler {
	return func(s warpnet.WarpStream) {
		if s.Protocol() == event.PRIVATE_POST_PAIR && p.clientNodeID == "" { // first tether client node
			p.clientNodeID = s.Conn().RemotePeer()
			next(s)
			return
		}
		route := stream.FromPrIDToRoute(s.Protocol())
		if route.IsPrivate() && p.clientNodeID == "" {
			log.Errorf("middleware: client peer ID not set, ignoring private route: %s", route)
			_, _ = s.Write(ErrUnknownClientPeer.Bytes())
			return
		}
		if route.IsPrivate() && p.clientNodeID != "" { // not private == no auth
			if !(p.clientNodeID == s.Conn().RemotePeer()) { // only own client node can do private requests
				log.Errorf("middleware: client peer id mismatch: %s", s.Conn().RemotePeer())
				_, _ = s.Write(ErrUnknownClientPeer.Bytes())
				return
			}
		}
		next(s)
	}
}

func (p *WarpMiddleware) UnwrapStreamMiddleware(fn WarpHandler) warpnet.WarpStreamHandler {
	return func(s warpnet.WarpStream) {
		defer func() {
			s.Close()
			if r := recover(); r != nil {
				log.Errorf("middleware: unwrap stream middleware panic: %v", r)
			}
		}() //#nosec

		var (
			response any
			encoder  = json.JSON.NewEncoder(s)
		)

		reader := io.LimitReader(s, units.MiB*5) // TODO size limit???
		data, err := io.ReadAll(reader)
		if err != nil && err != io.EOF {
			log.Errorf("middleware: reading from stream: %v", err)
			response = event.ErrorResponse{Message: ErrStreamReadError.Error()}
			return
		}
		log.Debugf(">>> STREAM REQUEST %s %s\n", string(s.Protocol()), string(data))

		if response == nil {
			response, err = fn(data, s)
			if err != nil && !errors.Is(err, warpnet.ErrNodeIsOffline) {
				log.Debugf(">>> STREAM REQUEST %s %s\n", string(s.Protocol()), string(data))
				log.Debugf("<<< STREAM RESPONSE: %s %+v\n", string(s.Protocol()), response)
				if len(data) > 500 {
					data = data[:500]
				}
				log.Errorf("middleware: handling of %s %s message: %s failed: %v\n", s.Protocol(), s.Conn().RemotePeer(), string(data), err)
				response = event.ErrorResponse{Code: 500, Message: err.Error()} // TODO errors ranking
			}
		}
		log.Debugf("<<< STREAM RESPONSE: %s %+v\n", string(s.Protocol()), response)

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

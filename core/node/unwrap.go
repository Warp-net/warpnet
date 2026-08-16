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
	"bytes"
	"errors"
	"io"

	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

// unwrap adapts a WarpHandlerFunc to a raw stream handler: it reads the
// request payload, dispatches the handler, and writes the response back.
// Every handler registered via SetStreamHandlers is wrapped by it before
// the stream middlewares are applied on top.
func (n *WarpNode) unwrap(handler warpnet.WarpHandlerFunc) warpnet.StreamHandler {
	return func(s warpnet.WarpStream) {
		defer func() {
			_ = s.Close()
		}()

		var data []byte
		switch typedStream := s.(type) {
		case *warpnet.WarpStreamBody:
			data = typedStream.Body
		default:
			reader := io.LimitReader(s, middleware.MaxLimit)
			d, err := io.ReadAll(reader)
			if err != nil && !errors.Is(err, io.EOF) {
				log.Errorf("node: unwrap: reading from stream: %v", err)
				_ = json.NewEncoder(s).Encode(event.ResponseError{Message: middleware.ErrStreamReadError.Error()})
				return
			}
			data = d
		}

		log.Debugf(">>> STREAM REQUEST %s %s\n", string(s.Protocol()), string(data))

		payload, err := runHandler(handler, data, s)
		if err != nil {
			log.Errorf("node: unwrap: handler dispatch error: %v", err)
		}
		if len(payload) == 0 {
			return
		}

		if _, werr := s.Write(payload); werr != nil {
			log.Errorf("node: unwrap: writing response to stream: %v", werr)
		}
	}
}

// runHandler invokes the wrapped handler and normalises its return value
// into a writable byte payload.
func runHandler(
	handler warpnet.WarpHandlerFunc,
	data []byte,
	s warpnet.WarpStream,
) ([]byte, error) {
	response, err := handler(data, s)
	if err == nil && s.Protocol() == event.PRIVATE_POST_PAIR {
		log.Debugf("node: unwrap: paired alias: %s", s.Conn().RemotePeer())
	}
	if err != nil && !errors.Is(err, warpnet.ErrNodeIsOffline) {
		clip := data
		if len(clip) > 500 { //nolint:mnd
			clip = clip[:500]
		}
		log.Errorf("node: unwrap: handling of %s %s message: %s failed: %v\n",
			s.Protocol(), s.Conn().RemotePeer(), string(clip), err)
		response = event.ResponseError{Code: middleware.InternalNodeErrorCode, Message: err.Error()}
	}

	log.Debugf("<<< STREAM RESPONSE: %s %+v\n", string(s.Protocol()), response)
	if response == nil {
		response = event.ResponseError{Message: middleware.EmptyResponseMessage}
	}

	var payload []byte
	switch typedResponse := response.(type) {
	case []byte:
		payload = typedResponse
	case string:
		payload = []byte(typedResponse)
	default:
		var buf bytes.Buffer
		if encErr := json.NewEncoder(&buf).Encode(response); encErr != nil {
			log.Errorf("node: unwrap: failed encoding generic response: %v %v", response, encErr)
			return nil, encErr
		}
		payload = buf.Bytes()
	}
	return payload, nil
}

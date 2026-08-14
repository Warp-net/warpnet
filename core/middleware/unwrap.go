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
	"bytes"
	"errors"
	"io"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

func (p *WarpMiddleware) UnwrapStreamMiddleware(handler warpnet.WarpHandlerFunc) warpnet.StreamHandler {
	return func(s warpnet.WarpStream) {
		defer func() {
			_ = s.Close()
		}()

		var (
			data      []byte
			messageID string
		)

		switch typedStream := s.(type) {
		case *warpnet.WarpStreamBody:
			data = typedStream.Body
			messageID = typedStream.MessageId
		default:
			reader := io.LimitReader(s, MaxLimit)
			d, err := io.ReadAll(reader)
			if err != nil && !errors.Is(err, io.EOF) {
				log.Errorf("middleware: reading from stream: %v", err)
				_ = json.NewEncoder(s).Encode(event.ResponseError{Message: ErrStreamReadError.Error()})
				return
			}
			data = d
		}

		log.Debugf(">>> STREAM REQUEST %s %s\n", string(s.Protocol()), string(data))

		protocol := string(s.Protocol())
		idempotent := p.idempotency != nil && messageID != "" && isIdempotencyApplicable(protocol)
		var cacheKey string
		if idempotent {
			// Scope the key by authenticated remote peer so two peers
			// can't collide on the same message id within the TTL window.
			var peerID string
			if conn := s.Conn(); conn != nil {
				peerID = conn.RemotePeer().String()
			}
			cacheKey = idempotencyKey(protocol, peerID, messageID)
		}

		// idempotencyCache.do collapses cache lookup, in-flight dedup, and
		// store into one call: cache hits short-circuit the handler;
		// concurrent same-key requests share a single handler invocation;
		// the result is cached only when the compute reports it cacheable.
		var payload []byte
		var err error
		if idempotent {
			payload, err = p.idempotency.do(cacheKey, func() ([]byte, bool, error) {
				return p.runHandler(handler, data, s)
			})
		} else {
			payload, _, err = p.runHandler(handler, data, s)
		}
		if err != nil {
			log.Errorf("middleware: handler dispatch error: %v", err)
		}
		if len(payload) == 0 {
			return
		}

		if _, werr := s.Write(payload); werr != nil {
			log.Errorf("middleware: writing response to stream: %v", werr)
		}
	}
}

// runHandler invokes the wrapped handler, normalises its return value into
// a writable byte payload, and reports whether the result is cacheable.
// Error-shaped responses (handler-returned ResponseError, the synthesized
// "empty response" fallback, or a non-offline error) are not cacheable so
// transient failures don't lock callers into the same response for the TTL.
func (p *WarpMiddleware) runHandler(
	handler warpnet.WarpHandlerFunc,
	data []byte,
	s warpnet.WarpStream,
) ([]byte, bool, error) {
	var (
		response any
		err      error
	)
	switch {
	case s.Protocol() == event.PRIVATE_POST_PAIR:
		response, err = handler(data, s)
		if err == nil {
			log.Debugf("middleware: paired alias: %s", s.Conn().RemotePeer())
		}
	default:
		response, err = handler(data, s)
	}
	if err != nil && !errors.Is(err, warpnet.ErrNodeIsOffline) {
		clip := data
		if len(clip) > 500 { //nolint:mnd
			clip = clip[:500]
		}
		log.Errorf("middleware: handling of %s %s message: %s failed: %v\n",
			s.Protocol(), s.Conn().RemotePeer(), string(clip), err)
		response = event.ResponseError{Code: InternalNodeErrorCode, Message: err.Error()}
	}

	log.Debugf("<<< STREAM RESPONSE: %s %+v\n", string(s.Protocol()), response)
	responseIsError := response == nil
	if response == nil {
		response = event.ResponseError{Message: "empty response"}
	}
	if _, ok := response.(event.ResponseError); ok {
		responseIsError = true
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
			log.Errorf("middleware: failed encoding generic response: %v %v", response, encErr)
			return nil, false, encErr
		}
		payload = buf.Bytes()
	}

	cacheable := err == nil && !responseIsError
	return payload, cacheable, nil
}

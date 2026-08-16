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
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/docker/go-units"
	log "github.com/sirupsen/logrus"
)

type middlewareError string

func (e middlewareError) Error() string {
	return string(e)
}

const (
	ErrUnknownClientPeer middlewareError = "middleware: auth: unknown client peer"
	ErrStreamReadError   middlewareError = "middleware: stream: reading failed"
	ErrInternalNodeError middlewareError = "middleware: internal node error"
	ErrStaleMessage      middlewareError = "middleware: auth: stale or replayed message"
)

// messageFreshnessWindow caps how far a signed timestamp may drift from now.
const messageFreshnessWindow = 5 * time.Minute

const (
	MaxLimit              = units.MiB * 50
	InternalNodeErrorCode = 5000
)

type AliasPairer interface {
	GetNodeIDs() (ids []string, err error)
}

type WarpMiddleware struct {
	idempotency     *idempotencyCache
	freshnessWindow time.Duration
	ownNodeId       warpnet.WarpPeerID
	aliases         AliasPairer
}

func NewWarpMiddleware(ownNodeId warpnet.WarpPeerID, aliases AliasPairer) *WarpMiddleware {
	wm := &WarpMiddleware{
		idempotency:     newIdempotencyCache(idempotencyTTL),
		freshnessWindow: messageFreshnessWindow,
		ownNodeId:       ownNodeId,
		aliases:         aliases,
	}
	return wm
}

func (p *WarpMiddleware) Close() {
	if p.idempotency != nil {
		p.idempotency.Close()
	}
}

// NormalizeResponse converts a handler's return value into the byte payload
// written to the stream, and reports whether that payload may be replayed
// for an idempotent retry: failures and error envelopes must not be cached.
func NormalizeResponse(response any, err error, data []byte, s warpnet.WarpStream) ([]byte, bool, error) {
	if err != nil && !errors.Is(err, warpnet.ErrNodeIsOffline) {
		clip := data
		if len(clip) > 500 { //nolint:mnd
			clip = clip[:500]
		}
		var remotePeer warpnet.WarpPeerID
		if conn := s.Conn(); conn != nil {
			remotePeer = conn.RemotePeer()
		}
		log.Errorf("middleware: handling of %s %s message: %s failed: %v\n",
			s.Protocol(), remotePeer, string(clip), err)
		response = event.ResponseError{Code: InternalNodeErrorCode, Message: err.Error()}
	}

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

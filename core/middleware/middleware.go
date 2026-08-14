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
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/docker/go-units"
	log "github.com/sirupsen/logrus"
)

type middlewareError string

func (e middlewareError) Error() string {
	return string(e)
}

// Bytes renders the error as event.ResponseError, the shape callers already
// parse. A bare JSON array would reach them as an unmarshal failure instead
// of the actual reason.
func (e middlewareError) Bytes() []byte {
	bt, err := json.Marshal(event.ResponseError{
		Code:    InternalNodeErrorCode,
		Message: string(e),
	})
	if err != nil {
		return []byte(e)
	}
	return bt
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

type PairedDevicesStore interface {
	GetDevices(ownerNodeId string) ([]domain.Device, error)
}

type WarpMiddleware struct {
	idempotency     *idempotencyCache
	freshnessWindow time.Duration
	ownNodeId       warpnet.WarpPeerID
	devices         PairedDevicesStore
}

func NewWarpMiddleware(ownNodeId warpnet.WarpPeerID) *WarpMiddleware {
	wm := &WarpMiddleware{
		idempotency:     newIdempotencyCache(idempotencyTTL),
		freshnessWindow: messageFreshnessWindow,
		ownNodeId:       ownNodeId,
	}
	return wm
}

func (p *WarpMiddleware) SetPairedDevices(store PairedDevicesStore) {
	if p == nil {
		return
	}
	p.devices = store
}

func (p *WarpMiddleware) isPaired(id warpnet.WarpPeerID) bool {
	if p.devices == nil {
		return false
	}

	devices, err := p.devices.GetDevices(p.ownNodeId.String())
	if err != nil {
		log.Errorf("middleware: auth: paired devices lookup: %v", err)
		return false
	}
	for _, device := range devices {
		if device.NodeId == id {
			return true
		}
	}
	return false
}

// Close releases background resources owned by the middleware (currently
// the idempotency cache's expirable-LRU janitor goroutine). Safe to call
// multiple times.
func (p *WarpMiddleware) Close() {
	if p.idempotency != nil {
		p.idempotency.Close()
	}
}

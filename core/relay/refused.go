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

package relay

import (
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p/core/connmgr"
)

const (
	ErrNoReservation       warpnet.WarpError = "relay: peer holds no reservation"
	ErrTooManyRefusedPeers warpnet.WarpError = "relay: too many refused peers"

	maxRefusedPeers = 256
	reservationTag  = "relay-reservation"
)

type TagInfoReader interface {
	GetTagInfo(p warpnet.WarpPeerID) *connmgr.TagInfo
}

type RefusedPeers struct {
	mx      sync.Mutex
	tags    TagInfoReader
	ttl     time.Duration
	refused map[warpnet.WarpPeerID]map[warpnet.WarpPeerID]time.Time
}

func NewRefusedPeers(tags TagInfoReader) *RefusedPeers {
	return &RefusedPeers{
		tags:    tags,
		ttl:     warpnet.DefaultResources.ReservationTTL,
		refused: make(map[warpnet.WarpPeerID]map[warpnet.WarpPeerID]time.Time),
	}
}

func (r *RefusedPeers) AllowReserve(_ warpnet.WarpPeerID, _ warpnet.WarpAddress) bool {
	return true
}

func (r *RefusedPeers) AllowConnect(src warpnet.WarpPeerID, _ warpnet.WarpAddress, dest warpnet.WarpPeerID) bool {
	r.mx.Lock()
	defer r.mx.Unlock()

	expiry, ok := r.refused[dest][src]
	return !ok || time.Now().After(expiry)
}

func (r *RefusedPeers) Refuse(dest, src warpnet.WarpPeerID) error {
	if !r.isReserved(dest) {
		return ErrNoReservation
	}

	r.mx.Lock()
	defer r.mx.Unlock()

	now := time.Now()
	r.dropExpired(now)

	peers, ok := r.refused[dest]
	if !ok {
		peers = make(map[warpnet.WarpPeerID]time.Time)
		r.refused[dest] = peers
	}
	if _, ok := peers[src]; !ok && len(peers) >= maxRefusedPeers {
		return ErrTooManyRefusedPeers
	}
	peers[src] = now.Add(r.ttl)
	return nil
}

func (r *RefusedPeers) isReserved(p warpnet.WarpPeerID) bool {
	info := r.tags.GetTagInfo(p)
	if info == nil {
		return false
	}
	_, ok := info.Tags[reservationTag]
	return ok
}

func (r *RefusedPeers) dropExpired(now time.Time) {
	for dest, peers := range r.refused {
		for src, expiry := range peers {
			if now.After(expiry) {
				delete(peers, src)
			}
		}
		if len(peers) == 0 || !r.isReserved(dest) {
			delete(r.refused, dest)
		}
	}
}

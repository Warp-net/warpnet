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

 WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
 Use at your own risk. The maintainers shall not be liable for any damages or data loss
 resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package warpnet

import (
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

// PeerLimits is what a peer may have of this node, as the rating decided
// it. Every field is a weight, never a refusal: the worst-rated peer is
// served slowly and last, not turned away.
type PeerLimits struct {
	PeerID string

	// ConnTag is what the peer is worth to the connection manager when it
	// has to choose whom to keep.
	ConnTag int
	// GossipScore is the peer's application-specific gossipsub score.
	GossipScore float64
	// RateMultiplier is the share of a route's allowance the peer may
	// spend. It never reaches zero, so the peer is slowed and never cut off.
	RateMultiplier float64
	// InRoutingTable keeps the peer in the DHT and in its queries.
	InRoutingTable bool
}

// unlimited is what a peer nobody has rated may have: its whole
// allowance, its place in the routing table, and no opinion either way.
var unlimited = PeerLimits{RateMultiplier: 1, InRoutingTable: true} //nolint:gochecknoglobals

const (
	peerLimitsCacheSize = 4096
	peerLimitsCacheTTL  = 24 * time.Hour
)

// PeerLimiter holds what each peer may have. A node keeps one: the rating
// writes it, and the modules that limit, score or route a peer read it,
// so none of them keeps a copy of its own.
type PeerLimiter struct {
	limits *lru.LRU[string, PeerLimits]
}

func NewPeerLimiter() *PeerLimiter {
	return &PeerLimiter{
		limits: lru.NewLRU[string, PeerLimits](peerLimitsCacheSize, nil, peerLimitsCacheTTL),
	}
}

// Limit records what the rating decided a peer may have.
func (l *PeerLimiter) Limit(limits PeerLimits) {
	if l == nil || l.limits == nil || limits.PeerID == "" {
		return
	}
	l.limits.Add(limits.PeerID, limits)
}

// PeerLimits is what one peer may have. A peer nobody has rated may have
// everything.
func (l *PeerLimiter) PeerLimits(peerID WarpPeerID) PeerLimits {
	if l == nil || l.limits == nil {
		return unlimited
	}
	limits, ok := l.limits.Get(peerID.String())
	if !ok {
		return unlimited
	}
	return limits
}

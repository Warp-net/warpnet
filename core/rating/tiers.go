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
package rating

import (
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

const (
	peerTiersCacheSize = 4096
	peerTiersCacheTTL  = 24 * time.Hour
)

// PeerTiers is what the engine rated each peer, kept where the modules
// that act on it can read it without holding the engine. A node keeps
// one: the engine writes it as ratings move, and the middleware, the
// gossip, the routing table and the connection manager read it.
//
// Every answer it gives is a weight, never a refusal, and a peer the
// engine has said nothing about is answered for as a trusted one.
type PeerTiers struct {
	tiers *lru.LRU[string, Tier]
}

func NewPeerTiers() *PeerTiers {
	return &PeerTiers{tiers: lru.NewLRU[string, Tier](peerTiersCacheSize, nil, peerTiersCacheTTL)}
}

// Set records how the engine now rates a peer.
func (t *PeerTiers) Set(peerID warpnet.WarpPeerID, tier Tier) {
	if t == nil || t.tiers == nil || peerID == "" {
		return
	}
	t.tiers.Add(peerID.String(), tier)
}

// Tier is how a peer is rated. One the engine has said nothing about is
// trusted, which is what a node that has never met it assumes anyway.
func (t *PeerTiers) Tier(peerID warpnet.WarpPeerID) Tier {
	if t == nil || t.tiers == nil {
		return TierTrusted
	}
	tier, ok := t.tiers.Get(peerID.String())
	if !ok {
		return TierTrusted
	}
	return tier
}

// ConnTag is what a peer is worth to the connection manager when it has
// to choose whom to keep.
func (t *PeerTiers) ConnTag(peerID warpnet.WarpPeerID) int {
	return t.Tier(peerID).ConnTag()
}

// GossipScore is what gossipsub weighs a peer by.
func (t *PeerTiers) GossipScore(peerID warpnet.WarpPeerID) float64 {
	return t.Tier(peerID).GossipScore()
}

// RateMultiplier is the share of a route's allowance a peer may spend.
func (t *PeerTiers) RateMultiplier(peerID warpnet.WarpPeerID) float64 {
	return t.Tier(peerID).RateMultiplier()
}

// InRoutingTable reports whether the DHT may hold a peer.
func (t *PeerTiers) InRoutingTable(peerID warpnet.WarpPeerID) bool {
	return t.Tier(peerID).InRoutingTable()
}

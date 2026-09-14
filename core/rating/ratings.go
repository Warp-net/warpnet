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
	peersRatingsCacheSize = 4096
	peersRatingsCacheTTL  = 24 * time.Hour
)

// PeersRatings is how this node rates the peers it has evidence about,
// kept where the modules that act on it read it without holding the
// engine: the middleware, the gossip, the routing table and the
// connection manager.
//
// Every answer it gives is a weight, never a refusal, and a peer the
// engine has said nothing about is answered for as a trusted one.
type PeersRatings struct {
	tiers *lru.LRU[string, Tier]
}

// CollectPeersRatings starts collecting how this node rates its peers.
func CollectPeersRatings() *PeersRatings {
	return &PeersRatings{
		tiers: lru.NewLRU[string, Tier](peersRatingsCacheSize, nil, peersRatingsCacheTTL),
	}
}

// Rate records how the engine now rates a peer.
func (r *PeersRatings) Rate(peerID warpnet.WarpPeerID, tier Tier) {
	if r == nil || r.tiers == nil || peerID == "" {
		return
	}
	r.tiers.Add(peerID.String(), tier)
}

// Tier is how a peer is rated. One the engine has said nothing about is
// trusted, which is what a node that has never met it assumes anyway.
func (r *PeersRatings) Tier(peerID warpnet.WarpPeerID) Tier {
	if r == nil || r.tiers == nil {
		return TierTrusted
	}
	tier, ok := r.tiers.Get(peerID.String())
	if !ok {
		return TierTrusted
	}
	return tier
}

// ConnTag is what a peer is worth to the connection manager when it has
// to choose whom to keep.
func (r *PeersRatings) ConnTag(peerID warpnet.WarpPeerID) int {
	return r.Tier(peerID).ConnTag()
}

// GossipScore is what gossipsub weighs a peer by.
func (r *PeersRatings) GossipScore(peerID warpnet.WarpPeerID) float64 {
	return r.Tier(peerID).GossipScore()
}

// RateMultiplier is the share of a route's allowance a peer may spend.
func (r *PeersRatings) RateMultiplier(peerID warpnet.WarpPeerID) float64 {
	return r.Tier(peerID).RateMultiplier()
}

// IsAllowedInDHT reports whether the DHT may hold a peer.
func (r *PeersRatings) IsAllowedInDHT(peerID warpnet.WarpPeerID) bool {
	return r.Tier(peerID).IsAllowedInDHT()
}

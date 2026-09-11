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

// PeerStanding is what the rating concluded about a peer, in the terms
// each module acts on. Every knob is a weight, never a refusal: a peer
// with the worst standing is served slowly and last, not turned away.
type PeerStanding struct {
	PeerID string

	// ConnTag is what the peer is worth to the connection manager when it
	// has to choose whom to keep.
	ConnTag int
	// GossipScore is the peer's application-specific gossipsub score.
	GossipScore float64
	// LimitMultiplier scales what the peer may spend on a route. It never
	// reaches zero, so the peer is slowed down and never refused.
	LimitMultiplier float64
	// AllowedInDHT keeps the peer in the routing table and in queries.
	AllowedInDHT bool
}

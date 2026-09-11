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

// GossipGraylistThreshold is the gossipsub score below which a peer is graylisted.
const GossipGraylistThreshold = -100

// ConnTag is the connection manager's tag value for a peer of this tier.
func (t Tier) ConnTag() int {
	switch t {
	case TierTrusted:
		return 60
	case TierWatched:
		return 30
	case TierDegraded:
		return 10
	case TierFloor:
		return 1
	default:
		return 60
	}
}

// GossipScore is the application-specific gossipsub score for this tier.
func (t Tier) GossipScore() float64 {
	switch t {
	case TierTrusted:
		return 0
	case TierWatched:
		return -10
	case TierDegraded:
		return -60
	case TierFloor:
		return -200
	default:
		return 0
	}
}

// LimitMultiplier scales a route's burst and per-minute allowance. It never
// reaches zero: a low tier slows a peer down, it does not refuse it service.
func (t Tier) LimitMultiplier() float64 {
	switch t {
	case TierTrusted:
		return 1
	case TierWatched:
		return 0.5
	case TierDegraded:
		return 0.25
	case TierFloor:
		return 0.1
	default:
		return 1
	}
}

// AllowedInDHT reports whether peers of this tier stay in the routing table.
func (t Tier) AllowedInDHT() bool {
	return t != TierFloor
}

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

// The knobs a band turns. Pure functions with no dependencies, so the
// mapping from standing to consequence is testable on its own and
// lives in exactly one place.
//
// What is deliberately absent: nothing here refuses service, and
// nothing here blocklists. A low rating makes a peer slow and last in
// the queue; it never cuts it off. Automatic blocklisting on a
// gossiped reputation would let a slander campaign partition an honest
// node off the network, which is a worse attack than the one being
// defended against.

// ConnTagValue is the libp2p ConnManager tag weight. Written under a
// tag of its own, separate from the reachability tag, so the two
// compose additively instead of overwriting each other.
func ConnTagValue(b Band) int {
	switch b {
	case BandTrusted:
		return 60
	case BandWatched:
		return 30
	case BandDegraded:
		return 10
	case BandFloor:
		return 1
	default:
		return 60
	}
}

// GossipAppScore feeds gossipsub's AppSpecificScore. GraylistThreshold
// is -100, so only BandFloor reaches it — and per CapRemoteTotal a
// peer only reaches BandFloor on evidence we gathered ourselves.
func GossipAppScore(b Band) float64 {
	switch b {
	case BandTrusted:
		return 0
	case BandWatched:
		return -10
	case BandDegraded:
		return -60
	case BandFloor:
		return -200
	default:
		return 0
	}
}

// GossipGraylistThreshold is the score below which gossipsub stops
// reading from a peer entirely.
const GossipGraylistThreshold = -100

// LimitMultiplier scales a route's burst and per-minute allowance.
func LimitMultiplier(b Band) float64 {
	switch b {
	case BandTrusted:
		return 1
	case BandWatched:
		return 0.5
	case BandDegraded:
		return 0.25
	case BandFloor:
		return 0.1
	default:
		return 1
	}
}

// AllowInDHT reports whether a peer may enter the routing table and be
// dialled during queries.
func AllowInDHT(b Band) bool {
	return b != BandFloor
}

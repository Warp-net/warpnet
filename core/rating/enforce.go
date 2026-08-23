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

func AllowInDHT(b Band) bool {
	return b != BandFloor
}

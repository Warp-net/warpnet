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
	"github.com/Warp-net/warpnet/core/warpnet"
)

// Dimension is one axis of a node's rating. Which axes a node tracks
// depends on its role: a relay can only ever witness wire behaviour, a
// moderator is additionally judged on the verdicts it casts.
type Dimension uint8

const (
	Network     Dimension = iota // every node type
	Application                  // member nodes
	Moderation                   // moderator nodes
)

// String is the wire and key form. Kept short: it appears in every
// CRDT key.
func (d Dimension) String() string {
	switch d {
	case Network:
		return "net"
	case Application:
		return "app"
	case Moderation:
		return "mod"
	default:
		return "unknown"
	}
}

func ParseDimension(s string) (Dimension, bool) {
	switch s {
	case "net":
		return Network, true
	case "app":
		return Application, true
	case "mod":
		return Moderation, true
	default:
		return 0, false
	}
}

func (d Dimension) Valid() bool {
	return d == Network || d == Application || d == Moderation
}

// DimensionsFor maps a warpnet.NodeInfo.Type to the axes that node
// tracks. An unknown type gets the network axis only: every node
// speaks the wire, nothing else can be assumed.
func DimensionsFor(nodeType string) []Dimension {
	switch nodeType {
	case warpnet.MemberNode:
		return []Dimension{Network, Application}
	case warpnet.ModeratorNode:
		return []Dimension{Network, Moderation}
	case warpnet.RelayNode:
		return []Dimension{Network}
	default:
		return []Dimension{Network}
	}
}

// Score is a node's standing on one axis, or the minimum across the
// axes its role tracks. A node nobody has ever observed scores
// MaxScore — full trust, no probation.
type Score int32

const (
	MaxScore Score = 1000
	MinScore Score = 0
)

func (s Score) clamp() Score {
	if s > MaxScore {
		return MaxScore
	}
	if s < MinScore {
		return MinScore
	}
	return s
}

// Band buckets a Score into the four states enforcement actually
// keys off. Thresholds live here and nowhere else.
type Band uint8

const (
	BandTrusted  Band = iota // 800..1000  no effect
	BandWatched              // 500..799   mild deprioritisation
	BandDegraded             // 200..499   halved rate limits, low priority
	BandFloor                // 0..199     minimum priority, gossipsub graylist range
)

const (
	trustedFloor  Score = 800
	watchedFloor  Score = 500
	degradedFloor Score = 200
)

func BandOf(s Score) Band {
	switch {
	case s >= trustedFloor:
		return BandTrusted
	case s >= watchedFloor:
		return BandWatched
	case s >= degradedFloor:
		return BandDegraded
	default:
		return BandFloor
	}
}

func (b Band) String() string {
	switch b {
	case BandTrusted:
		return "trusted"
	case BandWatched:
		return "watched"
	case BandDegraded:
		return "degraded"
	case BandFloor:
		return "floor"
	default:
		return "unknown"
	}
}

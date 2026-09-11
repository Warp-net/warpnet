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
	"math"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
)

const unknownName = "unknown"

// Dimension is one axis of a node's behaviour. A node witnesses only the
// dimensions its role can observe.
type Dimension uint8

const (
	Network     Dimension = iota // every node type
	Application                  // member nodes
	Moderation                   // moderator nodes
)

// retentionHalfLives is how many half-lives a record stays relevant.
const retentionHalfLives = 8

// String is the wire name of the dimension.
func (d Dimension) String() string {
	switch d {
	case Network:
		return "net"
	case Application:
		return "app"
	case Moderation:
		return "mod"
	default:
		return unknownName
	}
}

// Valid reports whether d is a known dimension.
func (d Dimension) Valid() bool {
	return d == Network || d == Application || d == Moderation
}

// HalfLife is the time a penalty on this dimension takes to halve.
func (d Dimension) HalfLife() time.Duration {
	switch d {
	case Network:
		return 12 * time.Hour
	case Application, Moderation:
		return 7 * 24 * time.Hour
	default:
		return 0
	}
}

// Retention is how long a record on this dimension still counts; older
// records are ignored on read and deleted by their author.
func (d Dimension) Retention() time.Duration {
	return retentionHalfLives * d.HalfLife()
}

// decay is the weight left on evidence of the given age.
func (d Dimension) decay(age time.Duration) float64 {
	if age <= 0 {
		return 1
	}
	half := d.HalfLife()
	if half <= 0 {
		return 0
	}
	return math.Exp2(-age.Hours() / half.Hours())
}

// ParseDimension resolves a dimension from its wire name.
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

// Dimensions lists what a node of the given type can witness.
func Dimensions(nodeType string) []Dimension {
	switch nodeType {
	case warpnet.MemberNode:
		return []Dimension{Network, Application}
	case warpnet.ModeratorNode:
		// A moderator judges content for a living, so it witnesses the
		// application axis as well as the wire and its own peers.
		return []Dimension{Network, Application, Moderation}
	default:
		return []Dimension{Network}
	}
}

// Score is a peer's standing. A peer nobody has observed holds MaxScore.
type Score int32

// MaxScore is full trust and MinScore the floor; every score lies between them.
const (
	MaxScore Score = 1000
	MinScore Score = 0
)

func (s Score) clamp() Score {
	return max(MinScore, min(MaxScore, s))
}

// Tier is the coarse standing that enforcement acts on.
type Tier uint8

const (
	TierTrusted  Tier = iota // 800..1000  no effect
	TierWatched              // 500..799   mild deprioritisation
	TierDegraded             // 200..499   halved rate limits, low priority
	TierFloor                // 0..199     minimum priority, gossipsub graylist range
)

const (
	trustedFloor  Score = 800
	watchedFloor  Score = 500
	degradedFloor Score = 200
)

// Tier is the coarse standing this score falls into.
func (s Score) Tier() Tier {
	switch {
	case s >= trustedFloor:
		return TierTrusted
	case s >= watchedFloor:
		return TierWatched
	case s >= degradedFloor:
		return TierDegraded
	default:
		return TierFloor
	}
}

// String is the wire name of the tier.
func (t Tier) String() string {
	switch t {
	case TierTrusted:
		return "trusted"
	case TierWatched:
		return "watched"
	case TierDegraded:
		return "degraded"
	case TierFloor:
		return "floor"
	default:
		return unknownName
	}
}

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

import "github.com/Warp-net/warpnet/core/warpnet"

// Reporter is the write side as seen by an entry source. Keeping
// it this narrow means middleware, handlers and the discovery loop
// depend on one method, not on the store.
type Reporter interface {
	Record(subject warpnet.WarpPeerID, k Kind)
}

// Scorer is the read side as seen by an enforcement point.
type Scorer interface {
	Score(subject warpnet.WarpPeerID) Score
	// Band is the standing actually observed, for display and for
	// deciding what to report.
	Band(subject warpnet.WarpPeerID) Band
	// EffectiveBand is the standing enforcement should apply. In
	// shadow mode it is always BandTrusted and the observed band is
	// reported instead, so no enforcement point has to know about
	// modes and no call site can forget the check.
	EffectiveBand(subject warpnet.WarpPeerID) Band
	Mode() Mode
}

// Rater is both halves, which is what most call sites actually hold.
type Rater interface {
	Reporter
	Scorer
}

// Nop stands in wherever no store was built — early startup, tests,
// a node type that does not carry one yet.
//
// It reports BandTrusted for everyone, so an absent rating store means
// "nobody is penalised" rather than "everybody is". Enforcement that
// silently engages because a dependency is missing would be far worse
// than enforcement that silently does not.
type Nop struct{}

func (Nop) Record(warpnet.WarpPeerID, Kind)       {}
func (Nop) Score(warpnet.WarpPeerID) Score        { return MaxScore }
func (Nop) Band(warpnet.WarpPeerID) Band          { return BandTrusted }
func (Nop) EffectiveBand(warpnet.WarpPeerID) Band { return BandTrusted }
func (Nop) Mode() Mode                            { return ModeShadow }

var _ Rater = Nop{}

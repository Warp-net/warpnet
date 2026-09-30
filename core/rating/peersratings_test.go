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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAPeerTheEngineSaidNothingAboutReadsAsTrusted(t *testing.T) {
	r := NewPeersRatings()
	peer := newIdentity(t).id

	assert.Equal(t, TierTrusted, r.Tier(peer))
	assert.Equal(t, TierTrusted.ConnTag(), r.ConnTag(peer))
	assert.Zero(t, r.GossipScore(peer))
	assert.Equal(t, float64(1), r.RateMultiplier(peer))
	assert.True(t, r.IsAllowedInDHT(peer))
}

// Every module reads its own knob from the one tier the engine recorded.
func TestEveryModuleReadsTheTierTheEngineRecorded(t *testing.T) {
	r := NewPeersRatings()
	peer := newIdentity(t).id

	for _, tier := range []Tier{TierWatched, TierDegraded, TierFloor, TierTrusted} {
		r.Rate(peer, tier)
		assert.Equal(t, tier, r.Tier(peer))
		assert.Equal(t, tier.ConnTag(), r.ConnTag(peer), "%s", tier)
		assert.Equal(t, tier.GossipScore(), r.GossipScore(peer), "%s", tier)
		assert.Equal(t, tier.RateMultiplier(), r.RateMultiplier(peer), "%s", tier)
		assert.Equal(t, tier.IsAllowedInDHT(), r.IsAllowedInDHT(peer), "%s", tier)
	}
}

func TestRatingOnePeerLeavesTheOthersAlone(t *testing.T) {
	r := NewPeersRatings()
	floored, bystander := newIdentity(t).id, newIdentity(t).id

	r.Rate(floored, TierFloor)

	assert.Equal(t, TierFloor, r.Tier(floored))
	assert.Equal(t, TierTrusted, r.Tier(bystander))
}

func TestRatingsAreSafeWhenMissingOrEmpty(t *testing.T) {
	var nilRatings *PeersRatings
	zero := &PeersRatings{}
	peer := newIdentity(t).id

	for name, r := range map[string]*PeersRatings{"nil": nilRatings, "zero": zero} {
		assert.NotPanics(t, func() { r.Rate(peer, TierFloor) }, name)
		assert.Equal(t, TierTrusted, r.Tier(peer), name)
		assert.True(t, r.IsAllowedInDHT(peer), name)
	}

	r := NewPeersRatings()
	r.Rate("", TierFloor)
	assert.Equal(t, TierTrusted, r.Tier(""), "a peer with no id is nobody to rate")
}

// An out-of-range tier must fail safe: full service, never a refusal.
func TestAnUnknownTierFailsOpen(t *testing.T) {
	unknown := Tier(42)

	assert.Equal(t, unknownName, unknown.String())
	assert.Equal(t, TierTrusted.ConnTag(), unknown.ConnTag())
	assert.Equal(t, TierTrusted.GossipScore(), unknown.GossipScore())
	assert.Equal(t, TierTrusted.RateMultiplier(), unknown.RateMultiplier())
	assert.True(t, unknown.IsAllowedInDHT())

	assert.Equal(t, unknownName, Dimension(9).String())
	assert.False(t, Dimension(9).Valid())
	assert.Zero(t, Dimension(9).HalfLife())
	assert.Zero(t, Dimension(9).decay(time.Hour))
}

// Gossipsub graylists below its threshold: only the floor goes there, and
// every other tier stays above it.
func TestOnlyTheFloorIsGraylisted(t *testing.T) {
	for _, tier := range []Tier{TierTrusted, TierWatched, TierDegraded} {
		assert.Greater(t, tier.GossipScore(), float64(GossipGraylistThreshold), "%s", tier)
	}
	assert.Less(t, TierFloor.GossipScore(), float64(GossipGraylistThreshold))
}

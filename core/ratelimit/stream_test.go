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

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package ratelimit

import (
	"testing"

	"github.com/Warp-net/warpnet/core/fediverse"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamLimiterRoutes(t *testing.T) {
	limiter := NewStreamLimiter(Settings{}, nil)
	t.Cleanup(limiter.Close)
	peer := warpnet.WarpPeerID("member-peer")

	for _, route := range []string{event.PUBLIC_GET_TWEETS, event.PRIVATE_GET_TIMELINE} {
		assert.Equal(t, limiter.read, limiter.route(stream.WarpRoute(route), peer),
			"%s falls back to the read budget", route)
	}
	for _, route := range []string{event.PUBLIC_POST_REACT, event.PRIVATE_DELETE_TWEET} {
		assert.Equal(t, limiter.write, limiter.route(stream.WarpRoute(route), peer),
			"%s falls back to the write budget", route)
	}
	assert.Equal(t, media, limiter.route(stream.WarpRoute(event.PUBLIC_GET_IMAGE), peer))
	assert.Equal(t, pairing, limiter.route(stream.WarpRoute(event.PRIVATE_POST_PAIR), peer))
	assert.Equal(t, limiter.read, limiter.route(stream.WarpRoute(event.PUBLIC_POST_VIEW), peer),
		"a route that is a read in all but name follows the read budget")
}

func TestStreamLimiterCarriesTheOwnersBudgets(t *testing.T) {
	limiter := NewStreamLimiter(Settings{
		StreamReadBurst: 7, StreamReadPerMinute: 70,
		StreamWriteBurst: 3, StreamWritePerMinute: 30,
	}, nil)
	t.Cleanup(limiter.Close)
	peer := warpnet.WarpPeerID("member-peer")

	cases := map[string]Limit{
		event.PUBLIC_POST_VIEW:  PerMinute(7, 70),
		event.PUBLIC_GET_TWEETS: PerMinute(7, 70),
		event.PUBLIC_POST_REACT: PerMinute(3, 30),
		event.PUBLIC_GET_IMAGE:  PerMinute(150, 600),
	}
	for route, want := range cases {
		assert.Equal(t, want, limiter.route(stream.WarpRoute(route), peer), route)
	}
}

func TestStreamLimiterFallsBackToTheDefaults(t *testing.T) {
	limiter := NewStreamLimiter(Settings{}, nil)
	t.Cleanup(limiter.Close)

	assert.Equal(t, PerMinute(int64(Defaults.StreamReadBurst), int64(Defaults.StreamReadPerMinute)), limiter.read)
	assert.Equal(t, PerMinute(int64(Defaults.StreamWriteBurst), int64(Defaults.StreamWritePerMinute)), limiter.write)
}

func TestStreamLimiterGivesTheGatewayItsOwnBudget(t *testing.T) {
	limiter := NewStreamLimiter(Settings{}, nil)
	t.Cleanup(limiter.Close)

	gatewayPeer := warpnet.FromStringToPeerID(fediverse.GatewayNodeID())
	require.NotEmpty(t, gatewayPeer, "fediverse.GatewayNodeID() must be a valid peer id")

	for _, route := range []string{
		event.PUBLIC_GET_USER, event.PUBLIC_GET_IMAGE, event.PUBLIC_POST_REACT, event.PRIVATE_POST_PAIR,
	} {
		assert.Equal(t, gateway, limiter.route(stream.WarpRoute(route), gatewayPeer), route)
	}
	assert.Equal(t, limiter.read, limiter.route(stream.WarpRoute(event.PUBLIC_GET_USER), "someone-else"),
		"another peer stays on its own budget")
}

func TestStreamLimiterSpendsAndRefuses(t *testing.T) {
	limiter := NewStreamLimiter(Settings{StreamWriteBurst: 2, StreamWritePerMinute: 2}, nil)
	t.Cleanup(limiter.Close)
	peer := warpnet.WarpPeerID("member-peer")
	other := warpnet.WarpPeerID("another-peer")
	route := stream.WarpRoute(event.PUBLIC_POST_REACT)

	assert.True(t, limiter.Allow(route, peer))
	assert.True(t, limiter.Allow(route, peer))
	assert.False(t, limiter.Allow(route, peer), "a spent budget is refused")
	assert.True(t, limiter.Allow(route, other), "one peer's spent bucket must not limit another")
	assert.True(t, limiter.Allow(stream.WarpRoute(event.PUBLIC_GET_USER), peer),
		"a spent write bucket must not limit reads")
}

func TestStreamLimiterFollowsTheRating(t *testing.T) {
	ratings := rating{}
	limiter := NewStreamLimiter(Settings{StreamWriteBurst: 4, StreamWritePerMinute: 4}, ratings)
	t.Cleanup(limiter.Close)
	peer := warpnet.WarpPeerID("member-peer")
	route := stream.WarpRoute(event.PUBLIC_POST_REACT)

	spend := func() int {
		var spent int
		for limiter.Allow(route, peer) {
			spent++
			if spent > 100 {
				t.Fatal("expected the budget to run out")
			}
		}
		return spent
	}

	assert.Equal(t, 4, spend(), "a peer nobody has rated spends everything")

	ratings[peer.String()] = 0.5
	assert.Equal(t, 2, spend(), "a peer whose rating dropped is measured against what it has now")
	assert.Zero(t, spend(), "a rating that did not change must not hand a peer a fresh bucket")

	ratings[peer.String()] = 0.01
	assert.Equal(t, 1, spend(), "a peer is slowed down, never starved")
}

func TestStreamLimiterIsNilSafe(t *testing.T) {
	var limiter *StreamLimiter
	require.True(t, limiter.Allow(stream.WarpRoute(event.PUBLIC_GET_USER), "peer"))
	require.NotPanics(t, limiter.Close)
}

type rating map[string]float64

func (r rating) RateMultiplier(peerID warpnet.WarpPeerID) float64 {
	if multiplier, ok := r[peerID.String()]; ok {
		return multiplier
	}
	return 1
}

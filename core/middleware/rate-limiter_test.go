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

package middleware

import (
	"testing"

	"github.com/Warp-net/warpnet/core/fediverse"
	"github.com/Warp-net/warpnet/core/ratelimit"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
)

func newLimiterMiddlewareForTest(t *testing.T, ownNodeId warpnet.WarpPeerID) *WarpMiddleware {
	t.Helper()
	mw := NewWarpMiddleware(ownNodeId, nil, nil, ratelimit.Settings{})
	t.Cleanup(mw.Close)
	return mw
}

func callLimited(
	t *testing.T, mw *WarpMiddleware, local, remote warpnet.WarpPeerID, route string,
) bool {
	t.Helper()

	reached := false
	handler := mw.RateLimiterMiddleware(func(_ []byte, _ warpnet.WarpStream) (any, error) {
		reached = true
		return []byte(`["ok"]`), nil
	})

	client, server := stream.NewLoopbackStream(local, remote, warpnet.WarpProtocolID(route))
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	response, err := handler(nil, remoteStream{
		WarpStream: server,
		conn:       remoteConn{local: local, remote: remote},
	})
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", route, err)
	}
	if reached {
		return true
	}

	respErr, ok := response.(event.ResponseError)
	if !ok {
		t.Fatalf("%s: expected a rate limit response, got %T", route, response)
	}
	if respErr.Code != event.RateLimitErrorCode {
		t.Fatalf("%s: expected code %d, got %d", route, event.RateLimitErrorCode, respErr.Code)
	}
	return false
}

func TestRateLimiterMiddleware_LimitsPerRouteAndPeer(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	peer, _ := newRemotePeer(t)
	otherPeer, _ := newRemotePeer(t)
	mw := newLimiterMiddlewareForTest(t, ownNodeId)

	burst := int(mw.limitForRoute(stream.WarpRoute(event.PRIVATE_POST_PAIR), peer).Burst)
	for i := range burst {
		if !callLimited(t, mw, ownNodeId, peer, event.PRIVATE_POST_PAIR) {
			t.Fatalf("pairing request %d of the burst must be admitted", i+1)
		}
	}
	if callLimited(t, mw, ownNodeId, peer, event.PRIVATE_POST_PAIR) {
		t.Fatal("expected the pairing request past the burst to be limited")
	}

	if !callLimited(t, mw, ownNodeId, peer, event.PUBLIC_GET_USER) {
		t.Fatal("a spent pairing bucket must not limit reads")
	}
	if !callLimited(t, mw, ownNodeId, otherPeer, event.PRIVATE_POST_PAIR) {
		t.Fatal("one peer's spent bucket must not limit another peer")
	}
}

func TestRateLimiterMiddleware_SelfStreamsExempt(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	mw := newLimiterMiddlewareForTest(t, ownNodeId)

	pairing := mw.limitForRoute(stream.WarpRoute(event.PRIVATE_POST_PAIR), ownNodeId)
	for i := range int(pairing.Burst) + 5 {
		if !callLimited(t, mw, ownNodeId, ownNodeId, event.PRIVATE_POST_PAIR) {
			t.Fatalf("self stream %d must not be limited", i+1)
		}
	}
}

func TestLimitForRoute(t *testing.T) {
	mw := newLimiterMiddlewareForTest(t, "own-node")
	peer := warpnet.WarpPeerID("member-peer")

	reads := []string{event.PUBLIC_GET_TWEETS, event.PRIVATE_GET_TIMELINE}
	for _, route := range reads {
		if got := mw.limitForRoute(stream.WarpRoute(route), peer); got != mw.limits.Read() {
			t.Fatalf("%s: expected the read budget %+v, got %+v", route, mw.limits.Read(), got)
		}
	}

	writes := []string{event.PUBLIC_POST_REACT, event.PRIVATE_DELETE_TWEET}
	for _, route := range writes {
		if got := mw.limitForRoute(stream.WarpRoute(route), peer); got != mw.limits.Write() {
			t.Fatalf("%s: expected the write budget %+v, got %+v", route, mw.limits.Write(), got)
		}
	}

	own := []string{event.PUBLIC_GET_IMAGE, event.PUBLIC_POST_TIMELINE, event.PRIVATE_POST_PAIR}
	for _, route := range own {
		want, ok := mw.limits.Route(route)
		if !ok {
			t.Fatalf("%s: expected a budget of its own", route)
		}
		if got := mw.limitForRoute(stream.WarpRoute(route), peer); got != want {
			t.Fatalf("%s: expected %+v, got %+v", route, want, got)
		}
	}
}

func TestLimitForRouteGivesTheGatewayItsOwnBudget(t *testing.T) {
	mw := newLimiterMiddlewareForTest(t, "own-node")

	gateway := warpnet.FromStringToPeerID(fediverse.GatewayNodeID())
	if gateway == "" {
		t.Fatalf("fediverse.GatewayNodeID() is not a valid peer id: %q", fediverse.GatewayNodeID())
	}
	for _, route := range []string{
		event.PUBLIC_GET_USER, event.PUBLIC_GET_IMAGE, event.PUBLIC_POST_REACT, event.PRIVATE_POST_PAIR,
	} {
		if got := mw.limitForRoute(stream.WarpRoute(route), gateway); got != mw.limits.Gateway() {
			t.Fatalf("%s: expected the gateway budget %+v, got %+v", route, mw.limits.Gateway(), got)
		}
	}
}

func TestLimitForRouteKeepsOtherPeersOnTheirBudget(t *testing.T) {
	mw := newLimiterMiddlewareForTest(t, "own-node")

	got := mw.limitForRoute(stream.WarpRoute(event.PUBLIC_GET_USER), warpnet.WarpPeerID("someone-else"))
	if got != mw.limits.Read() {
		t.Fatalf("expected %+v, got %+v", mw.limits.Read(), got)
	}
}

func TestOwnerLimitsReachMappedAndFallbackRoutes(t *testing.T) {
	mw := NewWarpMiddleware("own-node", nil, nil, ratelimit.Settings{
		StreamReadBurst: 7, StreamReadPerMinute: 70,
		StreamWriteBurst: 3, StreamWritePerMinute: 30,
	})
	t.Cleanup(mw.Close)

	peer := warpnet.WarpPeerID("member-peer")
	cases := map[string]ratelimit.Limit{
		event.PUBLIC_POST_VIEW:  ratelimit.PerMinute(7, 70),
		event.PUBLIC_GET_TWEETS: ratelimit.PerMinute(7, 70),
		event.PUBLIC_POST_REACT: ratelimit.PerMinute(3, 30),
		event.PUBLIC_GET_IMAGE:  ratelimit.PerMinute(150, 600),
	}
	for route, want := range cases {
		if got := mw.limitForRoute(stream.WarpRoute(route), peer); got != want {
			t.Fatalf("%s: expected %+v, got %+v", route, want, got)
		}
	}
}

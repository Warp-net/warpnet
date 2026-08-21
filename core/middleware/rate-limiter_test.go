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
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
)

func newLimiterMiddlewareForTest(t *testing.T, ownNodeId warpnet.WarpPeerID) *WarpMiddleware {
	t.Helper()
	mw := &WarpMiddleware{ownNodeId: ownNodeId, rateLimiters: newRateLimitersCache()}
	t.Cleanup(func() { closeExpirableLRU(mw.rateLimiters) })
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

func TestLeakyBucket_AdmitsBurstThenLeaks(t *testing.T) {
	b := newRateLimiter(routeLimit{burst: 3, perMinute: 60_000})

	for i := range 3 {
		if !b.Allow() {
			t.Fatalf("request %d of the burst must be admitted", i+1)
		}
	}
	if b.Allow() {
		t.Fatal("expected the request past the burst to be refused")
	}

	time.Sleep(5 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("expected the bucket to admit again after leaking")
	}
}

func TestLeakyBucket_ZeroLimitFallsBackToOne(t *testing.T) {
	b := newRateLimiter(routeLimit{})
	if !b.Allow() {
		t.Fatal("expected the first request to be admitted")
	}
	if b.Allow() {
		t.Fatal("expected the second request to be refused")
	}
}

func TestRateLimiterMiddleware_LimitsPerRouteAndPeer(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	peer, _ := newRemotePeer(t)
	otherPeer, _ := newRemotePeer(t)
	mw := newLimiterMiddlewareForTest(t, ownNodeId)

	burst := int(limitPairing.burst)
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

	for i := range int(limitPairing.burst) + 5 {
		if !callLimited(t, mw, ownNodeId, ownNodeId, event.PRIVATE_POST_PAIR) {
			t.Fatalf("self stream %d must not be limited", i+1)
		}
	}
}

func TestLimitForRoute(t *testing.T) {
	cases := map[string]routeLimit{
		event.PUBLIC_GET_IMAGE:     limitMedia,
		event.PUBLIC_POST_TIMELINE: limitDelivery,
		event.PUBLIC_POST_VIEW:     limitRead,
		event.PRIVATE_POST_PAIR:    limitPairing,
		event.PUBLIC_GET_TWEETS:    limitRead,
		event.PRIVATE_GET_TIMELINE: limitRead,
		event.PUBLIC_POST_REACT:    limitWrite,
		event.PRIVATE_DELETE_TWEET: limitWrite,
	}
	for route, want := range cases {
		if got := limitForRoute(stream.WarpRoute(route)); got != want {
			t.Fatalf("%s: expected %+v, got %+v", route, want, got)
		}
	}
}

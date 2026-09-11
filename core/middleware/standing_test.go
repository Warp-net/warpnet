// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pairingRoute has the tightest bucket, so an allowance shows in it soonest.
const pairingRoute = event.PRIVATE_POST_PAIR

// spend counts how many requests a peer gets through before its bucket runs out.
func spend(t *testing.T, mw *WarpMiddleware, local, remote warpnet.WarpPeerID) int {
	t.Helper()
	var allowed int
	for range 200 {
		if !callLimited(t, mw, local, remote, pairingRoute) {
			break
		}
		allowed++
	}
	return allowed
}

func TestAPeerInPoorStandingSpendsLess(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	trusted, _ := newRemotePeer(t)
	degraded, _ := newRemotePeer(t)
	mw := newLimiterMiddlewareForTest(t, ownNodeId)

	mw.Apply(warpnet.PeerStanding{PeerID: degraded.String(), LimitMultiplier: 0.25})

	full := spend(t, mw, ownNodeId, trusted)
	tightened := spend(t, mw, ownNodeId, degraded)

	require.Positive(t, tightened, "a peer in poor standing is still served")
	assert.Less(t, tightened, full, "and served less than one nobody has rated")
}

func TestAPeerNobodyHasRatedSpendsEverything(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	peer, _ := newRemotePeer(t)
	mw := newLimiterMiddlewareForTest(t, ownNodeId)

	assert.Equal(t, float64(1), mw.allowance(peer.String()))
	assert.Equal(t, limitPairing, scale(limitPairing, mw.allowance(peer.String())))
}

// A peer that filled its bucket at the old allowance must not keep it.
func TestANewStandingDropsTheBucketsFilledUnderTheOldOne(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	peer, _ := newRemotePeer(t)
	mw := newLimiterMiddlewareForTest(t, ownNodeId)

	require.Positive(t, spend(t, mw, ownNodeId, peer), "the peer drains its bucket")
	require.Zero(t, spend(t, mw, ownNodeId, peer), "and it stays drained")

	mw.Apply(warpnet.PeerStanding{PeerID: peer.String(), LimitMultiplier: 0.5})

	assert.Positive(t, spend(t, mw, ownNodeId, peer),
		"a peer whose standing changed is measured against the allowance it has now")
}

func TestTheSameStandingTwiceKeepsTheBucket(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	peer, _ := newRemotePeer(t)
	mw := newLimiterMiddlewareForTest(t, ownNodeId)

	standing := warpnet.PeerStanding{PeerID: peer.String(), LimitMultiplier: 0.5}
	mw.Apply(standing)

	require.Positive(t, spend(t, mw, ownNodeId, peer))

	mw.Apply(standing) // unchanged: nothing to reset
	assert.Zero(t, spend(t, mw, ownNodeId, peer),
		"an unchanged standing must not hand a peer a fresh bucket")
}

func TestScaleNeverStarvesAPeer(t *testing.T) {
	tightest := scale(routeLimit{burst: 1, perMinute: 1}, 0.1)
	assert.EqualValues(t, 1, tightest.burst)
	assert.EqualValues(t, 1, tightest.perMinute)
}

func TestApplyIgnoresAStandingThatNamesNobody(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	mw := newLimiterMiddlewareForTest(t, ownNodeId)

	assert.NotPanics(t, func() { mw.Apply(warpnet.PeerStanding{LimitMultiplier: 0.1}) })

	var nilMiddleware *WarpMiddleware
	assert.NotPanics(t, func() { nilMiddleware.Apply(warpnet.PeerStanding{PeerID: "peer"}) })
}

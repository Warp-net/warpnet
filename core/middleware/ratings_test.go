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

// allowing answers the share of a route it was told a peer may spend. A
// peer it holds nothing about spends everything.
type allowing map[string]float64

func (a allowing) RateMultiplier(peerID warpnet.WarpPeerID) float64 {
	if multiplier, ok := a[peerID.String()]; ok {
		return multiplier
	}
	return 1
}

// pairingRoute has the tightest bucket, so a rating shows in it soonest.
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

func TestABadlyRatedPeerSpendsLess(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	trusted, _ := newRemotePeer(t)
	degraded, _ := newRemotePeer(t)
	mw := newLimiterMiddlewareForTest(t, ownNodeId)
	mw.ratings = allowing{degraded.String(): 0.25}

	full := spend(t, mw, ownNodeId, trusted)
	tightened := spend(t, mw, ownNodeId, degraded)

	require.Positive(t, tightened, "a badly rated peer is still served")
	assert.Less(t, tightened, full, "and served less than one nobody has rated")
}

func TestAPeerNobodyHasRatedSpendsEverything(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	peer, _ := newRemotePeer(t)
	mw := newLimiterMiddlewareForTest(t, ownNodeId)

	assert.Equal(t, float64(1), mw.rateMultiplier(peer))
	assert.Equal(t, limitPairing, limitPairing.multipliedBy(mw.rateMultiplier(peer)))
}

// A peer that filled its bucket at the old allowance must not keep it.
func TestANewRatingDropsTheBucketFilledUnderTheOldOne(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	peer, _ := newRemotePeer(t)
	mw := newLimiterMiddlewareForTest(t, ownNodeId)
	ratings := allowing{}
	mw.ratings = ratings

	require.Positive(t, spend(t, mw, ownNodeId, peer), "the peer drains its bucket")
	require.Zero(t, spend(t, mw, ownNodeId, peer), "and it stays drained")

	ratings[peer.String()] = 0.5

	assert.Positive(t, spend(t, mw, ownNodeId, peer),
		"a peer whose rating changed is measured against the allowance it has now")
}

func TestAnUnchangedRatingKeepsTheBucket(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	peer, _ := newRemotePeer(t)
	mw := newLimiterMiddlewareForTest(t, ownNodeId)
	mw.ratings = allowing{peer.String(): 0.5}

	require.Positive(t, spend(t, mw, ownNodeId, peer))

	assert.Zero(t, spend(t, mw, ownNodeId, peer),
		"a rating that did not change must not hand a peer a fresh bucket")
}

func TestAMultiplierNeverStarvesAPeer(t *testing.T) {
	tightest := routeLimit{burst: 1, perMinute: 1}.multipliedBy(0.1)
	assert.EqualValues(t, 1, tightest.burst)
	assert.EqualValues(t, 1, tightest.perMinute)
}

func TestAMiddlewareWithNoRatingsServesEveryPeerInFull(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	peer, _ := newRemotePeer(t)
	mw := &WarpMiddleware{ownNodeId: ownNodeId, rateLimiters: newRateLimitersCache()}
	t.Cleanup(func() { closeExpirableLRU(mw.rateLimiters) })

	assert.Equal(t, float64(1), mw.rateMultiplier(peer))
	assert.Positive(t, spend(t, mw, ownNodeId, peer))
}

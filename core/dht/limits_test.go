// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package dht

import (
	"context"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/stretchr/testify/assert"
)

const ratedPeer = "12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo"

func TestAPeerNobodyHasRatedIsAdmitted(t *testing.T) {
	d := NewDHTable(context.Background(), RoutingStore(memStore()), PeerLimits(warpnet.NewPeerLimiter()))

	assert.True(t, d.admits(warpnet.FromStringToPeerID(ratedPeer)),
		"the rating only ever takes peers out of the routing table")
}

func TestAPeerAtTheFloorIsKeptOutAndLetBackIn(t *testing.T) {
	limiter := warpnet.NewPeerLimiter()
	d := NewDHTable(context.Background(), RoutingStore(memStore()), PeerLimits(limiter))
	peerID := warpnet.FromStringToPeerID(ratedPeer)

	limiter.Limit(warpnet.PeerLimits{PeerID: ratedPeer, InRoutingTable: false})
	assert.False(t, d.admits(peerID))

	limiter.Limit(warpnet.PeerLimits{PeerID: ratedPeer, InRoutingTable: true})
	assert.True(t, d.admits(peerID), "a standing that recovers lets the peer back in")
}

func TestATableWithNoLimiterAdmitsEveryPeer(t *testing.T) {
	d := NewDHTable(context.Background(), RoutingStore(memStore()))

	assert.True(t, d.admits(warpnet.FromStringToPeerID(ratedPeer)))
}

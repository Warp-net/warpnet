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
	d := NewDHTable(context.Background(), RoutingStore(memStore()))

	assert.True(t, d.admits(warpnet.FromStringToPeerID(ratedPeer)),
		"the rating only ever takes peers out of the routing table")
}

func TestAPeerAtTheFloorIsKeptOutAndLetBackIn(t *testing.T) {
	d := NewDHTable(context.Background(), RoutingStore(memStore()))
	peerID := warpnet.FromStringToPeerID(ratedPeer)

	d.Apply(warpnet.PeerStanding{PeerID: ratedPeer, AllowedInDHT: false})
	assert.False(t, d.admits(peerID))

	d.Apply(warpnet.PeerStanding{PeerID: ratedPeer, AllowedInDHT: true})
	assert.True(t, d.admits(peerID), "a standing that recovers lets the peer back in")
}

func TestApplyIgnoresAStandingThatNamesNobody(t *testing.T) {
	d := NewDHTable(context.Background(), RoutingStore(memStore()))

	assert.NotPanics(t, func() { d.Apply(warpnet.PeerStanding{AllowedInDHT: false}) })
	assert.True(t, d.admits(warpnet.FromStringToPeerID(ratedPeer)))

	var nilTable *distributedHashTable
	assert.NotPanics(t, func() { nilTable.Apply(warpnet.PeerStanding{PeerID: ratedPeer}) })
	assert.True(t, nilTable.admits(warpnet.FromStringToPeerID(ratedPeer)))
}

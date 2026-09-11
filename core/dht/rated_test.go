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

// refusing admits every peer but the ones it was told to refuse.
type refusing map[string]bool

func (r refusing) InRoutingTable(peerID warpnet.WarpPeerID) bool {
	return !r[peerID.String()]
}

func TestAPeerNobodyHasRatedIsAdmitted(t *testing.T) {
	d := NewDHTable(context.Background(), RoutingStore(memStore()), Rated(refusing{}))

	assert.True(t, d.admits(warpnet.FromStringToPeerID(ratedPeer)),
		"the rating only ever takes peers out of the routing table")
}

func TestAPeerAtTheFloorIsKeptOut(t *testing.T) {
	refused := refusing{ratedPeer: true}
	d := NewDHTable(context.Background(), RoutingStore(memStore()), Rated(refused))
	peerID := warpnet.FromStringToPeerID(ratedPeer)

	assert.False(t, d.admits(peerID))

	refused[ratedPeer] = false
	assert.True(t, d.admits(peerID), "a rating that recovers lets the peer back in")
}

func TestATableWithNoRatingAdmitsEveryPeer(t *testing.T) {
	d := NewDHTable(context.Background(), RoutingStore(memStore()))

	assert.True(t, d.admits(warpnet.FromStringToPeerID(ratedPeer)))
}

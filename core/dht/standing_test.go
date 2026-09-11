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

// admits is what the routing table and query filters ask of a peer.
func admits(d *distributedHashTable, peerID warpnet.WarpPeerID) bool {
	return d.cfg.standings.Peer(peerID).AllowedInDHT
}

func TestAPeerNobodyHasRatedIsAdmitted(t *testing.T) {
	d := NewDHTable(context.Background(), RoutingStore(memStore()), Standings(warpnet.NewPeerStandings()))

	assert.True(t, admits(d, warpnet.FromStringToPeerID(ratedPeer)),
		"the rating only ever takes peers out of the routing table")
}

func TestAPeerAtTheFloorIsKeptOutAndLetBackIn(t *testing.T) {
	standings := warpnet.NewPeerStandings()
	d := NewDHTable(context.Background(), RoutingStore(memStore()), Standings(standings))
	peerID := warpnet.FromStringToPeerID(ratedPeer)

	standings.Apply(warpnet.PeerStanding{PeerID: ratedPeer, AllowedInDHT: false})
	assert.False(t, admits(d, peerID))

	standings.Apply(warpnet.PeerStanding{PeerID: ratedPeer, AllowedInDHT: true})
	assert.True(t, admits(d, peerID), "a standing that recovers lets the peer back in")
}

func TestATableWithNoStandingsAdmitsEveryPeer(t *testing.T) {
	d := NewDHTable(context.Background(), RoutingStore(memStore()))

	assert.True(t, admits(d, warpnet.FromStringToPeerID(ratedPeer)))
}

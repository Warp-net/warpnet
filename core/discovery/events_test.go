// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reported drains what discovery said about its peers.
func reported(t *testing.T, s *discoveryService) []warpnet.PeerEvent {
	t.Helper()
	var out []warpnet.PeerEvent
	for {
		select {
		case ev := <-s.Event():
			out = append(out, ev)
		default:
			return out
		}
	}
}

func TestEverySightingIsReported(t *testing.T) {
	s, _, _, _ := newService(t)
	pi := warpnet.WarpAddrInfo{ID: warpnet.FromStringToPeerID(peerID)}

	for range 3 {
		s.enqueue(pi, sourceGossip)
	}

	events := reported(t, s)
	require.Len(t, events, 3, "how often a peer may turn up is the rating's call, not discovery's")
	for _, ev := range events {
		assert.Equal(t, warpnet.PeerDiscovered, ev.Type)
		assert.Equal(t, peerID, ev.PeerID)
	}
}

func TestOurOwnSightingIsNotReported(t *testing.T) {
	s, _, _, _ := newService(t)

	s.enqueue(warpnet.WarpAddrInfo{ID: s.ownId}, sourceDHT)
	s.enqueue(warpnet.WarpAddrInfo{}, sourceMDNS)

	assert.Empty(t, reported(t, s), "a node neither discovers nor rates itself")
}

// dialled is a sighting that carries an address, so a failure to reach it
// is the peer's, not ours.
func dialled(t *testing.T, id string) discoveredPeer {
	t.Helper()
	addr, err := warpnet.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	require.NoError(t, err)
	peer := discovered(id)
	peer.Addrs = []warpnet.WarpAddress{addr}
	return peer
}

func TestAPeerThatWillNotAnswerAKnownAddressIsReported(t *testing.T) {
	s, node, _, _ := newService(t)
	node.connectErr = warpnet.ErrAllDialsFailed

	s.handleAsMember(dialled(t, peerID))

	events := reported(t, s)
	require.NotEmpty(t, events)
	last := events[len(events)-1]
	assert.Equal(t, warpnet.PeerDialFailure, last.Type)
	assert.Equal(t, peerID, last.PeerID)
}

// A peer we hold no address for was never dialled, so it owes nothing.
func TestAPeerWithNoAddressIsNotReportedForADialFailure(t *testing.T) {
	s, node, _, _ := newService(t)
	node.connectErr = warpnet.ErrAllDialsFailed

	s.emitDialFailure(warpnet.WarpAddrInfo{ID: warpnet.FromStringToPeerID(peerID)})

	assert.Empty(t, reported(t, s))
}

func TestARelayReportsItsPeersToo(t *testing.T) {
	s, node, _, _ := newService(t)
	node.connectErr = warpnet.ErrAllDialsFailed

	s.handleAsRelay(dialled(t, peerID))

	events := reported(t, s)
	require.NotEmpty(t, events)
	assert.Equal(t, warpnet.PeerDialFailure, events[len(events)-1].Type)
}

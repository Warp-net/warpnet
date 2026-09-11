// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package warpnet

import (
	"time"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeerEmitterCarriesTheObservation(t *testing.T) {
	events := NewPeerEmitter()

	events.Emit(PeerEvent{PeerID: "peer", Type: PeerBadSignature, Route: "/public/post/tweet/0.0.0"})

	require.Len(t, events, 1)
	assert.Equal(t, PeerEvent{
		PeerID: "peer",
		Type:   PeerBadSignature,
		Route:  "/public/post/tweet/0.0.0",
	}, <-events)
}

func TestPeerEmitterIgnoresAnObservationThatNamesNobody(t *testing.T) {
	events := NewPeerEmitter()

	events.Emit(PeerEvent{Type: PeerBadSignature})

	assert.Empty(t, events, "an observation with no peer charges nobody")
}

// A module must never wait on the rating: past the buffer an observation
// is dropped, and the module carries on.
func TestPeerEmitterDropsRatherThanBlock(t *testing.T) {
	events := NewPeerEmitter()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range cap(events) + 100 {
			events.Emit(PeerEvent{PeerID: "peer", Type: PeerRateLimited})
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Emit blocked once the fan-out was full")
	}
	assert.Len(t, events, cap(events))
}

func TestNilPeerEmitterIsSafe(t *testing.T) {
	var events PeerEmitter
	assert.NotPanics(t, func() {
		events.Emit(PeerEvent{PeerID: "peer", Type: PeerBadSignature})
	})
}

func TestPeerEventTypesAreDistinct(t *testing.T) {
	seen := make(map[PeerEventType]struct{})
	for _, evType := range []PeerEventType{
		PeerBadSignature, PeerMissingSignature, PeerMalformedFrame, PeerOversizePayload,
		PeerStaleMessage, PeerPrivateRouteDenied, PeerRateLimited, PeerDialFailure,
		PeerConnected, PeerDiscovered, PeerModerationUpheld, PeerForeignAuthorship,
		PeerVerdictMalformed, PeerAuditWrong, PeerAuditInvalid, PeerAuditUnreachable,
	} {
		assert.NotEmpty(t, evType)
		_, dup := seen[evType]
		assert.False(t, dup, "%s is used twice", evType)
		seen[evType] = struct{}{}
	}
}

func TestAPeerNobodyHasRatedStandsWell(t *testing.T) {
	standings := NewPeerStandings()
	peerID := FromStringToPeerID("12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo")

	standing := standings.Peer(peerID)
	assert.Equal(t, float64(1), standing.LimitMultiplier, "it spends its whole allowance")
	assert.True(t, standing.AllowedInDHT, "and keeps its place in the routing table")
	assert.Zero(t, standing.GossipScore)
	assert.Zero(t, standing.ConnTag)
}

func TestAStandingIsRememberedUntilItIsReplaced(t *testing.T) {
	standings := NewPeerStandings()
	peerID := FromStringToPeerID("12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo")

	standings.Apply(PeerStanding{
		PeerID: peerID.String(), ConnTag: 10, GossipScore: -60, LimitMultiplier: 0.25,
	})
	assert.Equal(t, 10, standings.Peer(peerID).ConnTag)
	assert.Equal(t, float64(-60), standings.Peer(peerID).GossipScore)
	assert.Equal(t, 0.25, standings.Peer(peerID).LimitMultiplier)
	assert.False(t, standings.Peer(peerID).AllowedInDHT)

	standings.Apply(PeerStanding{
		PeerID: peerID.String(), ConnTag: 60, LimitMultiplier: 1, AllowedInDHT: true,
	})
	assert.Equal(t, 60, standings.Peer(peerID).ConnTag, "a standing that recovers replaces the old one")
	assert.True(t, standings.Peer(peerID).AllowedInDHT)
}

func TestAStandingThatNamesNobodyIsIgnored(t *testing.T) {
	standings := NewPeerStandings()

	standings.Apply(PeerStanding{LimitMultiplier: 0.1})

	assert.Equal(t, float64(1), standings.Peer("").LimitMultiplier)
}

func TestNilPeerStandingsStandsEveryPeerWell(t *testing.T) {
	var standings *PeerStandings
	peerID := FromStringToPeerID("12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo")

	assert.NotPanics(t, func() {
		standings.Apply(PeerStanding{PeerID: peerID.String(), LimitMultiplier: 0.1})
	})
	assert.Equal(t, float64(1), standings.Peer(peerID).LimitMultiplier)
	assert.True(t, standings.Peer(peerID).AllowedInDHT)
}

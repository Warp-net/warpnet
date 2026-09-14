// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package audit

import (
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// concluded drains what the audit reported about the moderators it probed.
func concluded(t *testing.T, events warpnet.PeerEmitter) []warpnet.PeerEventType {
	t.Helper()
	var out []warpnet.PeerEventType
	for {
		select {
		case ev := <-events:
			assert.Equal(t, moderatorPeer, ev.PeerID)
			out = append(out, ev.Type)
		default:
			return out
		}
	}
}

const moderatorPeer = "12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo"

func TestAnHonestModeratorIsNeverReported(t *testing.T) {
	events := warpnet.NewPeerEmitter()
	ledger := NewLedger(events)

	for range 50 {
		ledger.Record(moderatorPeer, OutcomeCorrect)
	}

	require.Equal(t, StandingTrusted, ledger.StandingOf(moderatorPeer))
	assert.Empty(t, concluded(t, events), "answering correctly says nothing about a peer")
}

func TestAWorseningStandingIsReportedOncePerCrossing(t *testing.T) {
	events := warpnet.NewPeerEmitter()
	ledger := NewLedger(events)

	for range 100 {
		ledger.Record(moderatorPeer, OutcomeWrong)
	}

	require.Equal(t, StandingBanned, ledger.StandingOf(moderatorPeer))

	reported := concluded(t, events)
	assert.Contains(t, reported, warpnet.PeerAuditInvalid, "the ban line must be reported")
	assert.LessOrEqual(t, countOf(reported, warpnet.PeerAuditInvalid), 1,
		"a long audit must not grind a peer down for a conclusion it had already drawn")
	assert.LessOrEqual(t, countOf(reported, warpnet.PeerAuditWrong), 1)
}

func TestAnInvalidAnswerCrossesStraightToTheBanLine(t *testing.T) {
	events := warpnet.NewPeerEmitter()
	ledger := NewLedger(events)

	ledger.Record(moderatorPeer, OutcomeInvalid)
	ledger.Record(moderatorPeer, OutcomeInvalid)

	require.Equal(t, StandingBanned, ledger.StandingOf(moderatorPeer))
	assert.Equal(t, []warpnet.PeerEventType{warpnet.PeerAuditInvalid}, concluded(t, events))
}

func TestEveryUnreachableProbeIsReportedAndNoneBans(t *testing.T) {
	events := warpnet.NewPeerEmitter()
	ledger := NewLedger(events)

	for range 5 {
		ledger.Record(moderatorPeer, OutcomeUnreachable)
	}

	reported := concluded(t, events)
	assert.Equal(t, 5, countOf(reported, warpnet.PeerAuditUnreachable),
		"being unreachable is reported every time: it is cheap and capped in the rating")
	assert.NotEqual(t, StandingBanned, ledger.StandingOf(moderatorPeer),
		"silence alone must never ban a moderator")
}

func TestALedgerWithNoFanOutIsSafe(t *testing.T) {
	ledger := NewLedger(nil)
	assert.NotPanics(t, func() {
		ledger.Record(moderatorPeer, OutcomeInvalid)
		ledger.Record(moderatorPeer, OutcomeInvalid)
	})
	assert.Equal(t, StandingBanned, ledger.StandingOf(moderatorPeer),
		"a ledger nobody listens to still keeps its own count")
}

func countOf(in []warpnet.PeerEventType, want warpnet.PeerEventType) int {
	var n int
	for _, got := range in {
		if got == want {
			n++
		}
	}
	return n
}

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package moderator

import (
	"testing"

	"github.com/Warp-net/warpnet/cmd/node/moderator/audit"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ruled drains what this moderator reported about the nodes it judged.
func ruled(t *testing.T, m *Moderator) []warpnet.PeerEvent {
	t.Helper()
	var out []warpnet.PeerEvent
	for {
		select {
		case ev := <-m.Event():
			out = append(out, ev)
		default:
			return out
		}
	}
}

func decidedModerator(t *testing.T) *Moderator {
	t.Helper()
	privKey, err := security.GenerateKeyFromSeed([]byte("decided-reports"))
	require.NoError(t, err)
	m, _, _, _ := decidedFixture(t, privKey)
	return m
}

// The moderator that carried the round reports the offending node, so one
// verdict is one observation rather than one from every node it reaches.
func TestAnUpheldReportChargesTheNodeThatHostsTheOffender(t *testing.T) {
	m := decidedModerator(t)
	rep := tweetReport("tweet-1")

	m.Decided(rep, decidedBallot(domain.FAIL, "Hate"), []domain.ID{"mod-a"})

	events := ruled(t, m)
	require.Len(t, events, 1)
	assert.Equal(t, warpnet.PeerModerationUpheld, events[0].Type)
	assert.Equal(t, rep.TargetNodeID, events[0].PeerID,
		"the report already names the node the offending user lives on")
}

func TestAClearedReportChargesNobody(t *testing.T) {
	m := decidedModerator(t)

	m.Decided(tweetReport("tweet-1"), decidedBallot(domain.OK, ""), []domain.ID{"mod-a"})

	assert.Empty(t, ruled(t, m), "a report the quorum cleared is not an offence")
}

func TestAReportThatNamesNoNodeChargesNobody(t *testing.T) {
	m := decidedModerator(t)
	rep := tweetReport("tweet-1")
	rep.TargetNodeID = ""

	m.Decided(rep, decidedBallot(domain.FAIL, "Hate"), []domain.ID{"mod-a"})

	assert.Empty(t, ruled(t, m))
}

// The audit reports into the same fan-out, so the node listens once.
func TestTheAuditReportsOnTheSameFanOut(t *testing.T) {
	m := decidedModerator(t)

	m.ledger.Record("12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo", audit.OutcomeUnreachable)

	events := ruled(t, m)
	require.Len(t, events, 1)
	assert.Equal(t, warpnet.PeerAuditUnreachable, events[0].Type)
}

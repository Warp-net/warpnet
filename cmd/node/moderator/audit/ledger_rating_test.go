// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package audit

import (
	"sync"
	"testing"

	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturingReporter struct {
	mu    sync.Mutex
	kinds []rating.Kind
}

func (c *capturingReporter) Record(_ warpnet.WarpPeerID, k rating.Kind) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.kinds = append(c.kinds, k)
}

func (c *capturingReporter) seen() []rating.Kind {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]rating.Kind(nil), c.kinds...)
}

func (c *capturingReporter) count(want rating.Kind) int {
	n := 0
	for _, k := range c.seen() {
		if k == want {
			n++
		}
	}
	return n
}

func ratedPeer(t *testing.T) string {
	t.Helper()
	id := warpnet.FromStringToPeerID("12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j")
	require.NotEmpty(t, id)
	return id.String()
}

func TestHonestPeerIsNeverReported(t *testing.T) {
	reporter := &capturingReporter{}
	ledger := NewLedger(reporter)
	peer := ratedPeer(t)

	for range 54 {
		ledger.Record(peer, OutcomeCorrect)
	}
	for range 6 { // 90% agreement
		ledger.Record(peer, OutcomeWrong)
	}

	require.Equal(t, StandingTrusted, ledger.StandingOf(peer))
	assert.Empty(t, reporter.seen(), "an honest peer must produce no observations at all")
}

func TestCoinFlippingBotIsReported(t *testing.T) {
	reporter := &capturingReporter{}
	ledger := NewLedger(reporter)
	peer := ratedPeer(t)

	for range 30 {
		ledger.Record(peer, OutcomeCorrect)
		ledger.Record(peer, OutcomeWrong)
	}

	require.Equal(t, StandingBanned, ledger.StandingOf(peer))
	assert.Equal(t, 1, reporter.count(rating.KindAuditInvalid))
	assert.Zero(t, reporter.count(rating.KindAuditWrong))
}

func TestMildlyDisagreeingPeerIsReportedAsSuspectOnly(t *testing.T) {
	reporter := &capturingReporter{}
	ledger := NewLedger(reporter)
	peer := ratedPeer(t)

	for range 15 {
		ledger.Record(peer, OutcomeCorrect)
		ledger.Record(peer, OutcomeCorrect)
		ledger.Record(peer, OutcomeCorrect)
		ledger.Record(peer, OutcomeWrong)
	}

	require.Equal(t, StandingSuspect, ledger.StandingOf(peer))
	assert.Equal(t, 1, reporter.count(rating.KindAuditWrong))
	assert.Zero(t, reporter.count(rating.KindAuditInvalid))
}

func TestAStandingIsReportedOnlyOnce(t *testing.T) {
	reporter := &capturingReporter{}
	ledger := NewLedger(reporter)
	peer := ratedPeer(t)

	for range 200 {
		ledger.Record(peer, OutcomeWrong)
	}

	require.Equal(t, StandingBanned, ledger.StandingOf(peer))
	assert.Equal(t, 1, reporter.count(rating.KindAuditInvalid))
	assert.LessOrEqual(t, reporter.count(rating.KindAuditWrong), 1)
}

func TestInvalidAnswersReportImmediately(t *testing.T) {
	reporter := &capturingReporter{}
	ledger := NewLedger(reporter)
	peer := ratedPeer(t)

	ledger.Record(peer, OutcomeInvalid)
	ledger.Record(peer, OutcomeInvalid)

	require.Equal(t, StandingBanned, ledger.StandingOf(peer))
	assert.Equal(t, 1, reporter.count(rating.KindAuditInvalid))
}

func TestUnreachableIsReportedEveryTimeAndCheaply(t *testing.T) {
	reporter := &capturingReporter{}
	ledger := NewLedger(reporter)
	peer := ratedPeer(t)

	for range 5 {
		ledger.Record(peer, OutcomeUnreachable)
	}

	assert.Equal(t, 5, reporter.count(rating.KindAuditUnreachable))
	assert.NotEqual(t, StandingBanned, ledger.StandingOf(peer),
		"liveness alone must never ban")
}

func TestNilReporterIsSafe(t *testing.T) {
	ledger := NewLedger(nil)
	assert.NotPanics(t, func() {
		ledger.Record(ratedPeer(t), OutcomeInvalid)
	})
}

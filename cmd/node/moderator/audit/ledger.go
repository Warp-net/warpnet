// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package audit

import (
	"sync"

	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/warpnet"
	log "github.com/sirupsen/logrus"
)

// Outcome is the auditor's classification of one challenge exchange.
type Outcome int

const (
	// OutcomeCorrect: answered with the expected verdict class.
	OutcomeCorrect Outcome = iota
	// OutcomeWrong: answered, cryptographically valid, but with the wrong
	// class on a flagrant probe. One of these is model noise; a pattern is
	// a fake — the thresholds below draw that line.
	OutcomeWrong
	// OutcomeUnreachable: no usable answer (transport error, timeout, or
	// an error reply). Could be the network's fault, so it degrades
	// standing only in bulk and never to Banned.
	OutcomeUnreachable
	// OutcomeInvalid: an answer that is wrong cryptographically or
	// protocol-wise — bad signature, foreign responder id, mismatched
	// challenge binding. Never accidental; two of these ban the peer.
	OutcomeInvalid
)

// Standing is the local trust state derived from a peer's audit history.
type Standing int

const (
	// StandingProbation: not enough evidence yet (the "trainee" period —
	// fresh identities must earn weight before their votes should count).
	StandingProbation Standing = iota
	StandingTrusted
	StandingSuspect
	StandingBanned
)

func (s Standing) String() string {
	switch s {
	case StandingProbation:
		return "probation"
	case StandingTrusted:
		return "trusted"
	case StandingSuspect:
		return "suspect"
	case StandingBanned:
		return "banned"
	default:
		return "unknown"
	}
}

// Tolerance thresholds. Moderators legitimately run different models, so
// agreement is judged statistically on flagrant probes only. On the
// balanced corpus an honest moderator — even an exotic model disagreeing
// on one probe in ten — sits near 0.9, while anything without a working
// model (constant answers, coin flips) converges to ~0.5. The ban line is
// drawn between those two populations, not at "one mistake".
const (
	// minSample is how many ANSWERED probes it takes to leave probation.
	minSample = 20
	// banAgreeBelow: no better than guessing on flagrant content, i.e. no
	// usable moderation model behind the votes.
	banAgreeBelow = 0.65
	// suspectAgreeBelow: more disagreement than model diversity explains.
	suspectAgreeBelow = 0.8
	// refusalSuspectAbove: mostly silent peers are suspect, never banned —
	// the network may be at fault, and liveness alone must not execute.
	refusalSuspectAbove = 0.5
	// maxInvalid tolerated before banning; invalid responses are deliberate.
	maxInvalid = 1
)

type peerStats struct {
	correct     int
	wrong       int
	unreachable int
	invalid     int
}

func (s *peerStats) standing() Standing {
	if s.invalid > maxInvalid {
		return StandingBanned
	}
	answered := s.correct + s.wrong
	asked := answered + s.unreachable + s.invalid
	if asked >= minSample && float64(s.unreachable) > refusalSuspectAbove*float64(asked) {
		return StandingSuspect
	}
	if answered < minSample {
		return StandingProbation
	}
	rate := float64(s.correct) / float64(answered)
	switch {
	case rate < banAgreeBelow:
		return StandingBanned
	case rate < suspectAgreeBelow:
		return StandingSuspect
	default:
		return StandingTrusted
	}
}

// PeerReport is a read-only snapshot row, the shape a future reputation
// gossip or admin surface would consume.
type PeerReport struct {
	Correct     int
	Wrong       int
	Unreachable int
	Invalid     int
	Standing    Standing
}

// Ledger accumulates audit outcomes per moderator peer. In-memory and
// local-first by design: every node judges from its own evidence; sharing
// signed transcripts across nodes is a later layer.
//
// The statistical judgement stays here rather than moving into the
// rating store, and that is deliberate. Audit quality is a rate —
// agreement over many probes — while the rating counts discrete
// offences, and a count cannot tell "six wrong out of sixty" from "six
// wrong out of six". So the ledger keeps the tolerance encoded in the
// thresholds above and reports to the rating only when a peer crosses
// one, once per crossing.
type Ledger struct {
	reporter rating.Reporter

	mu       sync.Mutex
	peers    map[string]*peerStats
	reported map[string]Standing
}

func NewLedger(reporter rating.Reporter) *Ledger {
	if reporter == nil {
		reporter = rating.Nop{}
	}
	return &Ledger{
		reporter: reporter,
		peers:    make(map[string]*peerStats),
		reported: make(map[string]Standing),
	}
}

func (l *Ledger) Record(peerID string, o Outcome) {
	if peerID == "" {
		return
	}

	l.mu.Lock()
	s, ok := l.peers[peerID]
	if !ok {
		s = &peerStats{}
		l.peers[peerID] = s
	}
	switch o {
	case OutcomeCorrect:
		s.correct++
	case OutcomeWrong:
		s.wrong++
	case OutcomeUnreachable:
		s.unreachable++
	case OutcomeInvalid:
		s.invalid++
	}

	standing := s.standing()
	worsened := standing != l.reported[peerID] && severity(standing) > severity(l.reported[peerID])
	if worsened {
		l.reported[peerID] = standing
	}
	l.mu.Unlock()

	// An unreachable peer may just be behind a bad link, so it is
	// reported every time and weighs almost nothing; a standing is
	// reported only when it first worsens.
	if o == OutcomeUnreachable {
		l.report(peerID, rating.KindAuditUnreachable)
	}
	if worsened {
		if kind, ok := standingKind(standing); ok {
			l.report(peerID, kind)
		}
	}
}

// report files one audit conclusion with the rating. A refused record
// means this node reported something its role cannot witness, which is
// a bug here rather than misbehaviour by the peer, and the audit loop
// has nowhere to return it to.
func (l *Ledger) report(peerID string, kind rating.Kind) {
	id := warpnet.FromStringToPeerID(peerID)
	if id == "" {
		return
	}
	if err := l.reporter.Record(id, kind); err != nil {
		log.Warnf("audit: rating %s for %s: %v", kind, peerID, err)
	}
}

// severity orders standings so only a genuine downgrade is reported.
func severity(s Standing) int {
	switch s {
	case StandingTrusted, StandingProbation:
		return 0
	case StandingSuspect:
		return 1
	case StandingBanned:
		return 2 //nolint:mnd
	default:
		return 0
	}
}

// standingKind maps a worsened standing to what the rating records.
// Probation and Trusted report nothing: an unproven peer is not an
// offending one, and the rating starts everyone at full trust by
// design.
func standingKind(s Standing) (rating.Kind, bool) {
	switch s {
	case StandingSuspect:
		return rating.KindAuditWrong, true
	case StandingBanned:
		return rating.KindAuditInvalid, true
	default:
		return 0, false
	}
}

// StandingOf reports the local trust state; peers never audited are on
// probation, not trusted.
func (l *Ledger) StandingOf(peerID string) Standing {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.peers[peerID]
	if !ok {
		return StandingProbation
	}
	return s.standing()
}

func (l *Ledger) Snapshot() map[string]PeerReport {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]PeerReport, len(l.peers))
	for id, s := range l.peers {
		out[id] = PeerReport{
			Correct:     s.correct,
			Wrong:       s.wrong,
			Unreachable: s.unreachable,
			Invalid:     s.invalid,
			Standing:    s.standing(),
		}
	}
	return out
}

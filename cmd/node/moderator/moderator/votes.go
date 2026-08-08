/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package moderator

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"time"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	log "github.com/sirupsen/logrus"
)

// Report round machinery. Every moderator sees every report on the gossip
// topic, but hash-ordered volunteer timers keep the actual LLM work bounded
// to about quorumTarget nodes regardless of how many moderators run: a
// moderator whose timer fires after the round already holds enough votes
// stays silent and spends nothing. The chair (lowest pair hash among actual
// voters) is the only node that publishes the aggregate verdict, so the
// reporter hears exactly one answer.
const quorumTarget = 3

var (
	// voteWindow is how long a round collects votes before the tally.
	voteWindow = 30 * time.Second
	// voteDelayStep spaces the volunteer timers of moderators ranked
	// below the quorum target; one step must fit a fetch + inference +
	// gossip propagation so a suppressed rank never starts work.
	voteDelayStep = 8 * time.Second
	// failoverDelay spaces the takeover chain of the round's voters: the
	// voter at kept-rank k finalizes at k*failoverDelay unless a Final
	// announcement arrives first. One step must comfortably fit gossip
	// propagation of that announcement.
	failoverDelay = 10 * time.Second
)

const (
	// finalizedTTL guards against gossip re-deliveries reopening a
	// finished round.
	finalizedTTL = time.Hour
	// seenModTTL bounds the passive moderator-population estimate used
	// only to scale volunteer delays; no trust decision reads it.
	seenModTTL = 24 * time.Hour
)

type voteRound struct {
	report     *event.ReportEvent
	votes      map[string]event.ModerationVoteEvent
	voteTimer  *time.Timer
	tallyTimer *time.Timer
	// pending holds the tallied outcome on a backup voter that is waiting
	// for the chair's Final announcement before taking over.
	pending    *pendingFinalize
	finalTimer *time.Timer
}

// pendingFinalize is a tallied round outcome parked on a backup voter.
type pendingFinalize struct {
	report event.ReportEvent
	agg    verdict
	voters []domain.ID
}

func (r *voteRound) stopTimersLocked() {
	if r.voteTimer != nil {
		r.voteTimer.Stop()
	}
	if r.tallyTimer != nil {
		r.tallyTimer.Stop()
	}
	if r.finalTimer != nil {
		r.finalTimer.Stop()
	}
}

// pairHash orders (round, moderator) pairs: it drives the volunteer delay,
// the chair choice and the odd-count trim, so every moderator derives the
// same ranking with no coordination.
func pairHash(reportID, moderatorID string) uint64 {
	h := sha256.Sum256([]byte(reportID + "|" + moderatorID))
	return binary.BigEndian.Uint64(h[:8])
}

func (m *Moderator) openRound(rep event.ReportEvent) {
	id := rep.ReportID()

	m.mx.Lock()
	defer m.mx.Unlock()
	if m.isFinalizedLocked(id) {
		return
	}
	r := m.ensureRoundLocked(id)
	if r.report == nil {
		r.report = &rep
	}
	if r.voteTimer == nil {
		r.voteTimer = time.AfterFunc(m.voteDelayLocked(id), func() { m.castVote(id) })
	}
}

// ensureRoundLocked returns the round, creating it with a running tally
// timer. Rounds can be opened by a report or by a vote that arrived first.
func (m *Moderator) ensureRoundLocked(id string) *voteRound {
	r, ok := m.rounds[id]
	if !ok {
		r = &voteRound{votes: make(map[string]event.ModerationVoteEvent)}
		r.tallyTimer = time.AfterFunc(voteWindow, func() { m.closeRound(id) })
		m.rounds[id] = r
	}
	return r
}

func (m *Moderator) isFinalizedLocked(id string) bool {
	ts, ok := m.finalized[id]
	if !ok {
		return false
	}
	if time.Since(ts) > finalizedTTL {
		delete(m.finalized, id)
		return false
	}
	return true
}

// voteDelayLocked maps this moderator's deterministic rank for the round
// onto a start delay: the estimated top-quorumTarget ranks start at once,
// everyone below waits in voteDelayStep increments and will usually find
// the round already served when their timer fires.
func (m *Moderator) voteDelayLocked(id string) time.Duration {
	now := time.Now()
	for k, ts := range m.seenMods {
		if now.Sub(ts) > seenModTTL {
			delete(m.seenMods, k)
		}
	}
	self := m.node.ID().String()
	population := len(m.seenMods)
	if _, ok := m.seenMods[self]; !ok {
		population++
	}
	u := float64(pairHash(id, self)>>11) / float64(1<<53) // uniform [0,1)
	rank := int(u * float64(population))
	if rank < quorumTarget {
		return 0
	}
	return time.Duration(rank-quorumTarget+1) * voteDelayStep
}

// castVote runs when this moderator's volunteer timer fires: unless the
// round is already served, assess the content and publish the vote.
func (m *Moderator) castVote(id string) {
	if m.isClosed.Load() {
		return
	}
	m.mx.Lock()
	r, ok := m.rounds[id]
	if !ok || r.report == nil {
		m.mx.Unlock()
		return
	}
	if len(r.votes) >= quorumTarget {
		m.mx.Unlock()
		return
	}
	rep := *r.report
	m.mx.Unlock()

	v, ok, err := m.assessReport(rep)
	if err != nil {
		log.Errorf("moderator: assess report %s: %v", id, err)
		return
	}
	if !ok {
		return
	}

	vote := event.ModerationVoteEvent{
		ReportID:    id,
		Type:        rep.Type,
		Result:      v.result,
		Reason:      v.reason,
		UserID:      v.userID,
		ObjectID:    v.objectID,
		ModeratorID: m.node.ID().String(),
	}
	// Record the own vote before publishing: gossip loopback delivery is
	// not guaranteed (recordVote dedups if it does loop back), and the
	// vote must count even when the publish fails.
	m.recordVote(vote)
	if err := m.votes.PublishVote(vote); err != nil {
		log.Errorf("moderator: publish vote %s: %v", id, err)
	}
}

func (m *Moderator) handleVote(ev event.ModerationVoteEvent) error {
	if m.isClosed.Load() {
		return nil
	}
	if ev.ReportID == "" || ev.ModeratorID == "" {
		return nil
	}
	if ev.Final {
		m.handleRoundFinal(ev)
		return nil
	}
	m.recordVote(ev)
	return nil
}

// handleRoundFinal cancels this voter's takeover chain: someone ahead of it
// (the chair or an earlier backup) already finalized the round.
func (m *Moderator) handleRoundFinal(ev event.ModerationVoteEvent) {
	m.mx.Lock()
	defer m.mx.Unlock()
	m.seenMods[ev.ModeratorID] = time.Now()
	if m.isFinalizedLocked(ev.ReportID) {
		return
	}
	m.finalized[ev.ReportID] = time.Now()
	if r, ok := m.rounds[ev.ReportID]; ok {
		r.stopTimersLocked()
		delete(m.rounds, ev.ReportID)
	}
	log.Infof("moderator: round %s finalized by %s", ev.ReportID, ev.ModeratorID)
}

func (m *Moderator) recordVote(vote event.ModerationVoteEvent) {
	m.mx.Lock()
	defer m.mx.Unlock()
	m.seenMods[vote.ModeratorID] = time.Now()
	if m.isFinalizedLocked(vote.ReportID) {
		return
	}
	r := m.ensureRoundLocked(vote.ReportID)
	if _, dup := r.votes[vote.ModeratorID]; dup {
		return
	}
	r.votes[vote.ModeratorID] = vote
}

// closeRound tallies the collected votes when the window elapses. Every
// moderator that saw the round runs the same tally: the chair (kept-rank 0)
// finalizes at once, every other voter parks the outcome and takes over at
// rank*failoverDelay unless a Final announcement lands first, non-voters
// just remember the round as spent.
func (m *Moderator) closeRound(id string) {
	if m.isClosed.Load() {
		return
	}
	m.mx.Lock()
	r, ok := m.rounds[id]
	if !ok {
		m.mx.Unlock()
		return
	}
	if r.voteTimer != nil {
		r.voteTimer.Stop()
	}
	for k, ts := range m.finalized {
		if time.Since(ts) > finalizedTTL {
			delete(m.finalized, k)
		}
	}
	rep := r.report
	ordered := sortedVotes(id, r.votes)
	kept := trimEven(ordered)

	// The takeover chain ranks over the full voter order, pre-trim: a
	// voter dropped by the odd-count trim still holds everything needed
	// to finalize the kept tally if the chair dies.
	self := m.node.ID().String()
	myRank := -1
	for i, v := range ordered {
		if v.ModeratorID == self {
			myRank = i
			break
		}
	}

	// A voter always holds the report (voting requires it); a self-named
	// vote without one means someone forged votes for a round this node
	// never saw. Both drop out of the takeover chain like a non-voter.
	if len(kept) == 0 || myRank < 0 || rep == nil {
		if myRank >= 0 && rep == nil {
			log.Warnf("moderator: round %s: voter without report, skipping finalize", id)
		}
		delete(m.rounds, id)
		m.finalized[id] = time.Now()
		m.mx.Unlock()
		return
	}

	agg, voters := aggregate(kept)
	chair := kept[0].ModeratorID

	if myRank > 0 {
		r.pending = &pendingFinalize{report: *rep, agg: agg, voters: voters}
		r.finalTimer = time.AfterFunc(time.Duration(myRank)*failoverDelay, func() { m.tryFinalize(id) })
		m.mx.Unlock()
		log.Infof("moderator: round %s closed votes=%d result=%t chair=%s (standing by at rank %d)",
			id, len(kept), bool(agg.result), chair, myRank)
		return
	}

	delete(m.rounds, id)
	m.finalized[id] = time.Now()
	m.mx.Unlock()

	log.Infof("moderator: round %s closed votes=%d result=%t chair=%s", id, len(kept), bool(agg.result), chair)
	m.finalizeRound(*rep, agg, voters)
	m.publishFinal(id, *rep, agg)
}

// tryFinalize fires on a backup voter when its takeover slot elapses with no
// Final announcement: the chair and every earlier backup stayed silent, so
// this voter finalizes the round itself.
func (m *Moderator) tryFinalize(id string) {
	if m.isClosed.Load() {
		return
	}
	m.mx.Lock()
	r, ok := m.rounds[id]
	if !ok || r.pending == nil || m.isFinalizedLocked(id) {
		m.mx.Unlock()
		return
	}
	p := r.pending
	r.stopTimersLocked()
	delete(m.rounds, id)
	m.finalized[id] = time.Now()
	m.mx.Unlock()

	log.Warnf("moderator: round %s: chair stayed silent, taking over finalization", id)
	m.finalizeRound(p.report, p.agg, p.voters)
	m.publishFinal(id, p.report, p.agg)
}

// publishFinal announces on the votes topic that the round was finalized,
// cancelling the takeover chain on the other voters.
func (m *Moderator) publishFinal(id string, rep event.ReportEvent, agg verdict) {
	final := event.ModerationVoteEvent{
		ReportID:    id,
		Type:        rep.Type,
		Result:      agg.result,
		Reason:      agg.reason,
		UserID:      agg.userID,
		ObjectID:    agg.objectID,
		ModeratorID: m.node.ID().String(),
		Final:       true,
	}
	if err := m.votes.PublishVote(final); err != nil {
		log.Errorf("moderator: publish final %s: %v", id, err)
	}
}

// sortedVotes orders a round's votes by their deterministic pair hash, so
// every moderator derives the identical ranking with no coordination.
func sortedVotes(id string, votes map[string]event.ModerationVoteEvent) []event.ModerationVoteEvent {
	ordered := make([]event.ModerationVoteEvent, 0, len(votes))
	for _, v := range votes {
		ordered = append(ordered, v)
	}
	sort.Slice(ordered, func(i, j int) bool {
		return pairHash(id, ordered[i].ModeratorID) < pairHash(id, ordered[j].ModeratorID)
	})
	return ordered
}

// trimEven drops the highest-ranked vote of an even count so a strict
// majority always exists in the tally.
func trimEven(ordered []event.ModerationVoteEvent) []event.ModerationVoteEvent {
	if len(ordered) > 0 && len(ordered)%2 == 0 {
		return ordered[:len(ordered)-1]
	}
	return ordered
}

// keptVotes is the tally set: hash-ordered votes trimmed to an odd count.
func keptVotes(id string, votes map[string]event.ModerationVoteEvent) []event.ModerationVoteEvent {
	return trimEven(sortedVotes(id, votes))
}

// aggregate reduces the kept votes to the round verdict: FAIL on strict
// majority, with the details (reason, ids) taken from the lowest-ranked
// vote of the winning side so every moderator aggregates identically.
func aggregate(kept []event.ModerationVoteEvent) (verdict, []domain.ID) {
	failCount := 0
	for _, v := range kept {
		if !bool(v.Result) {
			failCount++
		}
	}
	majority := domain.OK
	if failCount*2 > len(kept) {
		majority = domain.FAIL
	}

	voters := make([]domain.ID, 0, len(kept))
	for _, v := range kept {
		voters = append(voters, domain.ID(v.ModeratorID))
	}

	for _, v := range kept {
		if v.Result == majority {
			return verdict{result: majority, reason: v.Reason, objectID: v.ObjectID, userID: v.UserID}, voters
		}
	}
	return verdict{result: majority}, voters
}

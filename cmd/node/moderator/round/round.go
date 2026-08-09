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

package round

import (
	"sync"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/moderator/vote"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	log "github.com/sirupsen/logrus"
)

const (
	// voteWindow is how long a round collects votes before the tally.
	voteWindow = 30 * time.Second
	// failoverDelay spaces the takeover chain of the round's voters: the
	// voter at rank k finalizes at k*failoverDelay unless a Final
	// announcement arrives first. One step must comfortably fit gossip
	// propagation of that announcement.
	failoverDelay = 10 * time.Second
	// voteDelayStep spaces the volunteer timers of moderators ranked
	// below the quorum target; one step must fit a fetch + inference +
	// gossip propagation so a suppressed rank never starts work.
	voteDelayStep = 8 * time.Second
	// quorumTarget is how many votes make a round served: once that many
	// are in, a moderator whose volunteer timer fires later stays silent
	// instead of spending an inference nobody needs.
	quorumTarget = 3
)

// Schedule carries a round's clock settings as state, so callers (and
// tests) can stretch them without turning the constants above into mutable
// globals.
type Schedule struct {
	Window   time.Duration
	Failover time.Duration
	Step     time.Duration
}

func DefaultSchedule() Schedule {
	return Schedule{Window: voteWindow, Failover: failoverDelay, Step: voteDelayStep}
}

// Participant is the local voter a round acts for. This package owns the
// voting protocol — who votes, when, who carries the decision — and knows
// nothing of what a ballot means or what happens once a round decides:
// everything domain-specific sits behind this interface, implemented by
// the caller.
type Participant interface {
	// Ballot returns this participant's vote on the subject. The bool is
	// false to abstain, which happens when the subject cannot be judged.
	Ballot(reportID string, subject event.ReportEvent) (vote.Event, bool, error)
	// Broadcast publishes a ballot (or a Final announcement) to the other
	// participants.
	Broadcast(v vote.Event) error
	// Decided is invoked exactly once per round, on the single
	// participant that carries the decision.
	Decided(subject event.ReportEvent, outcome vote.Event, voters []domain.ID)
}

// round is one subject's vote round, self-contained: it collects ballots,
// decides whether this participant should vote at all, tallies at the
// window close, and either carries the decision (as chair) or stands by to
// take over. It knows nothing about the participant's internals, the
// population or the other rounds — it reaches the outside world only
// through Participant and reports its own end through onDone.
type round struct {
	id       string
	self     string
	member   Participant
	schedule Schedule
	onDone   func(id string)

	mx         sync.Mutex
	report     *event.ReportEvent
	votes      map[string]vote.Event
	voteTimer  *time.Timer
	tallyTimer *time.Timer
	finalTimer *time.Timer
	// pending holds the tallied outcome while this node stands by as a
	// backup finalizer.
	pending *pendingOutcome
	closed  bool
}

// pendingOutcome is a tallied round outcome parked on a backup voter.
type pendingOutcome struct {
	subject event.ReportEvent
	outcome vote.Event
	voters  []domain.ID
}

// newRound starts a round: the tally timer runs from creation, because a
// round may be opened by an incoming vote before this node sees the report.
func newRound(id, self string, member Participant, schedule Schedule, onDone func(id string)) *round {
	r := &round{
		id:       id,
		self:     self,
		member:   member,
		schedule: schedule,
		onDone:   onDone,
		votes:    make(map[string]vote.Event),
	}
	r.tallyTimer = time.AfterFunc(schedule.Window, r.tally)
	return r
}

// setReport hands the round the report it is about and schedules this
// node's own vote after voteDelay (its deterministic turn in the volunteer
// order). Repeat deliveries of the same report are ignored.
func (r *round) setReport(rep event.ReportEvent, voteDelay time.Duration) {
	r.mx.Lock()
	defer r.mx.Unlock()
	if r.closed || r.report != nil {
		return
	}
	r.report = &rep
	r.voteTimer = time.AfterFunc(voteDelay, r.castVote)
}

func (r *round) addVote(v vote.Event) {
	r.mx.Lock()
	defer r.mx.Unlock()
	if r.closed {
		return
	}
	if _, dup := r.votes[v.ModeratorID]; dup {
		return
	}
	r.votes[v.ModeratorID] = v
}

// stop ends the round without finalizing: the owner is shutting down, or
// somebody else announced the round finished.
func (r *round) stop() {
	r.mx.Lock()
	defer r.mx.Unlock()
	r.closeLocked()
}

func (r *round) closeLocked() {
	r.closed = true
	for _, t := range []*time.Timer{r.voteTimer, r.tallyTimer, r.finalTimer} {
		if t != nil {
			t.Stop()
		}
	}
}

// castVote fires when this node's turn in the volunteer order comes up. If
// the round already holds a quorum of votes it costs nothing: no fetch, no
// inference, no message.
func (r *round) castVote() {
	r.mx.Lock()
	if r.closed || r.report == nil || len(r.votes) >= quorumTarget {
		r.mx.Unlock()
		return
	}
	rep := *r.report
	r.mx.Unlock()

	v, ok, err := r.member.Ballot(r.id, rep)
	if err != nil {
		log.Errorf("round %s: ballot: %v", r.id, err)
		return
	}
	if !ok {
		return
	}

	// Count the own ballot before broadcasting: loopback delivery is not
	// guaranteed (addVote dedups if it does loop back), and the ballot
	// must count even when the broadcast fails.
	r.addVote(v)
	if err := r.member.Broadcast(v); err != nil {
		log.Errorf("round %s: broadcast ballot: %v", r.id, err)
	}
}

// tally closes the voting window and decides this node's part: chair
// (finalize now), backup (stand by and take over if the chair goes silent)
// or bystander (nothing to do).
func (r *round) tally() {
	r.mx.Lock()
	if r.closed {
		r.mx.Unlock()
		return
	}
	if r.voteTimer != nil {
		r.voteTimer.Stop()
	}

	rep := r.report
	ordered := sortedVotes(r.id, r.votes)
	kept := trimEven(ordered)

	// The takeover chain ranks over the full voter order, pre-trim: a
	// voter dropped by the odd-count trim still holds everything needed
	// to finalize the kept tally if the chair dies.
	myRank := rankOf(ordered, r.self)

	// A voter always holds the report (voting requires it); a self-named
	// vote without one means someone forged votes for a round this node
	// never saw. Both drop out of the takeover chain like a bystander.
	if len(kept) == 0 || myRank < 0 || rep == nil {
		if myRank >= 0 && rep == nil {
			log.Warnf("round %s: voter without subject, skipping decision", r.id)
		}
		r.closeLocked()
		r.mx.Unlock()
		r.onDone(r.id)
		return
	}

	outcome, voters := aggregate(kept)
	chair := kept[0].ModeratorID

	if myRank > 0 {
		r.pending = &pendingOutcome{subject: *rep, outcome: outcome, voters: voters}
		r.finalTimer = time.AfterFunc(time.Duration(myRank)*r.schedule.Failover, r.takeOver)
		r.mx.Unlock()
		log.Infof("round %s closed votes=%d result=%t chair=%s (standing by at rank %d)",
			r.id, len(kept), bool(outcome.Result), chair, myRank)
		return
	}

	r.closeLocked()
	r.mx.Unlock()
	r.onDone(r.id)

	log.Infof("round %s closed votes=%d result=%t chair=%s",
		r.id, len(kept), bool(outcome.Result), chair)
	r.decide(*rep, outcome, voters)
}

// takeOver fires on a backup voter when its slot elapses with no Final
// announcement: the chair and every earlier backup stayed silent, so this
// node finalizes the round itself.
func (r *round) takeOver() {
	r.mx.Lock()
	if r.closed || r.pending == nil {
		r.mx.Unlock()
		return
	}
	p := r.pending
	r.closeLocked()
	r.mx.Unlock()
	r.onDone(r.id)

	log.Warnf("round %s: chair stayed silent, taking over the decision", r.id)
	r.decide(p.subject, p.outcome, p.voters)
}

// decide hands the outcome to the participant and then announces the round
// finished, so the other voters cancel their takeover timers.
func (r *round) decide(subject event.ReportEvent, outcome vote.Event, voters []domain.ID) {
	r.member.Decided(subject, outcome, voters)

	final := outcome
	final.ReportID = r.id
	final.Type = subject.Type
	final.ModeratorID = r.self
	final.Final = true
	if err := r.member.Broadcast(final); err != nil {
		log.Errorf("round %s: broadcast final: %v", r.id, err)
	}
}

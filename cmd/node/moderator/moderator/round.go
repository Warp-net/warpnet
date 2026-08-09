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
	"sync"
	"time"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	log "github.com/sirupsen/logrus"
)

var (
	// voteWindow is how long a round collects votes before the tally.
	voteWindow = 30 * time.Second
	// failoverDelay spaces the takeover chain of the round's voters: the
	// voter at rank k finalizes at k*failoverDelay unless a Final
	// announcement arrives first. One step must comfortably fit gossip
	// propagation of that announcement.
	failoverDelay = 10 * time.Second
)

// quorumTarget is how many votes make a round served: once that many are
// in, a moderator whose volunteer timer fires later stays silent instead of
// spending an inference nobody needs.
const quorumTarget = 3

// roundHost is everything a round needs from the moderator running it.
// The round owns the protocol (who votes, when, who finalizes); the host
// owns the side effects (running the engine, gossip, isolation).
type roundHost interface {
	// SelfID is the host moderator's peer id, the round's own identity
	// among the voters.
	SelfID() string
	// AssessReport runs the moderation engine over the reported object.
	// The bool is false when the report is unusable and no vote is due.
	AssessReport(rep event.ReportEvent) (verdict, bool, error)
	// PublishVote broadcasts a vote (or a Final announcement) to the
	// other moderators.
	PublishVote(vote event.ModerationVoteEvent) error
	// FinalizeRound delivers the agreed verdict: reporter notification
	// plus, on FAIL, the isolation broadcast.
	FinalizeRound(rep event.ReportEvent, agg verdict, voters []domain.ID)
}

// round is one report's vote round, self-contained: it collects votes,
// decides whether this node should vote at all, tallies at the window
// close, and either finalizes (as chair) or stands by to take over. It
// knows nothing about the moderator's internals, the network population or
// the other rounds — it reaches the outside world only through roundHost
// and reports its own end through onDone.
type round struct {
	id     string
	host   roundHost
	onDone func(id string)

	mx         sync.Mutex
	report     *event.ReportEvent
	votes      map[string]event.ModerationVoteEvent
	voteTimer  *time.Timer
	tallyTimer *time.Timer
	finalTimer *time.Timer
	// pending holds the tallied outcome while this node stands by as a
	// backup finalizer.
	pending *pendingFinalize
	closed  bool
}

// pendingFinalize is a tallied round outcome parked on a backup voter.
type pendingFinalize struct {
	report event.ReportEvent
	agg    verdict
	voters []domain.ID
}

// newRound starts a round: the tally timer runs from creation, because a
// round may be opened by an incoming vote before this node sees the report.
func newRound(id string, host roundHost, onDone func(id string)) *round {
	r := &round{
		id:     id,
		host:   host,
		onDone: onDone,
		votes:  make(map[string]event.ModerationVoteEvent),
	}
	r.tallyTimer = time.AfterFunc(voteWindow, r.tally)
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

func (r *round) addVote(vote event.ModerationVoteEvent) {
	r.mx.Lock()
	defer r.mx.Unlock()
	if r.closed {
		return
	}
	if _, dup := r.votes[vote.ModeratorID]; dup {
		return
	}
	r.votes[vote.ModeratorID] = vote
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

	v, ok, err := r.host.AssessReport(rep)
	if err != nil {
		log.Errorf("moderator: round %s: assess report: %v", r.id, err)
		return
	}
	if !ok {
		return
	}

	vote := event.ModerationVoteEvent{
		ReportID:    r.id,
		Type:        rep.Type,
		Result:      v.result,
		Reason:      v.reason,
		UserID:      v.userID,
		ObjectID:    v.objectID,
		ModeratorID: r.host.SelfID(),
	}
	// Count the own vote before publishing: gossip loopback delivery is
	// not guaranteed (addVote dedups if it does loop back), and the vote
	// must count even when the publish fails.
	r.addVote(vote)
	if err := r.host.PublishVote(vote); err != nil {
		log.Errorf("moderator: round %s: publish vote: %v", r.id, err)
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
	myRank := rankOf(ordered, r.host.SelfID())

	// A voter always holds the report (voting requires it); a self-named
	// vote without one means someone forged votes for a round this node
	// never saw. Both drop out of the takeover chain like a bystander.
	if len(kept) == 0 || myRank < 0 || rep == nil {
		if myRank >= 0 && rep == nil {
			log.Warnf("moderator: round %s: voter without report, skipping finalize", r.id)
		}
		r.closeLocked()
		r.mx.Unlock()
		r.onDone(r.id)
		return
	}

	agg, voters := aggregate(kept)
	chair := kept[0].ModeratorID

	if myRank > 0 {
		r.pending = &pendingFinalize{report: *rep, agg: agg, voters: voters}
		r.finalTimer = time.AfterFunc(time.Duration(myRank)*failoverDelay, r.takeOver)
		r.mx.Unlock()
		log.Infof("moderator: round %s closed votes=%d result=%t chair=%s (standing by at rank %d)",
			r.id, len(kept), bool(agg.result), chair, myRank)
		return
	}

	r.closeLocked()
	r.mx.Unlock()
	r.onDone(r.id)

	log.Infof("moderator: round %s closed votes=%d result=%t chair=%s",
		r.id, len(kept), bool(agg.result), chair)
	r.finalize(*rep, agg, voters)
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

	log.Warnf("moderator: round %s: chair stayed silent, taking over finalization", r.id)
	r.finalize(p.report, p.agg, p.voters)
}

// finalize delivers the verdict and then announces the round finished, so
// the other voters cancel their takeover timers.
func (r *round) finalize(rep event.ReportEvent, agg verdict, voters []domain.ID) {
	r.host.FinalizeRound(rep, agg, voters)

	final := event.ModerationVoteEvent{
		ReportID:    r.id,
		Type:        rep.Type,
		Result:      agg.result,
		Reason:      agg.reason,
		UserID:      agg.userID,
		ObjectID:    agg.objectID,
		ModeratorID: r.host.SelfID(),
		Final:       true,
	}
	if err := r.host.PublishVote(final); err != nil {
		log.Errorf("moderator: round %s: publish final: %v", r.id, err)
	}
}

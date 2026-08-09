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

	"github.com/Warp-net/warpnet/event"
	log "github.com/sirupsen/logrus"
)

const (
	// finalizedTTL guards against gossip re-deliveries reopening a
	// finished round.
	finalizedTTL = time.Hour
	// seenModTTL bounds the passive moderator-population estimate used
	// only to scale volunteer delays; no trust decision reads it.
	seenModTTL = 24 * time.Hour
)

// rounds is the moderator's collection of live vote rounds. It owns what
// spans rounds — which ones are still open, which are already spent, and
// how many moderators are out there — and hands each round the one number
// it cannot derive alone: when this node's turn to vote comes up.
//
// Locking: rounds.mx may be held while calling into a round, never the
// other way around. A round calls back (onDone) without holding its own
// lock, so the two never deadlock.
type roundRegistry struct {
	host     roundHost
	schedule roundSchedule

	mx        sync.Mutex
	active    map[string]*round
	finalized map[string]time.Time
	seenMods  map[string]time.Time
}

func newRoundRegistry(host roundHost, schedule roundSchedule) *roundRegistry {
	return &roundRegistry{
		host:      host,
		schedule:  schedule,
		active:    make(map[string]*round),
		finalized: make(map[string]time.Time),
		seenMods:  make(map[string]time.Time),
	}
}

// open starts (or joins) the round for a report and schedules this node's
// vote at its deterministic turn.
func (rs *roundRegistry) open(rep event.ReportEvent) {
	id := rep.ReportID()

	rs.mx.Lock()
	if rs.isFinalizedLocked(id) {
		rs.mx.Unlock()
		return
	}
	r := rs.ensureLocked(id)
	delay := rs.voteDelayLocked(id)
	rs.mx.Unlock()

	r.setReport(rep, delay)
}

// addVote routes an incoming vote to its round, opening one if the vote
// arrived before the report did.
func (rs *roundRegistry) addVote(vote event.ModerationVoteEvent) {
	rs.mx.Lock()
	rs.seenMods[vote.ModeratorID] = time.Now()
	if rs.isFinalizedLocked(vote.ReportID) {
		rs.mx.Unlock()
		return
	}
	r := rs.ensureLocked(vote.ReportID)
	rs.mx.Unlock()

	r.addVote(vote)
}

// markFinalized records that some moderator finalized the round and drops
// this node's copy, cancelling its takeover timer.
func (rs *roundRegistry) markFinalized(id, by string) {
	rs.mx.Lock()
	rs.seenMods[by] = time.Now()
	if rs.isFinalizedLocked(id) {
		rs.mx.Unlock()
		return
	}
	rs.finalized[id] = time.Now()
	r, ok := rs.active[id]
	delete(rs.active, id)
	rs.mx.Unlock()

	if ok {
		r.stop()
	}
	log.Infof("moderator: round %s finalized by %s", id, by)
}

// forget is the round's own completion callback: it is done with itself,
// so drop it and remember the id as spent.
func (rs *roundRegistry) forget(id string) {
	rs.mx.Lock()
	defer rs.mx.Unlock()
	delete(rs.active, id)
	for k, ts := range rs.finalized {
		if time.Since(ts) > finalizedTTL {
			delete(rs.finalized, k)
		}
	}
	rs.finalized[id] = time.Now()
}

func (rs *roundRegistry) stopAll() {
	rs.mx.Lock()
	stopping := make([]*round, 0, len(rs.active))
	for id, r := range rs.active {
		stopping = append(stopping, r)
		delete(rs.active, id)
	}
	rs.mx.Unlock()

	for _, r := range stopping {
		r.stop()
	}
}

func (rs *roundRegistry) ensureLocked(id string) *round {
	r, ok := rs.active[id]
	if !ok {
		r = newRound(id, rs.host, rs.schedule, rs.forget)
		rs.active[id] = r
	}
	return r
}

func (rs *roundRegistry) isFinalizedLocked(id string) bool {
	ts, ok := rs.finalized[id]
	if !ok {
		return false
	}
	if time.Since(ts) > finalizedTTL {
		delete(rs.finalized, id)
		return false
	}
	return true
}

// voteDelayLocked maps this moderator's deterministic rank for the round
// onto a start delay: the estimated top-quorumTarget ranks start at once,
// everyone below waits in voteDelayStep increments and will usually find
// the round already served when their turn comes.
func (rs *roundRegistry) voteDelayLocked(id string) time.Duration {
	now := time.Now()
	for k, ts := range rs.seenMods {
		if now.Sub(ts) > seenModTTL {
			delete(rs.seenMods, k)
		}
	}
	self := rs.host.selfID()
	population := len(rs.seenMods)
	if _, ok := rs.seenMods[self]; !ok {
		population++
	}
	u := float64(pairHash(id, self)>>11) / float64(1<<53) // uniform [0,1)
	rank := int(u * float64(population))
	if rank < quorumTarget {
		return 0
	}
	return time.Duration(rank-quorumTarget+1) * rs.schedule.step
}

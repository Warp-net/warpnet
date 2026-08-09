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
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/Warp-net/warpnet/cmd/node/moderator/vote"
	"github.com/Warp-net/warpnet/domain"
)

// Pure tally math: no state, no I/O. Every participant runs these over the
// same ballots and must reach the same answer, which is what lets a round
// pick a chair and a takeover order without exchanging a single message.

// pairHash orders (round, participant) pairs. It drives the volunteer
// delay, the chair choice and the odd-count trim.
func pairHash(reportID, moderatorID string) uint64 {
	h := sha256.Sum256([]byte(reportID + "|" + moderatorID))
	return binary.BigEndian.Uint64(h[:8])
}

// sortedVotes orders a round's votes by their deterministic pair hash.
func sortedVotes(id string, votes map[string]vote.Event) []vote.Event {
	ordered := make([]vote.Event, 0, len(votes))
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
func trimEven(ordered []vote.Event) []vote.Event {
	if len(ordered) > 0 && len(ordered)%2 == 0 {
		return ordered[:len(ordered)-1]
	}
	return ordered
}

// keptVotes is the tally set: hash-ordered votes trimmed to an odd count.
func keptVotes(id string, votes map[string]vote.Event) []vote.Event {
	return trimEven(sortedVotes(id, votes))
}

// rankOf reports a participant's position in the ordered ballots, or -1
// when it did not vote.
func rankOf(ordered []vote.Event, moderatorID string) int {
	for i, v := range ordered {
		if v.ModeratorID == moderatorID {
			return i
		}
	}
	return -1
}

// aggregate reduces the kept ballots to the round outcome: FAIL on strict
// majority, returning the lowest-ranked ballot of the winning side so every
// participant aggregates identically.
func aggregate(kept []vote.Event) (vote.Event, []domain.ID) {
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
			return v, voters
		}
	}
	return vote.Event{Result: majority}, voters
}

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

package rating

import (
	"math"
	"slices"
	"time"
)

// BucketDuration is the granularity entries are folded to. One
// hour keeps the key count low while still letting decay be computed
// at read time from the bucket alone.
const BucketDuration = time.Hour

const (
	// CapPerObserver is the most one remote observer may subtract from
	// a subject's score.
	CapPerObserver Score = 150
	// CapRemoteTotal is the most every remote observer together may
	// subtract. It is the load-bearing constant of the whole scheme:
	// with it, remote entries alone can never push a peer below
	// MaxScore-CapRemoteTotal = 600, the bottom of BandWatched.
	// Reaching BandDegraded or BandFloor therefore requires evidence
	// this node gathered on its own wire, so a slander campaign costs
	// an honest node a mild priority drop and nothing more.
	CapRemoteTotal Score = 400
	// MinAcquaintance is how long we must have been connected to a
	// remote observer before its records count. A drive-by accuser
	// has no voice.
	MinAcquaintance = time.Hour
)

// halfLife is how fast each dimension forgets. Transport misbehaviour
// is often a bad build or a bad link, so it recovers within a day; an
// upheld moderation verdict should outlive a news cycle.
//
//nolint:gochecknoglobals // a lookup table, not state
var halfLife = map[Dimension]time.Duration{
	Network:     12 * time.Hour,
	Application: 7 * 24 * time.Hour,
	Moderation:  7 * 24 * time.Hour,
}

// retentionHalfLives is how many half-lives a record is kept. After
// eight, its contribution is 1/256 of its weight — below the
// resolution of the score.
const retentionHalfLives = 8

func retention(d Dimension) time.Duration {
	return retentionHalfLives * halfLife[d]
}

// BucketOf is the unix hour t falls in.
func BucketOf(t time.Time) int64 {
	return t.UTC().Unix() / int64(BucketDuration/time.Second)
}

func bucketTime(bucket int64) time.Time {
	return time.Unix(bucket*int64(BucketDuration/time.Second), 0).UTC()
}

// decayFactor is 2^(-age/halfLife), clamped to [0,1]. A record from
// the future (clock skew within the validation slack) does not get
// amplified.
func decayFactor(age, half time.Duration) float64 {
	if age <= 0 {
		return 1
	}
	if half <= 0 {
		return 0
	}
	return math.Exp2(-age.Hours() / half.Hours())
}

// entry is the index's flattened view of one record.
type entry struct {
	observer   string
	dim        Dimension
	bucket     int64
	generation string
	counts     []CountEntry
}

// penaltyOf sums the decayed weight of every count in obs, clamping
// each kind's total to its ceiling. Callers pass the entries of a
// single (subject, observer, dimension) group: the ceiling is per
// group, so a peer with a flaky link to us cannot be talked down by
// its own dial failures, but two independent observers each reporting
// dial failures still add up.
func penaltyOf(obs []entry, dim Dimension, now time.Time) Score {
	if len(obs) == 0 {
		return 0
	}
	half := halfLife[dim]
	perKind := make(map[Kind]float64, len(obs))
	for _, o := range obs {
		factor := decayFactor(now.Sub(bucketTime(o.bucket)), half)
		if factor == 0 {
			continue
		}
		for _, c := range o.counts {
			perKind[c.Kind] += float64(c.Kind.Weight()) * float64(c.Count) * factor
		}
	}

	var total float64
	for kind, sum := range perKind {
		if ceiling := kind.Ceiling(); ceiling > 0 && sum > float64(ceiling) {
			sum = float64(ceiling)
		}
		total += sum
	}
	if total > float64(math.MaxInt32) {
		return MaxScore
	}
	return Score(total)
}

// groupByObserver splits a subject's entries on one dimension.
func groupByObserver(obs []entry, dim Dimension) map[string][]entry {
	out := make(map[string][]entry)
	for _, o := range obs {
		if o.dim != dim {
			continue
		}
		out[o.observer] = append(out[o.observer], o)
	}
	return out
}

// subjectiveScore is what this node enforces with: its own
// entries at full weight and without cap, every remote observer
// discounted by how much this node trusts it and capped twice over.
//
// weightOf is the caller's own-entries-only score for an
// observer, normalised to [0,1]. Using an own-only score keeps the
// recursion one level deep and terminating.
func subjectiveScore(
	obs []entry,
	dim Dimension,
	self string,
	now time.Time,
	weightOf func(observer string) float64,
	counts func(observer string) bool,
) Score {
	byObserver := groupByObserver(obs, dim)

	own := penaltyOf(byObserver[self], dim, now)

	var remote Score
	for observer, group := range byObserver {
		if observer == self {
			continue
		}
		if counts != nil && !counts(observer) {
			continue
		}
		weighted := Score(float64(penaltyOf(group, dim, now)) * weightOf(observer))
		if weighted > CapPerObserver {
			weighted = CapPerObserver
		}
		remote += weighted
	}
	if remote > CapRemoteTotal {
		remote = CapRemoteTotal
	}

	return (MaxScore - own - remote).clamp()
}

// ownOnlyScore uses nothing but this node's first-hand evidence. It
// backs weightOf above and is the reason the weighting terminates.
func ownOnlyScore(obs []entry, dim Dimension, self string, now time.Time) Score {
	byObserver := groupByObserver(obs, dim)
	return (MaxScore - penaltyOf(byObserver[self], dim, now)).clamp()
}

// publicScore is the unweighted median of what each observer thinks,
// for display only. It never reaches a rate limiter, a priority tag or
// a peer score: an unweighted number is exactly what a clique can
// move.
func publicScore(obs []entry, dim Dimension, now time.Time) (Score, int) {
	byObserver := groupByObserver(obs, dim)
	if len(byObserver) == 0 {
		return MaxScore, 0
	}
	scores := make([]Score, 0, len(byObserver))
	for _, group := range byObserver {
		scores = append(scores, (MaxScore - penaltyOf(group, dim, now)).clamp())
	}
	slices.Sort(scores)
	mid := len(scores) / 2
	if len(scores)%2 == 1 {
		return scores[mid], len(scores)
	}
	return (scores[mid-1] + scores[mid]) / 2, len(scores) //nolint:mnd
}

// tally is one offence kind's live count for a subject, for the UI.
type tally struct {
	kind   Kind
	count  uint32
	lastAt time.Time
}

// recentTallies aggregates raw counts per kind across every observer,
// so a user can see what their node is actually being marked for.
// Undecayed on purpose: "37 rate-limit hits in the last six hours" is
// what tells them what to fix, not 37 × 0.63.
func recentTallies(obs []entry, dim Dimension) []tally {
	agg := make(map[Kind]*tally)
	for _, o := range obs {
		if o.dim != dim {
			continue
		}
		at := bucketTime(o.bucket)
		for _, c := range o.counts {
			t, ok := agg[c.Kind]
			if !ok {
				t = &tally{kind: c.Kind}
				agg[c.Kind] = t
			}
			t.count += c.Count
			if at.After(t.lastAt) {
				t.lastAt = at
			}
		}
	}
	out := make([]tally, 0, len(agg))
	for _, t := range agg {
		out = append(out, *t)
	}
	slices.SortFunc(out, func(a, b tally) int {
		if a.count != b.count {
			return int(b.count) - int(a.count) // busiest first
		}
		return int(a.kind) - int(b.kind)
	})
	return out
}

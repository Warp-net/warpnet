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

const BucketDuration = time.Hour

const (
	CapPerObserver  Score = 150
	CapRemoteTotal  Score = 400
	MinAcquaintance       = time.Hour
)

var halfLife = map[Dimension]time.Duration{
	Network:     12 * time.Hour,
	Application: 7 * 24 * time.Hour,
	Moderation:  7 * 24 * time.Hour,
}

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

func ownOnlyScore(obs []entry, dim Dimension, self string, now time.Time) Score {
	byObserver := groupByObserver(obs, dim)
	return (MaxScore - penaltyOf(byObserver[self], dim, now)).clamp()
}

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

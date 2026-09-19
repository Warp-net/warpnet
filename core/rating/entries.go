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

	"github.com/Warp-net/warpnet/domain"
)

const bucketDuration = time.Hour

// bucket is the hour a record covers, in unix hours.
type bucket int64

func bucketAt(t time.Time) bucket {
	return bucket(t.UTC().Unix() / int64(bucketDuration/time.Second))
}

func (b bucket) start() time.Time {
	return time.Unix(int64(b)*int64(bucketDuration/time.Second), 0).UTC()
}

type kindCount struct {
	kind  Kind
	count uint32
}

// entry is one record as the indexer holds it.
type entry struct {
	observer   string
	dim        Dimension
	bucket     bucket
	generation string
	counts     []kindCount
}

// entries is one peer's records.
type entries []entry

func (es entries) byObserver(dim Dimension) map[string]entries {
	out := make(map[string]entries)
	for _, e := range es {
		if e.dim == dim {
			out[e.observer] = append(out[e.observer], e)
		}
	}
	return out
}

// isFresh reports whether this observer still has something to say. Its
// records all ageing out is not the same as it looking and finding nothing:
// an observer that left the network goes on holding its silence forever, and
// silence counts towards a clean score.
func (es entries) isFresh(dim Dimension, now time.Time) bool {
	for _, e := range es {
		if now.Sub(e.bucket.start()) <= dim.Retention() {
			return true
		}
	}
	return false
}

// penalty is the decayed weight of these entries on one dimension, with
// every kind capped at its ceiling.
func (es entries) penalty(dim Dimension, now time.Time) Score {
	if len(es) == 0 {
		return 0
	}
	perKind := make(map[Kind]float64, len(es))
	for _, e := range es {
		age := now.Sub(e.bucket.start())
		if age > dim.Retention() {
			continue // the horizon gc deletes own records at, applied to every record
		}
		factor := dim.decay(age)
		for _, c := range e.counts {
			perKind[c.kind] += float64(c.kind.Weight()) * float64(c.count) * factor
		}
	}

	var total float64
	for kind, sum := range perKind {
		if ceiling := kind.Ceiling(); ceiling > 0 {
			sum = min(sum, float64(ceiling))
		}
		total += sum
	}
	if total > float64(math.MaxInt32) {
		return MaxScore
	}
	return Score(total)
}

// dimensions lists the dimensions with evidence, in canonical order.
func (es entries) dimensions() []Dimension {
	seen := make(map[Dimension]struct{}, 3) //nolint:mnd
	for _, e := range es {
		seen[e.dim] = struct{}{}
	}
	out := make([]Dimension, 0, len(seen))
	for _, dim := range []Dimension{Network, Application, Moderation} {
		if _, ok := seen[dim]; ok {
			out = append(out, dim)
		}
	}
	return out
}

// median is the unweighted median score over observers and their count:
// display only, never enforced.
func (es entries) median(dim Dimension, now time.Time) (Score, int) {
	byObserver := es.byObserver(dim)
	scores := make([]Score, 0, len(byObserver))
	for _, group := range byObserver {
		if !group.isFresh(dim, now) {
			continue
		}
		scores = append(scores, (MaxScore - group.penalty(dim, now)).clamp())
	}
	if len(scores) == 0 {
		return MaxScore, 0
	}
	slices.Sort(scores)
	mid := len(scores) / 2
	if len(scores)%2 == 1 {
		return scores[mid], len(scores)
	}
	return (scores[mid-1] + scores[mid]) / 2, len(scores) //nolint:mnd
}

// tallies are raw, undecayed counts per kind, busiest first.
func (es entries) tallies(dim Dimension, now time.Time) []domain.OffenceTally {
	type tally struct {
		kind   Kind
		count  uint32
		lastAt time.Time
	}
	agg := make(map[Kind]*tally)
	for _, e := range es {
		if e.dim != dim {
			continue
		}
		at := e.bucket.start()
		if now.Sub(at) > dim.Retention() {
			continue // past the horizon the node deletes its own records at
		}
		for _, c := range e.counts {
			t, ok := agg[c.kind]
			if !ok {
				t = &tally{kind: c.kind}
				agg[c.kind] = t
			}
			t.count += c.count
			if at.After(t.lastAt) {
				t.lastAt = at
			}
		}
	}
	sorted := make([]*tally, 0, len(agg))
	for _, t := range agg {
		sorted = append(sorted, t)
	}
	slices.SortFunc(sorted, func(a, b *tally) int {
		if a.count != b.count {
			return int(b.count) - int(a.count)
		}
		return int(a.kind) - int(b.kind)
	})
	out := make([]domain.OffenceTally, 0, len(sorted))
	for _, t := range sorted {
		out = append(out, domain.OffenceTally{Kind: t.kind.String(), Count: t.count, LastAt: t.lastAt})
	}
	return out
}

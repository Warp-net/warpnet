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
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const propertyRuns = 200

var allDimensions = []Dimension{Network, Application, Moderation}

func kindsOf(dim Dimension) []Kind {
	var out []Kind
	for k, o := range catalogue {
		if o.dim == dim {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

// world is a random history about one peer: who said what, and when.
type world struct {
	self      identity
	peer      identity
	observers []identity
	records   []domain.RatingRecord
}

type worldSpec struct {
	observers int
	withSelf  bool
	maxAge    func(Dimension) time.Duration
}

func newWorld(t *testing.T, rng *rand.Rand, now time.Time, spec worldSpec) world {
	t.Helper()
	w := world{self: newIdentity(t), peer: newIdentity(t)}
	for range spec.observers {
		w.observers = append(w.observers, newIdentity(t))
	}
	authors := slices.Clone(w.observers)
	if spec.withSelf {
		authors = append(authors, w.self)
	}
	for _, author := range authors {
		for range 1 + rng.IntN(4) {
			dim := allDimensions[rng.IntN(len(allDimensions))]
			age := time.Duration(rng.Int64N(int64(spec.maxAge(dim)) + 1))
			b := bucketAt(now.Add(-age))
			kinds := kindsOf(dim)
			var counts []kindCount
			for _, k := range kinds {
				if rng.IntN(3) == 0 {
					counts = append(counts, kindCount{k, uint32(1 + rng.IntN(40))})
				}
			}
			if len(counts) == 0 {
				counts = append(counts, kindCount{kinds[rng.IntN(len(kinds))], uint32(1 + rng.IntN(40))})
			}
			gen := fmt.Sprintf("%032x", rng.Uint64())
			w.records = append(w.records, signedRecord(author, w.peer.id, dim, b, gen, counts...))
		}
	}
	return w
}

func withinRetention(dim Dimension) time.Duration { return dim.Retention() - bucketDuration }

func (w world) engine(t *testing.T, clock *fixedClock) *Engine {
	t.Helper()
	store := newFakeStore(w.self.id)
	e := newMemberEngine(t, w.self, store, clock)
	for _, rec := range w.records {
		store.merge(rec)
	}
	return e
}

// ownPenaltyOracle recomputes first-hand penalty from the documented rules,
// independently of entries.penalty.
func ownPenaltyOracle(records []domain.RatingRecord, self string, dim Dimension, now time.Time) float64 {
	perKind := map[string]float64{}
	for _, rec := range records {
		if rec.ObserverID != self || rec.Dimension != dim.String() {
			continue
		}
		age := now.Sub(time.Unix(rec.Bucket*3600, 0))
		if age > dim.Retention() {
			continue
		}
		factor := 1.0
		if age > 0 {
			factor = math.Pow(0.5, age.Hours()/dim.HalfLife().Hours())
		}
		for _, o := range rec.Offences {
			k, _ := ParseKind(o.Kind)
			perKind[o.Kind] += float64(k.Weight()) * float64(o.Count) * factor
		}
	}
	var total float64
	for name, sum := range perKind {
		k, _ := ParseKind(name)
		if c := k.Ceiling(); c > 0 {
			sum = min(sum, float64(c))
		}
		total += sum
	}
	return total
}

func TestPropertyEveryScoreStaysInRange(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for run := range propertyRuns {
		clock := newClock()
		w := newWorld(t, rng, clock.Now(), worldSpec{
			observers: rng.IntN(6), withSelf: rng.IntN(2) == 0,
			maxAge: func(d Dimension) time.Duration { return d.Retention() + 3*bucketDuration },
		})
		e := w.engine(t, clock)

		score := e.Score(w.peer.id)
		require.True(t, score >= MinScore && score <= MaxScore, "run %d: score %d", run, score)

		view, err := e.View(w.peer.id)
		require.NoError(t, err)
		require.True(t, view.Overall >= int32(MinScore) && view.Overall <= int32(MaxScore), "run %d", run)
		for _, d := range view.Dimensions {
			require.True(t, d.Score >= int32(MinScore) && d.Score <= int32(MaxScore), "run %d: %s", run, d.Name)
			require.Equal(t, Score(d.Score).Tier().String(), d.Tier, "run %d: tier follows the score", run)
			require.LessOrEqual(t, d.Score, int32(MaxScore))
			require.GreaterOrEqual(t, view.Overall, int32(0))
			require.LessOrEqual(t, view.Overall, d.Score, "run %d: overall is the worst axis", run)
		}
		require.LessOrEqual(t, view.Observers, len(w.observers)+1, "run %d", run)
	}
}

// However many acquainted observers pile on, remote evidence alone never
// takes a peer below watched, and one observer alone costs at most its cap.
func TestPropertyRemoteEvidenceIsBounded(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	lowest := MaxScore
	for run := range propertyRuns {
		clock := newClock()
		observers := 1 + rng.IntN(8)
		w := newWorld(t, rng, clock.Now(), worldSpec{observers: observers, maxAge: withinRetention})
		e := w.engine(t, clock)

		score := e.Score(w.peer.id)
		lowest = min(lowest, score)
		assert.GreaterOrEqual(t, score, MaxScore-capRemoteTotal, "run %d: %d observers", run, observers)
		assert.NotEqual(t, TierDegraded, score.Tier(), "run %d", run)
		assert.NotEqual(t, TierFloor, score.Tier(), "run %d", run)
		if observers == 1 {
			assert.Equal(t, TierTrusted, score.Tier(), "run %d: a lone observer never moves a peer out of trusted", run)
		}
	}
	assert.Equal(t, MaxScore-capRemoteTotal, lowest, "the total cap must actually bind in some run")
}

// With only first-hand evidence the score is exactly what the documented
// rules give: decayed weights, capped per kind, worst axis wins.
func TestPropertyFirstHandScoreMatchesTheRules(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	seen := map[Tier]bool{}
	for run := range propertyRuns {
		clock := newClock()
		w := newWorld(t, rng, clock.Now(), worldSpec{
			withSelf: true,
			maxAge:   func(d Dimension) time.Duration { return d.Retention() + 3*bucketDuration },
		})
		e := w.engine(t, clock)

		want := MaxScore
		for _, dim := range allDimensions {
			penalty := ownPenaltyOracle(w.records, w.self.id.String(), dim, clock.Now())
			want = min(want, (MaxScore - Score(min(penalty, math.MaxInt32))).clamp())
		}
		assert.InDelta(t, float64(want), float64(e.Score(w.peer.id)), 1, "run %d", run)
		seen[want.Tier()] = true
	}
	assert.Len(t, seen, 4, "the random histories must cover every tier")
}

func TestPropertyMoreFirstHandEvidenceNeverRaisesTheScore(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	for run := range propertyRuns / 4 {
		clock := newClock()
		w := newWorld(t, rng, clock.Now(), worldSpec{
			observers: rng.IntN(4), withSelf: rng.IntN(2) == 0, maxAge: withinRetention,
		})
		e := w.engine(t, clock)

		last := e.Score(w.peer.id)
		for step := range 40 {
			k := Kind(1 + rng.IntN(len(catalogue)))
			require.NoError(t, e.record(w.peer.id, k))
			flushNow(t, e)
			now := e.Score(w.peer.id)
			require.LessOrEqual(t, now, last, "run %d step %d: %s raised the score", run, step, k)
			last = now
		}
	}
}

// Evidence only decays. As long as this node holds nothing against the
// accusers themselves, time alone never lowers a score.
func TestPropertyTimeAloneNeverLowersAScore(t *testing.T) {
	rng := rand.New(rand.NewPCG(9, 10))
	for run := range propertyRuns / 2 {
		clock := newClock()
		w := newWorld(t, rng, clock.Now(), worldSpec{
			observers: rng.IntN(5), withSelf: true, maxAge: withinRetention,
		})
		e := w.engine(t, clock)

		last := e.Score(w.peer.id)
		for step := range 30 {
			clock.advance(time.Duration(1+rng.IntN(12)) * time.Hour)
			now := e.Score(w.peer.id)
			require.GreaterOrEqual(t, now, last, "run %d step %d", run, step)
			last = now
		}
		clock.advance(Application.Retention() + bucketDuration)
		assert.Equal(t, MaxScore, e.Score(w.peer.id), "run %d: everything retires in the end", run)
	}
}

// The order records arrive in, and whether they arrive live or are read
// back from disk after a restart, changes nothing.
func TestPropertyArrivalOrderAndRestartDoNotMatter(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 12))
	for run := range propertyRuns / 2 {
		clock := newClock()
		w := newWorld(t, rng, clock.Now(), worldSpec{
			observers: rng.IntN(6), withSelf: rng.IntN(2) == 0, maxAge: withinRetention,
		})

		live := w.engine(t, clock)
		require.NotNil(t, live)
		_ = live.Score(w.peer.id) // index the peer so later merges land live

		shuffled := slices.Clone(w.records)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		liveStore := newFakeStore(w.self.id)
		indexedFirst := newMemberEngine(t, w.self, liveStore, clock)
		_ = indexedFirst.Score(w.peer.id)
		for _, rec := range shuffled {
			liveStore.merge(rec)
		}

		diskStore := newFakeStore(w.self.id)
		for _, rec := range w.records {
			diskStore.seed(rec)
		}
		restarted := newMemberEngine(t, w.self, diskStore, clock)

		want := live.Score(w.peer.id)
		assert.Equal(t, want, indexedFirst.Score(w.peer.id), "run %d: merge order", run)
		assert.Equal(t, want, restarted.Score(w.peer.id), "run %d: read back from disk", run)

		wantView, err := live.View(w.peer.id)
		require.NoError(t, err)
		gotView, err := restarted.View(w.peer.id)
		require.NoError(t, err)
		assert.Equal(t, wantView.Overall, gotView.Overall, "run %d", run)
		assert.Equal(t, wantView.Observers, gotView.Observers, "run %d", run)
		assert.ElementsMatch(t, wantView.Dimensions, gotView.Dimensions, "run %d", run)
	}
}

// The public view is the plain median of what each fresh observer
// concluded, recomputed here without the engine.
func TestPropertyViewIsTheMedianOfObservers(t *testing.T) {
	rng := rand.New(rand.NewPCG(13, 14))
	for run := range propertyRuns {
		clock := newClock()
		w := newWorld(t, rng, clock.Now(), worldSpec{
			observers: 1 + rng.IntN(7), withSelf: rng.IntN(2) == 0,
			maxAge: func(d Dimension) time.Duration { return d.Retention() + 3*bucketDuration },
		})
		e := w.engine(t, clock)
		view, err := e.View(w.peer.id)
		require.NoError(t, err)

		for _, d := range view.Dimensions {
			dim, ok := ParseDimension(d.Name)
			require.True(t, ok)
			var scores []float64
			for _, author := range append(slices.Clone(w.observers), w.self) {
				fresh := false
				for _, rec := range w.records {
					if rec.ObserverID == author.id.String() && rec.Dimension == d.Name &&
						clock.Now().Sub(time.Unix(rec.Bucket*3600, 0)) <= dim.Retention() {
						fresh = true
					}
				}
				if !fresh {
					continue
				}
				p := ownPenaltyOracle(w.records, author.id.String(), dim, clock.Now())
				scores = append(scores, math.Max(0, float64(MaxScore)-math.Floor(p)))
			}
			want := float64(MaxScore)
			if n := len(scores); n > 0 {
				slices.Sort(scores)
				if n%2 == 1 {
					want = scores[n/2]
				} else {
					want = math.Floor((scores[n/2-1] + scores[n/2]) / 2)
				}
			}
			assert.InDelta(t, want, float64(d.Score), 1, "run %d: %s", run, d.Name)
		}
	}
}

func TestPropertySignaturesSurviveTheWireAndNothingElse(t *testing.T) {
	rng := rand.New(rand.NewPCG(15, 16))
	for run := range propertyRuns {
		clock := newClock()
		w := newWorld(t, rng, clock.Now(), worldSpec{observers: 1, maxAge: withinRetention})
		rec := w.records[rng.IntN(len(w.records))]
		rec.UpdatedAt = clock.Now().Add(time.Duration(rng.Int64N(int64(time.Hour)))) // sub-second precision

		signed := resign(t, rec, w.observers[0])
		require.NoError(t, record(signed).verify(), "run %d", run)

		wire, err := json.Marshal(signed)
		require.NoError(t, err)
		var back domain.RatingRecord
		require.NoError(t, json.Unmarshal(wire, &back))
		require.NoError(t, record(back).verify(), "run %d: a record must verify after a JSON round trip", run)

		shuffled := back
		shuffled.Offences = slices.Clone(back.Offences)
		rng.Shuffle(len(shuffled.Offences), func(i, j int) {
			shuffled.Offences[i], shuffled.Offences[j] = shuffled.Offences[j], shuffled.Offences[i]
		})
		require.NoError(t, record(shuffled).verify(), "run %d: offence order is not signed", run)

		for name, tamper := range tampers(rng) {
			bad := back
			bad.Offences = slices.Clone(back.Offences)
			tamper(&bad)
			assert.Error(t, record(bad).verify(), "run %d: tampering with %s must break the signature", run, name)
		}
	}
}

func tampers(rng *rand.Rand) map[string]func(*domain.RatingRecord) {
	return map[string]func(*domain.RatingRecord){
		"peer":       func(r *domain.RatingRecord) { r.PeerID = "12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo" },
		"dimension":  func(r *domain.RatingRecord) { r.Dimension += "x" },
		"bucket":     func(r *domain.RatingRecord) { r.Bucket += 1 + rng.Int64N(100) },
		"generation": func(r *domain.RatingRecord) { r.Generation = "ffffffffffffffffffffffffffffffff" },
		"count":      func(r *domain.RatingRecord) { r.Offences[0].Count++ },
		"kind":       func(r *domain.RatingRecord) { r.Offences[0].Kind += "x" },
		"extra": func(r *domain.RatingRecord) {
			r.Offences = append(r.Offences, domain.OffenceCount{Kind: "zzz", Count: 1})
		},
		"updated":   func(r *domain.RatingRecord) { r.UpdatedAt = r.UpdatedAt.Add(time.Millisecond) },
		"signature": func(r *domain.RatingRecord) { r.Signature = r.Signature[:len(r.Signature)-4] + "AAAA" },
	}
}

// resign signs rec again as its own observer, or as signer when given.
func resign(t *testing.T, rec domain.RatingRecord, signer ...identity) domain.RatingRecord {
	t.Helper()
	var who identity
	switch {
	case len(signer) > 0:
		who = signer[0]
		rec.ObserverID = who.id.String()
	default:
		t.Fatalf("resign needs the observer's key")
	}
	r, err := record(rec).signed(who.priv)
	require.NoError(t, err)
	return domain.RatingRecord(r)
}

// The canonical bytes are the wire contract between versions: changing
// them silently invalidates every record already replicated.
func TestSigningBytesAreStable(t *testing.T) {
	r := record{
		PeerID:     "12D3KooWPeer",
		ObserverID: "12D3KooWObserver",
		Dimension:  "net",
		Bucket:     490000,
		Generation: genA,
		Offences: []domain.OffenceCount{
			{Kind: "rate_limit_hit", Count: 7},
			{Kind: "bad_signature", Count: 2},
		},
		UpdatedAt: time.UnixMilli(1764000000123).UTC(),
	}
	assert.Equal(t,
		"12D3KooWPeer|12D3KooWObserver|net|490000|00112233445566778899aabbccddeeff|bad_signature=2,rate_limit_hit=7,|1764000000123",
		string(r.signingBytes()))
}

// Every wire name the catalogue uses is part of the contract as well.
func TestWireNamesAreStable(t *testing.T) {
	want := map[Kind]string{
		KindBadSignature: "bad_signature", KindMissingSignature: "missing_signature",
		KindMalformedFrame: "malformed_frame", KindOversizePayload: "oversize_payload",
		KindStaleOrReplayed: "stale_or_replayed", KindPrivateRouteDenied: "private_route_denied",
		KindRateLimitHit: "rate_limit_hit", KindDiscoveryFlood: "discovery_flood",
		KindConnectionFlap: "connection_flap", KindDialFailure: "dial_failure",
		KindForgedRecord: "forged_observation", KindModerationUpheld: "moderation_upheld",
		KindForeignAuthorship: "foreign_authorship", KindWriteFlood: "write_flood",
		KindFalseReportBurst: "false_report_burst", KindVerdictMalformed: "verdict_malformed",
		KindVerdictOutlier: "verdict_outlier", KindAuditWrong: "audit_wrong",
		KindAuditInvalid: "audit_invalid", KindAuditUnreachable: "audit_unreachable",
	}
	require.Len(t, catalogue, len(want))
	for k, name := range want {
		assert.Equal(t, name, k.String())
	}
	for _, d := range allDimensions {
		back, ok := ParseDimension(d.String())
		assert.True(t, ok)
		assert.Equal(t, d, back)
	}
	assert.Equal(t, []string{"net", "app", "mod"}, []string{Network.String(), Application.String(), Moderation.String()})
}

// Tiers only ever get worse as the score falls.
func TestPropertyTierIsMonotonicInTheScore(t *testing.T) {
	prev := MinScore.Tier()
	for s := MinScore; s <= MaxScore; s++ {
		tier := s.Tier()
		require.LessOrEqual(t, tier, prev, "score %d", s)
		prev = tier
	}
	assert.Equal(t, TierTrusted, MaxScore.Tier())
	assert.Equal(t, TierFloor, MinScore.Tier())
}

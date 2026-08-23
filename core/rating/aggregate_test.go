// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fullWeight(string) float64 { return 1 }

func entryOf(observer string, dim Dimension, bucket int64, generation string, counts ...CountEntry) entry {
	return entry{
		observer:   observer,
		dim:        dim,
		bucket:     bucket,
		generation: generation,
		counts:     counts,
	}
}

func TestDecayHalvesEveryHalfLife(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	half := halfLife[Network]

	// One bad signature (weight 250) observed exactly one half-life ago
	// must cost half its weight.
	obs := []entry{
		entryOf("obs", Network, BucketOf(now.Add(-half)), genA, CountEntry{KindBadSignature, 1}),
	}
	got := penaltyOf(obs, Network, now)
	assert.InDelta(t, 125, float64(got), 1, "one half-life must halve the penalty")

	fresh := []entry{
		entryOf("obs", Network, BucketOf(now), genA, CountEntry{KindBadSignature, 1}),
	}
	assert.InDelta(t, 250, float64(penaltyOf(fresh, Network, now)), 1)
}

func TestDecayIsMonotonic(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	var previous Score = MaxScore
	for age := time.Duration(0); age < retention(Network); age += 6 * time.Hour {
		obs := []entry{
			entryOf("obs", Network, BucketOf(now.Add(-age)), genA, CountEntry{KindBadSignature, 1}),
		}
		got := penaltyOf(obs, Network, now)
		assert.LessOrEqual(t, got, previous, "penalty must never grow with age")
		previous = got
	}
}

func TestGenerationsUnderOneBucketAreSummed(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	bucket := BucketOf(now)

	// The same observer, same bucket, two process lifetimes: this is
	// exactly what a restarted stateless node produces. Summing is
	// what keeps the replayed history from being lost.
	obs := []entry{
		entryOf("obs", Network, bucket, genA, CountEntry{KindMalformedFrame, 1}),
		entryOf("obs", Network, bucket, genB, CountEntry{KindMalformedFrame, 1}),
	}
	assert.InDelta(t, 240, float64(penaltyOf(obs, Network, now)), 1,
		"two generations in one bucket must add up, not overwrite")
}

func TestKindCeilingCaps(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	bucket := BucketOf(now)

	// 100 dial failures at weight 2 would be 200, but the kind is
	// capped at 100: a flaky link must not talk a peer down.
	obs := []entry{
		entryOf("obs", Network, bucket, genA, CountEntry{KindDialFailure, 100}),
	}
	assert.EqualValues(t, KindDialFailure.Ceiling(), penaltyOf(obs, Network, now))

	// An uncapped kind keeps accumulating.
	uncapped := []entry{
		entryOf("obs", Network, bucket, genA, CountEntry{KindBadSignature, 4}),
	}
	assert.InDelta(t, 1000, float64(penaltyOf(uncapped, Network, now)), 1)
}

// TestRemoteObservationsCannotReachDegraded is the load-bearing
// invariant of the whole design: no amount of remote accusation, from
// any number of observers, may push a peer below the bottom of
// BandWatched. Slander costs an honest node a priority drop and
// nothing more.
func TestRemoteObservationsCannotReachDegraded(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	bucket := BucketOf(now)
	const self = "self"

	for _, observers := range []int{1, 3, 50, 500} {
		t.Run(fmt.Sprintf("%d_observers", observers), func(t *testing.T) {
			obs := make([]entry, 0, observers)
			for i := range observers {
				obs = append(obs, entryOf(
					fmt.Sprintf("accuser-%d", i), Network, bucket, genA,
					// Everything they can throw, at full trust.
					CountEntry{KindBadSignature, 50},
					CountEntry{KindPrivateRouteDenied, 50},
					CountEntry{KindForgedRecord, 50},
				))
			}
			score := subjectiveScore(obs, Network, self, now, fullWeight, nil)

			assert.GreaterOrEqual(t, score, MaxScore-CapRemoteTotal,
				"remote entries alone must never drop below %d", MaxScore-CapRemoteTotal)
			// Bands ascend in severity, so this asserts the outcome is
			// no worse than BandWatched.
			assert.LessOrEqual(t, BandOf(score), BandWatched,
				"remote-only accusations must never reach BandDegraded")
		})
	}
}

func TestSingleRemoteObserverIsCappedTighter(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	obs := []entry{
		entryOf("accuser", Network, BucketOf(now), genA, CountEntry{KindBadSignature, 100}),
	}
	score := subjectiveScore(obs, Network, "self", now, fullWeight, nil)
	assert.Equal(t, MaxScore-CapPerObserver, score)
}

func TestFirstHandEvidenceReachesFloor(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	const self = "self"
	obs := []entry{
		entryOf(self, Network, BucketOf(now), genA, CountEntry{KindBadSignature, 4}),
	}
	score := subjectiveScore(obs, Network, self, now, fullWeight, nil)
	assert.Equal(t, MinScore, score)
	assert.Equal(t, BandFloor, BandOf(score))
}

func TestDistrustedAccuserIsDiscounted(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	obs := []entry{
		entryOf("accuser", Network, BucketOf(now), genA, CountEntry{KindBadSignature, 1}),
	}

	trusted := subjectiveScore(obs, Network, "self", now, fullWeight, nil)
	distrusted := subjectiveScore(obs, Network, "self", now,
		func(string) float64 { return 0.1 }, nil)

	assert.Less(t, trusted, distrusted, "an accuser we distrust must move the score less")
}

func TestUnacquaintedObserverHasNoVoice(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	obs := []entry{
		entryOf("stranger", Network, BucketOf(now), genA, CountEntry{KindBadSignature, 1}),
	}
	score := subjectiveScore(obs, Network, "self", now, fullWeight,
		func(string) bool { return false })
	assert.Equal(t, MaxScore, score)
}

func TestPublicScoreIsUnweightedMedian(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	bucket := BucketOf(now)
	obs := []entry{
		entryOf("a", Network, bucket, genA, CountEntry{KindBadSignature, 1}), // 750
		entryOf("b", Network, bucket, genA, CountEntry{KindBadSignature, 1}), // 750
		entryOf("c", Network, bucket, genA, CountEntry{KindRateLimitHit, 1}), // 985
	}
	score, observers := publicScore(obs, Network, now)
	assert.Equal(t, 3, observers)
	assert.InDelta(t, 750, float64(score), 2, "median of {750,750,985}")
}

func TestPublicScoreOfUnobservedSubjectIsMax(t *testing.T) {
	score, observers := publicScore(nil, Network, time.Now())
	assert.Equal(t, MaxScore, score)
	assert.Equal(t, 0, observers)
}

func TestRecentTalliesAreUndecayedAndSortedByCount(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	obs := []entry{
		entryOf("a", Network, BucketOf(now.Add(-48*time.Hour)), genA, CountEntry{KindRateLimitHit, 30}),
		entryOf("b", Network, BucketOf(now), genA, CountEntry{KindRateLimitHit, 7}, CountEntry{KindMalformedFrame, 4}),
	}
	tallies := recentTallies(obs, Network)
	require.Len(t, tallies, 2)
	assert.Equal(t, KindRateLimitHit, tallies[0].kind)
	assert.EqualValues(t, 37, tallies[0].count, "counts are raw, not decayed")
	assert.Equal(t, KindMalformedFrame, tallies[1].kind)
	assert.EqualValues(t, 4, tallies[1].count)
	assert.Equal(t, bucketTime(BucketOf(now)), tallies[0].lastAt)
}

func TestOtherDimensionsAreIgnored(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	obs := []entry{
		entryOf("self", Moderation, BucketOf(now), genA, CountEntry{KindAuditInvalid, 4}),
	}
	assert.Equal(t, MaxScore, subjectiveScore(obs, Network, "self", now, fullWeight, nil),
		"a moderation offence must not move the network score")
}

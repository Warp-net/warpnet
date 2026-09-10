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

func testEntry(observer string, dim Dimension, bucket int64, generation string, counts ...kindCount) entry {
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

	obs := []entry{
		testEntry("obs", Network, BucketOf(now.Add(-half)), genA, kindCount{KindBadSignature, 1}),
	}
	got := penaltyOf(obs, Network, now)
	assert.InDelta(t, 125, float64(got), 1, "one half-life must halve the penalty")

	fresh := []entry{
		testEntry("obs", Network, BucketOf(now), genA, kindCount{KindBadSignature, 1}),
	}
	assert.InDelta(t, 250, float64(penaltyOf(fresh, Network, now)), 1)
}

func TestDecayIsMonotonic(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	previous := MaxScore
	for age := time.Duration(0); age < retention(Network); age += 6 * time.Hour {
		obs := []entry{
			testEntry("obs", Network, BucketOf(now.Add(-age)), genA, kindCount{KindBadSignature, 1}),
		}
		got := penaltyOf(obs, Network, now)
		assert.LessOrEqual(t, got, previous, "penalty must never grow with age")
		previous = got
	}
}

func TestGenerationsUnderOneBucketAreSummed(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	bucket := BucketOf(now)

	obs := []entry{
		testEntry("obs", Network, bucket, genA, kindCount{KindMalformedFrame, 1}),
		testEntry("obs", Network, bucket, genB, kindCount{KindMalformedFrame, 1}),
	}
	assert.InDelta(t, 240, float64(penaltyOf(obs, Network, now)), 1,
		"two generations in one bucket must add up, not overwrite")
}

func TestKindCeilingCaps(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	bucket := BucketOf(now)

	obs := []entry{
		testEntry("obs", Network, bucket, genA, kindCount{KindDialFailure, 100}),
	}
	assert.EqualValues(t, KindDialFailure.Ceiling(), penaltyOf(obs, Network, now))

	// An uncapped kind keeps accumulating.
	uncapped := []entry{
		testEntry("obs", Network, bucket, genA, kindCount{KindBadSignature, 4}),
	}
	assert.InDelta(t, 1000, float64(penaltyOf(uncapped, Network, now)), 1)
}

func TestRemoteObservationsCannotReachDegraded(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	bucket := BucketOf(now)
	const self = "self"

	for _, observers := range []int{1, 3, 50, 500} {
		t.Run(fmt.Sprintf("%d_observers", observers), func(t *testing.T) {
			obs := make([]entry, 0, observers)
			for i := range observers {
				obs = append(obs, testEntry(
					fmt.Sprintf("accuser-%d", i), Network, bucket, genA,
					// Everything they can throw, at full trust.
					kindCount{KindBadSignature, 50},
					kindCount{KindPrivateRouteDenied, 50},
					kindCount{KindForgedRecord, 50},
				))
			}
			score := localScore(obs, Network, self, now, fullWeight, nil)

			assert.GreaterOrEqual(t, score, MaxScore-CapRemoteTotal,
				"remote entries alone must never drop below %d", MaxScore-CapRemoteTotal)
			assert.LessOrEqual(t, TierOf(score), TierWatched,
				"remote-only accusations must never reach TierDegraded")
		})
	}
}

func TestSingleRemoteObserverIsCappedTighter(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	obs := []entry{
		testEntry("accuser", Network, BucketOf(now), genA, kindCount{KindBadSignature, 100}),
	}
	score := localScore(obs, Network, "self", now, fullWeight, nil)
	assert.Equal(t, MaxScore-CapPerObserver, score)
}

func TestFirstHandEvidenceReachesFloor(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	const self = "self"
	obs := []entry{
		testEntry(self, Network, BucketOf(now), genA, kindCount{KindBadSignature, 4}),
	}
	score := localScore(obs, Network, self, now, fullWeight, nil)
	assert.Equal(t, MinScore, score)
	assert.Equal(t, TierFloor, TierOf(score))
}

func TestDistrustedAccuserIsDiscounted(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	obs := []entry{
		testEntry("accuser", Network, BucketOf(now), genA, kindCount{KindBadSignature, 1}),
	}

	trusted := localScore(obs, Network, "self", now, fullWeight, nil)
	distrusted := localScore(obs, Network, "self", now,
		func(string) float64 { return 0.1 }, nil)

	assert.Less(t, trusted, distrusted, "an accuser we distrust must move the score less")
}

func TestUnacquaintedObserverHasNoVoice(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	obs := []entry{
		testEntry("stranger", Network, BucketOf(now), genA, kindCount{KindBadSignature, 1}),
	}
	score := localScore(obs, Network, "self", now, fullWeight,
		func(string) bool { return false })
	assert.Equal(t, MaxScore, score)
}

func TestPublicScoreIsUnweightedMedian(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	bucket := BucketOf(now)
	obs := []entry{
		testEntry("a", Network, bucket, genA, kindCount{KindBadSignature, 1}), // 750
		testEntry("b", Network, bucket, genA, kindCount{KindBadSignature, 1}), // 750
		testEntry("c", Network, bucket, genA, kindCount{KindRateLimitHit, 1}), // 985
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
		testEntry("a", Network, BucketOf(now.Add(-48*time.Hour)), genA, kindCount{KindRateLimitHit, 30}),
		testEntry("b", Network, BucketOf(now), genA, kindCount{KindRateLimitHit, 7}, kindCount{KindMalformedFrame, 4}),
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
		testEntry("self", Moderation, BucketOf(now), genA, kindCount{KindAuditInvalid, 4}),
	}
	assert.Equal(t, MaxScore, localScore(obs, Network, "self", now, fullWeight, nil),
		"a moderation offence must not move the network score")
}

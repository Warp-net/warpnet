// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"testing"
	"time"

	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecayHalvesEveryHalfLife(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	half := Network.HalfLife()

	aged := entries{testEntry("obs", Network, bucketAt(now.Add(-half)), genA, kindCount{KindBadSignature, 1})}
	assert.InDelta(t, 125, float64(aged.penalty(Network, now)), 1, "one half-life must halve the penalty")

	fresh := entries{testEntry("obs", Network, bucketAt(now), genA, kindCount{KindBadSignature, 1})}
	assert.InDelta(t, 250, float64(fresh.penalty(Network, now)), 1)
}

func TestDecayIsMonotonic(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	previous := MaxScore
	for age := time.Duration(0); age < Network.Retention(); age += 6 * time.Hour {
		es := entries{testEntry("obs", Network, bucketAt(now.Add(-age)), genA, kindCount{KindBadSignature, 1})}
		got := es.penalty(Network, now)
		assert.LessOrEqual(t, got, previous, "penalty must never grow with age")
		previous = got
	}
}

func TestRetentionIsEightHalfLives(t *testing.T) {
	for _, dim := range []Dimension{Network, Application, Moderation} {
		assert.Equal(t, 8*dim.HalfLife(), dim.Retention())
		assert.Positive(t, dim.HalfLife())
	}
	assert.Zero(t, Dimension(9).HalfLife(), "an unknown dimension decays nothing")
}

func TestGenerationsUnderOneBucketAreSummed(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	b := bucketAt(now)

	es := entries{
		testEntry("obs", Network, b, genA, kindCount{KindMalformedFrame, 1}),
		testEntry("obs", Network, b, genB, kindCount{KindMalformedFrame, 1}),
	}
	assert.InDelta(t, 240, float64(es.penalty(Network, now)), 1,
		"two generations in one bucket must add up, not overwrite")
}

func TestKindCeilingCaps(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	b := bucketAt(now)

	capped := entries{testEntry("obs", Network, b, genA, kindCount{KindDialFailure, 100})}
	assert.EqualValues(t, KindDialFailure.Ceiling(), capped.penalty(Network, now))

	uncapped := entries{testEntry("obs", Network, b, genA, kindCount{KindBadSignature, 4})}
	assert.InDelta(t, 1000, float64(uncapped.penalty(Network, now)), 1, "an uncapped kind keeps accumulating")
}

func TestMedianIsUnweighted(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	b := bucketAt(now)
	es := entries{
		testEntry("a", Network, b, genA, kindCount{KindBadSignature, 1}), // 750
		testEntry("b", Network, b, genA, kindCount{KindBadSignature, 1}), // 750
		testEntry("c", Network, b, genA, kindCount{KindRateLimitHit, 1}), // 985
	}
	score, observers := es.median(Network, now)
	assert.Equal(t, 3, observers)
	assert.InDelta(t, 750, float64(score), 2, "median of {750,750,985}")

	score, observers = entries(nil).median(Network, now)
	assert.Equal(t, MaxScore, score, "an unobserved peer is at full trust")
	assert.Zero(t, observers)
}

func TestTalliesAreUndecayedAndSortedByCount(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	es := entries{
		testEntry("a", Network, bucketAt(now.Add(-48*time.Hour)), genA, kindCount{KindRateLimitHit, 30}),
		testEntry("b", Network, bucketAt(now), genA, kindCount{KindRateLimitHit, 7}, kindCount{KindMalformedFrame, 4}),
		testEntry("b", Application, bucketAt(now), genA, kindCount{KindWriteFlood, 9}),
	}
	got := es.tallies(Network, now)
	require.Len(t, got, 2, "another dimension's offences stay out")
	assert.Equal(t, domain.OffenceTally{Kind: KindRateLimitHit.String(), Count: 37, LastAt: bucketAt(now).start()}, got[0],
		"counts are raw, not decayed, and the last sighting is the newest bucket")
	assert.Equal(t, KindMalformedFrame.String(), got[1].Kind)
	assert.EqualValues(t, 4, got[1].Count)
}

// An observation past the retention horizon is gone from the history as well
// as from the score: it is the same horizon gc deletes this node's own records
// at, so a foreign record nobody is left to delete cannot outlive it either.
// An observer whose records have all aged out has stopped observing, and a
// node that left the network keeps that silence forever. Counting it as a
// clean vote lets the departed outvote everyone still watching.
func TestMedianIgnoresObserversThatHaveNothingFreshToSay(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	stale := bucketAt(now.Add(-Network.Retention() - time.Hour))

	es := entries{
		testEntry("live", Network, bucketAt(now), genA, kindCount{KindRateLimitHit, 20}),
		testEntry("gone1", Network, stale, genA, kindCount{KindRateLimitHit, 1}),
		testEntry("gone2", Network, stale, genA, kindCount{KindRateLimitHit, 1}),
		testEntry("gone3", Network, stale, genA, kindCount{KindRateLimitHit, 1}),
	}

	score, observers := es.median(Network, now)
	assert.Equal(t, 1, observers, "only the observer still watching votes")
	assert.Less(t, score, MaxScore, "and what it saw decides the score")

	onlyGone := es[1:]
	score, observers = onlyGone.median(Network, now)
	assert.Equal(t, MaxScore, score, "with nobody left watching the peer owes nothing")
	assert.Zero(t, observers)
}

func TestTalliesDropObservationsPastTheRetentionHorizon(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	es := entries{
		testEntry("a", Network, bucketAt(now.Add(-Network.Retention()-time.Hour)), genA, kindCount{KindRateLimitHit, 500}),
		testEntry("b", Network, bucketAt(now), genA, kindCount{KindRateLimitHit, 3}),
	}

	got := es.tallies(Network, now)
	require.Len(t, got, 1)
	assert.EqualValues(t, 3, got[0].Count, "only what still counts is shown")

	assert.Zero(t, entries{es[0]}.penalty(Network, now),
		"and it weighs nothing in the score either")
}

func TestDimensionsAreListedInCanonicalOrder(t *testing.T) {
	b := bucketAt(time.Now())
	es := entries{
		testEntry("a", Moderation, b, genA, kindCount{KindAuditWrong, 1}),
		testEntry("a", Network, b, genA, kindCount{KindRateLimitHit, 1}),
	}
	assert.Equal(t, []Dimension{Network, Moderation}, es.dimensions())
	assert.Empty(t, entries(nil).dimensions())
}

func TestBucketRoundTrip(t *testing.T) {
	now := time.Date(2026, 9, 10, 15, 42, 7, 0, time.UTC)
	b := bucketAt(now)
	assert.Equal(t, time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC), b.start(), "a bucket starts on the hour")
	assert.Equal(t, b, bucketAt(b.start()))
	assert.Equal(t, b+1, bucketAt(now.Add(time.Hour)))
}

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSigningBytesAreOrderIndependent(t *testing.T) {
	obs := newIdentity(t)
	sub := newIdentity(t)
	now := time.Now()

	ascending := signedRecord(obs, sub.id, Network, BucketOf(now), genA,
		CountEntry{KindMalformedFrame, 2}, CountEntry{KindRateLimitHit, 5})
	descending := signedRecord(obs, sub.id, Network, BucketOf(now), genA,
		CountEntry{KindRateLimitHit, 5}, CountEntry{KindMalformedFrame, 2})

	assert.Equal(t, string(ascending.SigningBytes()), string(descending.SigningBytes()),
		"count order must not change the signed bytes")
	assert.Equal(t, ascending.Signature, descending.Signature)
}

func TestVerifyRejectsForeignSignature(t *testing.T) {
	obs := newIdentity(t)
	impostor := newIdentity(t)
	sub := newIdentity(t)
	bucket := BucketOf(time.Now())

	rec := signedRecord(obs, sub.id, Network, bucket, genA, CountEntry{KindBadSignature, 1})
	require.NoError(t, rec.Verify())

	// Same content, but claiming to come from someone else.
	rec.Observer = impostor.id.String()
	assert.Error(t, rec.Verify(), "a record must not verify under a foreign observer id")
}

func TestVerifyRejectsTamperedCounts(t *testing.T) {
	obs := newIdentity(t)
	sub := newIdentity(t)
	rec := signedRecord(obs, sub.id, Network, BucketOf(time.Now()), genA,
		CountEntry{KindRateLimitHit, 1})

	rec.Counts[0].Count = 9999
	assert.Error(t, rec.Verify(), "inflating a count must break the signature")
}

func TestValidate(t *testing.T) {
	obs := newIdentity(t)
	sub := newIdentity(t)
	now := time.Now()
	bucket := BucketOf(now)

	valid := signedRecord(obs, sub.id, Network, bucket, genA, CountEntry{KindRateLimitHit, 1})
	require.NoError(t, valid.Validate(now))

	t.Run("self rating is refused", func(t *testing.T) {
		rec := valid
		rec.Subject = rec.Observer
		assert.ErrorIs(t, rec.Validate(now), ErrRecordSelfRated)
	})

	t.Run("kind from another dimension is refused", func(t *testing.T) {
		rec := valid
		rec.Counts = []CountEntry{{KindModerationUpheld, 1}} // application kind on a network record
		assert.ErrorIs(t, rec.Validate(now), ErrRecordBadKind)
	})

	t.Run("unknown kind is refused", func(t *testing.T) {
		rec := valid
		rec.Counts = []CountEntry{{Kind(60000), 1}}
		assert.ErrorIs(t, rec.Validate(now), ErrRecordBadKind)
	})

	t.Run("malformed generation is refused", func(t *testing.T) {
		rec := valid
		rec.Generation = "not-hex"
		assert.ErrorIs(t, rec.Validate(now), ErrRecordBadGeneration)
	})

	t.Run("empty counts are refused", func(t *testing.T) {
		rec := valid
		rec.Counts = nil
		assert.ErrorIs(t, rec.Validate(now), ErrRecordEmptyCounts)
	})

	t.Run("future bucket is refused beyond one bucket of skew", func(t *testing.T) {
		rec := valid
		rec.Bucket = bucket + 2
		assert.ErrorIs(t, rec.Validate(now), ErrRecordBucketFuture)

		rec.Bucket = bucket + 1 // one bucket of clock skew is tolerated
		assert.NoError(t, rec.Validate(now))
	})

	t.Run("bucket past retention is refused", func(t *testing.T) {
		rec := signedRecord(obs, sub.id, Network,
			BucketOf(now.Add(-retention(Network)-time.Hour)), genA,
			CountEntry{KindRateLimitHit, 1})
		assert.ErrorIs(t, rec.Validate(now), ErrRecordBucketStale)
	})

	t.Run("subject that is not a peer id is refused", func(t *testing.T) {
		rec := valid
		rec.Subject = "definitely-not-a-peer-id"
		assert.ErrorIs(t, rec.Validate(now), ErrRecordBadSubject)
	})
}

func TestKeyRoundTrip(t *testing.T) {
	obs := newIdentity(t)
	sub := newIdentity(t)
	bucket := BucketOf(time.Now())
	rec := signedRecord(obs, sub.id, Moderation, bucket, genB, CountEntry{KindAuditWrong, 3})

	subject, observer, dim, gotBucket, generation, ok := parseKey(rec.Key())
	require.True(t, ok, "key %q must parse", rec.Key())
	assert.Equal(t, rec.Subject, subject)
	assert.Equal(t, rec.Observer, observer)
	assert.Equal(t, Moderation, dim)
	assert.Equal(t, bucket, gotBucket)
	assert.Equal(t, genB, generation)
}

func TestParseKeyRejectsForeignKeys(t *testing.T) {
	for _, key := range []string{
		"/STATS/incr/whatever/node/gen",
		"/RATING/obs/too/few/parts",
		"/RATING/obs/a/b/badDim/1/" + genA,
		"/RATING/obs/a/b/net/notanumber/" + genA,
	} {
		_, _, _, _, _, ok := parseKey(key)
		assert.False(t, ok, "key %q must not parse", key)
	}
}

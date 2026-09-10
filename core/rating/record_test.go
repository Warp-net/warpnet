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

func TestSigningBytesAreOrderIndependent(t *testing.T) {
	obs := newIdentity(t)
	sub := newIdentity(t)
	now := time.Now()

	ascending := signedRecord(obs, sub.id, Network, BucketOf(now), genA,
		kindCount{KindMalformedFrame, 2}, kindCount{KindRateLimitHit, 5})
	descending := signedRecord(obs, sub.id, Network, BucketOf(now), genA,
		kindCount{KindRateLimitHit, 5}, kindCount{KindMalformedFrame, 2})

	assert.Equal(t, string(signingBytes(ascending)), string(signingBytes(descending)),
		"offence order must not change the signed bytes")
	assert.Equal(t, ascending.Signature, descending.Signature)
}

func TestVerifyRejectsForeignSignature(t *testing.T) {
	obs := newIdentity(t)
	impostor := newIdentity(t)
	sub := newIdentity(t)

	rec := signedRecord(obs, sub.id, Network, BucketOf(time.Now()), genA, kindCount{KindBadSignature, 1})
	require.NoError(t, verifyRecord(rec))

	rec.ObserverId = impostor.id.String() // same content, claiming another author
	assert.Error(t, verifyRecord(rec), "a record must not verify under a foreign observer id")
}

func TestVerifyRejectsTamperedCounts(t *testing.T) {
	obs := newIdentity(t)
	sub := newIdentity(t)
	rec := signedRecord(obs, sub.id, Network, BucketOf(time.Now()), genA, kindCount{KindRateLimitHit, 1})

	rec.Offences[0].Count = 9999
	assert.Error(t, verifyRecord(rec), "inflating a count must break the signature")

	unsigned := rec
	unsigned.Signature = ""
	assert.ErrorIs(t, verifyRecord(unsigned), ErrRecordNoSignature)
}

func TestValidate(t *testing.T) {
	obs := newIdentity(t)
	sub := newIdentity(t)
	now := time.Now()
	bucket := BucketOf(now)

	valid := signedRecord(obs, sub.id, Network, bucket, genA, kindCount{KindRateLimitHit, 1})
	require.NoError(t, validateRecord(valid, now))

	t.Run("self rating is refused", func(t *testing.T) {
		rec := valid
		rec.PeerId = rec.ObserverId
		assert.ErrorIs(t, validateRecord(rec, now), ErrRecordSelfRated)
	})

	t.Run("kind from another dimension is refused", func(t *testing.T) {
		rec := valid
		rec.Offences = []domain.OffenceCount{{Kind: KindModerationUpheld.String(), Count: 1}}
		assert.ErrorIs(t, validateRecord(rec, now), ErrRecordBadKind)
	})

	t.Run("unknown kind is refused", func(t *testing.T) {
		rec := valid
		rec.Offences = []domain.OffenceCount{{Kind: "made_up", Count: 1}}
		assert.ErrorIs(t, validateRecord(rec, now), ErrRecordBadKind)
	})

	t.Run("unknown dimension is refused", func(t *testing.T) {
		rec := valid
		rec.Dimension = "vibes"
		assert.ErrorIs(t, validateRecord(rec, now), ErrRecordBadDimension)
	})

	t.Run("malformed generation is refused", func(t *testing.T) {
		rec := valid
		rec.Generation = "not-hex"
		assert.ErrorIs(t, validateRecord(rec, now), ErrRecordBadGeneration)
	})

	t.Run("empty offences are refused", func(t *testing.T) {
		rec := valid
		rec.Offences = nil
		assert.ErrorIs(t, validateRecord(rec, now), ErrRecordEmptyOffences)
	})

	t.Run("future bucket is refused beyond one bucket of skew", func(t *testing.T) {
		rec := valid
		rec.Bucket = bucket + 2
		assert.ErrorIs(t, validateRecord(rec, now), ErrRecordBucketFuture)

		rec.Bucket = bucket + 1
		assert.NoError(t, validateRecord(rec, now))
	})

	t.Run("bucket past retention is refused", func(t *testing.T) {
		rec := signedRecord(obs, sub.id, Network,
			BucketOf(now.Add(-retention(Network)-time.Hour)), genA, kindCount{KindRateLimitHit, 1})
		assert.ErrorIs(t, validateRecord(rec, now), ErrRecordBucketStale)
	})

	t.Run("peer that is not a peer id is refused", func(t *testing.T) {
		rec := valid
		rec.PeerId = "definitely-not-a-peer-id"
		assert.ErrorIs(t, validateRecord(rec, now), ErrRecordBadPeerId)
	})
}

func TestOnlyStructuralFailuresAreForgery(t *testing.T) {
	for _, err := range []error{
		ErrRecordSelfRated, ErrRecordBadPeerId, ErrRecordBadDimension,
		ErrRecordBadGeneration, ErrRecordEmptyOffences, ErrRecordBadKind,
	} {
		assert.True(t, forged(err), "%v proves its author wrote nonsense", err)
	}
	for _, err := range []error{ErrRecordBucketStale, ErrRecordBucketFuture, ErrRecordNoSignature, ErrRecordNoPubKey} {
		assert.False(t, forged(err), "%v is not attributable to the named observer", err)
	}
}

func TestEntryOfConvertsNamesBackToKinds(t *testing.T) {
	obs := newIdentity(t)
	sub := newIdentity(t)
	bucket := BucketOf(time.Now())
	rec := signedRecord(obs, sub.id, Moderation, bucket, genB, kindCount{KindAuditWrong, 3})

	assert.Equal(t, entry{
		observer:   obs.id.String(),
		dim:        Moderation,
		bucket:     bucket,
		generation: genB,
		counts:     []kindCount{{KindAuditWrong, 3}},
	}, entryOf(rec))
}

func TestGenerationIsUniquePerProcess(t *testing.T) {
	seen := make(map[string]struct{}, 64)
	for range 64 {
		gen, err := newGeneration()
		require.NoError(t, err)
		assert.Len(t, gen, generationHexLen)

		_, dup := seen[gen]
		assert.False(t, dup, "generation nonces must never repeat")
		seen[gen] = struct{}{}
	}
}

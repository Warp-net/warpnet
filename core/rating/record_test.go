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
	b := bucketAt(time.Now())

	ascending := record(signedRecord(obs, sub.id, Network, b, genA,
		kindCount{KindMalformedFrame, 2}, kindCount{KindRateLimitHit, 5}))
	descending := record(signedRecord(obs, sub.id, Network, b, genA,
		kindCount{KindRateLimitHit, 5}, kindCount{KindMalformedFrame, 2}))

	assert.Equal(t, string(ascending.signingBytes()), string(descending.signingBytes()),
		"offence order must not change the signed bytes")
	assert.Equal(t, ascending.Signature, descending.Signature)
}

func TestVerifyRejectsForeignSignature(t *testing.T) {
	obs := newIdentity(t)
	impostor := newIdentity(t)
	sub := newIdentity(t)

	r := record(signedRecord(obs, sub.id, Network, bucketAt(time.Now()), genA, kindCount{KindBadSignature, 1}))
	require.NoError(t, r.verify())

	r.ObserverID = impostor.id.String() // same content, claiming another author
	assert.Error(t, r.verify(), "a record must not verify under a foreign observer id")
}

func TestVerifyRejectsTamperedCounts(t *testing.T) {
	obs := newIdentity(t)
	sub := newIdentity(t)
	r := record(signedRecord(obs, sub.id, Network, bucketAt(time.Now()), genA, kindCount{KindRateLimitHit, 1}))

	r.Offences[0].Count = 9999
	assert.Error(t, r.verify(), "inflating a count must break the signature")

	unsigned := r
	unsigned.Signature = ""
	assert.ErrorIs(t, unsigned.verify(), ErrRecordNoSignature)
}

func TestValidate(t *testing.T) {
	obs := newIdentity(t)
	sub := newIdentity(t)
	now := time.Now()
	b := bucketAt(now)

	valid := record(signedRecord(obs, sub.id, Network, b, genA, kindCount{KindRateLimitHit, 1}))
	require.NoError(t, valid.validate(now))

	t.Run("self rating is refused", func(t *testing.T) {
		r := valid
		r.PeerID = r.ObserverID
		assert.ErrorIs(t, r.validate(now), ErrRecordSelfRated)
	})

	t.Run("kind from another dimension is refused", func(t *testing.T) {
		r := valid
		r.Offences = []domain.OffenceCount{{Kind: KindModerationUpheld.String(), Count: 1}}
		assert.ErrorIs(t, r.validate(now), ErrRecordBadKind)
	})

	t.Run("unknown kind is refused", func(t *testing.T) {
		r := valid
		r.Offences = []domain.OffenceCount{{Kind: "made_up", Count: 1}}
		assert.ErrorIs(t, r.validate(now), ErrRecordBadKind)
	})

	t.Run("unknown dimension is refused", func(t *testing.T) {
		r := valid
		r.Dimension = "vibes"
		assert.ErrorIs(t, r.validate(now), ErrRecordBadDimension)
	})

	t.Run("malformed generation is refused", func(t *testing.T) {
		r := valid
		r.Generation = "not-hex"
		assert.ErrorIs(t, r.validate(now), ErrRecordBadGeneration)
	})

	t.Run("empty offences are refused", func(t *testing.T) {
		r := valid
		r.Offences = nil
		assert.ErrorIs(t, r.validate(now), ErrRecordEmptyOffences)
	})

	t.Run("future bucket is refused beyond one bucket of skew", func(t *testing.T) {
		r := valid
		r.Bucket = int64(b) + 2
		assert.ErrorIs(t, r.validate(now), ErrRecordBucketFuture)

		r.Bucket = int64(b) + 1
		assert.NoError(t, r.validate(now))
	})

	t.Run("bucket past retention is refused", func(t *testing.T) {
		r := record(signedRecord(obs, sub.id, Network,
			bucketAt(now.Add(-Network.Retention()-time.Hour)), genA, kindCount{KindRateLimitHit, 1}))
		assert.ErrorIs(t, r.validate(now), ErrRecordBucketStale)
	})

	t.Run("peer that is not a peer id is refused", func(t *testing.T) {
		r := valid
		r.PeerID = "definitely-not-a-peer-id"
		assert.ErrorIs(t, r.validate(now), ErrRecordBadPeerID)
	})
}

func TestOnlyStructuralFailuresAreForgery(t *testing.T) {
	for _, err := range []error{
		ErrRecordSelfRated, ErrRecordBadPeerID, ErrRecordBadDimension,
		ErrRecordBadGeneration, ErrRecordEmptyOffences, ErrRecordBadKind,
	} {
		assert.True(t, isForgery(err), "%v proves its author wrote nonsense", err)
	}
	for _, err := range []error{ErrRecordBucketStale, ErrRecordBucketFuture, ErrRecordNoSignature, ErrRecordNoPubKey} {
		assert.False(t, isForgery(err), "%v is not attributable to the named observer", err)
	}
}

func TestEntryConvertsNamesBackToKinds(t *testing.T) {
	obs := newIdentity(t)
	sub := newIdentity(t)
	b := bucketAt(time.Now())
	r := record(signedRecord(obs, sub.id, Moderation, b, genB, kindCount{KindAuditWrong, 3}))

	assert.Equal(t, entry{
		observer:   obs.id.String(),
		dim:        Moderation,
		bucket:     b,
		generation: genB,
		counts:     []kindCount{{KindAuditWrong, 3}},
	}, r.entry())
}

func TestOffencesAreCanonical(t *testing.T) {
	c := counts{KindRateLimitHit: 5, KindBadSignature: 1, KindMalformedFrame: 2}
	assert.Equal(t, []domain.OffenceCount{
		{Kind: "bad_signature", Count: 1},
		{Kind: "malformed_frame", Count: 2},
		{Kind: "rate_limit_hit", Count: 5},
	}, c.offences(), "ascending by kind name, so two nodes sign the same bytes for the same counts")
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

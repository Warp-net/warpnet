// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/stretchr/testify/assert"
)

func TestDimensionsPerRole(t *testing.T) {
	assert.Equal(t, []Dimension{Network}, DimensionsFor(warpnet.RelayNode),
		"a relay can only witness wire behaviour")
	assert.Equal(t, []Dimension{Network, Application}, DimensionsFor(warpnet.MemberNode))
	assert.Equal(t, []Dimension{Network, Moderation}, DimensionsFor(warpnet.ModeratorNode))
	assert.Equal(t, []Dimension{Network}, DimensionsFor("something-new"),
		"an unknown role still speaks the wire and nothing else can be assumed")
}

func TestDimensionRoundTrip(t *testing.T) {
	for _, d := range []Dimension{Network, Application, Moderation} {
		got, ok := ParseDimension(d.String())
		assert.True(t, ok)
		assert.Equal(t, d, got)
		assert.True(t, d.Valid())
	}
	_, ok := ParseDimension("nope")
	assert.False(t, ok)
	assert.False(t, Dimension(9).Valid())
}

// Every kind must be reachable, named uniquely, and belong to the
// dimension its detection site can actually witness.
func TestCatalogueIsWellFormed(t *testing.T) {
	names := make(map[string]Kind, len(catalogue))
	for kind, o := range catalogue {
		assert.True(t, kind.Valid(), "%d", kind)
		assert.NotEmpty(t, o.name, "%d has no name", kind)
		assert.True(t, o.dim.Valid(), "%s has an invalid dimension", o.name)
		assert.Positive(t, o.weight, "%s must cost something", o.name)
		assert.LessOrEqual(t, o.weight, int32(MaxScore), "%s cannot cost more than the whole score", o.name)

		if prev, dup := names[o.name]; dup {
			t.Fatalf("kinds %d and %d share the name %q", prev, kind, o.name)
		}
		names[o.name] = kind

		back, ok := KindByName(o.name)
		assert.True(t, ok)
		assert.Equal(t, kind, back)
	}
}

// A ceiling below the kind's own weight would make the ceiling, not
// the weight, the effective cost of a single occurrence.
func TestCeilingsExceedTheirWeight(t *testing.T) {
	for kind, o := range catalogue {
		if o.ceiling == 0 {
			continue
		}
		assert.GreaterOrEqual(t, o.ceiling, o.weight,
			"%s: a ceiling under the weight makes one occurrence cost less than stated", kind)
	}
}

// The kinds that can drive a peer to BandFloor on their own must be
// the deliberate ones. Anything that can fire because of a bad link or
// a busy moment carries a ceiling.
func TestOnlyDeliberateKindsAreUncapped(t *testing.T) {
	deliberate := map[Kind]bool{
		KindBadSignature:         true,
		KindMissingSignature:     true,
		KindPrivateRouteDenied:   true,
		KindOversizePayload:      true,
		KindMalformedFrame:       true,
		KindStaleOrReplayed:      true,
		KindForgedObservation:    true,
		KindModerationUpheld:     true,
		KindForeignAuthorship:    true,
		KindVerdictBadSignature:  true,
		KindVerdictNoModeratorID: true,
		KindVerdictMalformed:     true,
		KindVerdictUnsolicited:   true,
		KindAuditInvalid:         true,
	}
	for kind, o := range catalogue {
		if o.ceiling == 0 {
			assert.True(t, deliberate[kind],
				"%s is uncapped but is not a deliberate offence", kind)
		}
	}
}

func TestUnknownKindIsInert(t *testing.T) {
	unknown := Kind(60000)
	assert.False(t, unknown.Valid())
	assert.Zero(t, unknown.Weight())
	assert.Equal(t, "unknown", unknown.String())
}

func TestScoreClamps(t *testing.T) {
	assert.Equal(t, MaxScore, Score(5000).clamp())
	assert.Equal(t, MinScore, Score(-5000).clamp())
	assert.Equal(t, Score(742), Score(742).clamp())
}

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"testing"

	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingRater captures what discovery charges, so a test can assert
// on offences instead of log lines.
type recordingRater struct {
	kinds []rating.Kind
}

func (r *recordingRater) Record(_ warpnet.WarpPeerID, k rating.Kind) {
	r.kinds = append(r.kinds, k)
}

func (r *recordingRater) Score(warpnet.WarpPeerID) rating.Score        { return rating.MaxScore }
func (r *recordingRater) Band(warpnet.WarpPeerID) rating.Band          { return rating.BandTrusted }
func (r *recordingRater) EffectiveBand(warpnet.WarpPeerID) rating.Band { return rating.BandTrusted }
func (r *recordingRater) Mode() rating.Mode                            { return rating.ModeShadow }

func mustAddr(t *testing.T, s string) warpnet.WarpAddress {
	t.Helper()
	a, err := warpnet.NewMultiaddr(s)
	require.NoError(t, err)
	return a
}

func testPeer(t *testing.T, seed string) warpnet.WarpPeerID {
	t.Helper()
	id := warpnet.FromStringToPeerID(seed)
	require.NotEmpty(t, id, "seed %q must be a valid peer id", seed)
	return id
}

const (
	peerA = "12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"
	peerB = "12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo"
)

// One chatty peer used to be able to spend the whole global budget,
// starving discovery of everyone else. The per-peer bucket is what
// stops that.
func TestOnePeerCannotStarveTheDiscoveryBudget(t *testing.T) {
	a := testPeer(t, peerA)
	b := testPeer(t, peerB)

	limiter := newPeerLimiter(nil)
	t.Cleanup(limiter.Close)

	var admitted int
	for range 100 {
		if limiter.Allow(a) {
			admitted++
		}
	}
	assert.LessOrEqual(t, admitted, perPeerCapacity,
		"a single peer must not exceed its own share")

	assert.True(t, limiter.Allow(b),
		"a quiet peer must still get through after a noisy one has flooded")
}

func TestWorseRatedPeerGetsASmallerShare(t *testing.T) {
	a := testPeer(t, peerA)

	limiter := newPeerLimiter(func(warpnet.WarpPeerID) rating.Band {
		return rating.BandFloor
	})
	t.Cleanup(limiter.Close)

	var admitted int
	for range 100 {
		if limiter.Allow(a) {
			admitted++
		}
	}
	assert.Less(t, admitted, perPeerCapacity,
		"a floor-band peer must be squeezed harder than a trusted one")
	assert.Positive(t, admitted, "but never shut out entirely")
}

// Rediscovery of a known peer used to cost a full info round trip
// every time, which is what turned republished gossip into O(N²)
// requests network-wide.
func TestKnownPeerIsNotReprobed(t *testing.T) {
	a := testPeer(t, peerA)
	b := testPeer(t, peerB)

	s := &discoveryService{probed: newProbedCache()}

	assert.True(t, s.shouldProbe(a), "the first sighting must probe")
	for range 10 {
		assert.False(t, s.shouldProbe(a), "further sightings must not")
	}
	assert.True(t, s.shouldProbe(b), "a different peer is still probed")
}

func TestShouldProbeWithoutCacheAlwaysProbes(t *testing.T) {
	// A service constructed without the cache (relay paths built in
	// tests) must not silently stop discovering.
	s := &discoveryService{}
	assert.True(t, s.shouldProbe(testPeer(t, peerA)))
}

// A dial that never had an address to try says nothing about the peer:
// it means gossip named someone our routing table cannot resolve yet.
// Charging it made honest nodes rate each other down during ordinary
// discovery in a live three-node run.
func TestUnresolvableDialChargesNobody(t *testing.T) {
	rater := &recordingRater{}
	s := &discoveryService{}
	s.SetRating(rater)

	s.recordDialFailure(warpnet.WarpAddrInfo{ID: testPeer(t, peerA)})

	assert.Empty(t, rater.kinds, "a dial with no address to try must charge nobody")
}

func TestFailedDialToAKnownAddressIsCharged(t *testing.T) {
	rater := &recordingRater{}
	s := &discoveryService{}
	s.SetRating(rater)
	id := testPeer(t, peerA)

	s.recordDialFailure(warpnet.WarpAddrInfo{
		ID:    id,
		Addrs: []warpnet.WarpAddress{mustAddr(t, "/ip4/127.0.0.1/tcp/1")},
	})

	assert.Equal(t, []rating.Kind{rating.KindDialFailure}, rater.kinds)
}

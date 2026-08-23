// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"errors"
	"sync"
	"testing"

	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRater records what the middleware charges and serves a fixed
// band back.
type fakeRater struct {
	mu       sync.Mutex
	observed []rating.Kind
	subjects []warpnet.WarpPeerID
	band     rating.Band
	// bandErr makes the store unreadable, which every enforcement point
	// has to survive by leaving the peer alone.
	bandErr error
}

func (f *fakeRater) Record(subject warpnet.WarpPeerID, k rating.Kind) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.observed = append(f.observed, k)
	f.subjects = append(f.subjects, subject)
	return nil
}

func (f *fakeRater) Score(warpnet.WarpPeerID) (Score, error) { return rating.MaxScore, nil }

func (f *fakeRater) Band(warpnet.WarpPeerID) (rating.Band, error) {
	if f.bandErr != nil {
		return rating.BandTrusted, f.bandErr
	}
	return f.band, nil
}

func (f *fakeRater) kinds() []rating.Kind {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]rating.Kind(nil), f.observed...)
}

// Score is aliased so the fake satisfies rating.Scorer without
// importing the type name twice in the signature above.
type Score = rating.Score

func callAuth(
	t *testing.T, mw *WarpMiddleware, local, remote warpnet.WarpPeerID, route string, body []byte,
) {
	t.Helper()
	handler := mw.AuthMiddleware(func(_ []byte, _ warpnet.WarpStream) (any, error) {
		return []byte(`["ok"]`), nil
	})
	client, server := stream.NewLoopbackStream(local, remote, warpnet.WarpProtocolID(route))
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	_, _ = handler(body, remoteStream{
		WarpStream: server,
		conn:       remoteConn{local: local, remote: remote},
	})
}

func TestAuthChargesTheRightOffences(t *testing.T) {
	local := warpnet.WarpPeerID("12D3KooWLocalLocalLocalLocalLocalLocalLoca")
	remote := warpnet.WarpPeerID("12D3KooWRemoteRemoteRemoteRemoteRemoteRemo")

	unsigned, err := json.Marshal(event.Message{MessageId: "m1", Destination: event.PUBLIC_GET_INFO})
	require.NoError(t, err)

	for _, tc := range []struct {
		name string
		body []byte
		want rating.Kind
	}{
		{"garbage is a malformed frame", []byte("not json at all"), rating.KindMalformedFrame},
		{"a message with no signature is charged as such", unsigned, rating.KindMissingSignature},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rater := &fakeRater{}
			mw := &WarpMiddleware{ownNodeId: local, rater: rater}
			callAuth(t, mw, local, remote, event.PUBLIC_GET_INFO, tc.body)
			assert.Equal(t, []rating.Kind{tc.want}, rater.kinds())
		})
	}
}

func TestAuthDoesNotChargeSelfStreams(t *testing.T) {
	self := warpnet.WarpPeerID("12D3KooWSelfSelfSelfSelfSelfSelfSelfSelfSe")
	rater := &fakeRater{}
	mw := &WarpMiddleware{ownNodeId: self, rater: rater}

	// A loopback self-stream carrying garbage: the node must not rate
	// itself, however broken its own message is.
	callAuth(t, mw, self, self, event.PUBLIC_GET_INFO, []byte("not json at all"))
	assert.Empty(t, rater.kinds())
}

func TestRateLimitHitIsCharged(t *testing.T) {
	local := warpnet.WarpPeerID("12D3KooWLocalLocalLocalLocalLocalLocalLoca")
	remote := warpnet.WarpPeerID("12D3KooWRemoteRemoteRemoteRemoteRemoteRemo")

	rater := &fakeRater{}
	mw := newLimiterMiddlewareForTest(t, local)
	mw.rater = rater

	// limitPairing is the tightest bucket: burst 5.
	route := event.PRIVATE_POST_PAIR
	for range 5 {
		require.True(t, callLimited(t, mw, local, remote, route))
	}
	require.False(t, callLimited(t, mw, local, remote, route))

	assert.Equal(t, []rating.Kind{rating.KindRateLimitHit}, rater.kinds())
	assert.Equal(t, remote, rater.subjects[0])
}

func TestDegradedPeerGetsATighterBucket(t *testing.T) {
	local := warpnet.WarpPeerID("12D3KooWLocalLocalLocalLocalLocalLocalLoca")
	remote := warpnet.WarpPeerID("12D3KooWRemoteRemoteRemoteRemoteRemoteRemo")

	rater := &fakeRater{band: rating.BandDegraded}
	mw := newLimiterMiddlewareForTest(t, local)
	mw.rater = rater

	// limitPairing burst 5, degraded multiplier 0.25 -> 1.
	route := event.PRIVATE_POST_PAIR
	require.True(t, callLimited(t, mw, local, remote, route))
	assert.False(t, callLimited(t, mw, local, remote, route),
		"a degraded peer must exhaust its burst far sooner")
}

// An enforcement point that cannot read the evidence must not act as
// if it had: a peer whose standing failed to load keeps the allowance
// of a peer with no record at all.
func TestUnreadableStandingDoesNotTightenAnything(t *testing.T) {
	local := warpnet.WarpPeerID("12D3KooWLocalLocalLocalLocalLocalLocalLoca")
	remote := warpnet.WarpPeerID("12D3KooWRemoteRemoteRemoteRemoteRemoteRemo")

	rater := &fakeRater{band: rating.BandFloor, bandErr: errors.New("datastore is down")}
	mw := newLimiterMiddlewareForTest(t, local)
	mw.rater = rater

	route := event.PRIVATE_POST_PAIR
	for i := range 5 {
		assert.True(t, callLimited(t, mw, local, remote, route),
			"a failed standing read must not apply a band (request %d)", i+1)
	}
}

func TestBucketIsRebuiltWhenTheBandChanges(t *testing.T) {
	local := warpnet.WarpPeerID("12D3KooWLocalLocalLocalLocalLocalLocalLoca")
	remote := warpnet.WarpPeerID("12D3KooWRemoteRemoteRemoteRemoteRemoteRemo")

	rater := &fakeRater{band: rating.BandTrusted}
	mw := newLimiterMiddlewareForTest(t, local)
	mw.rater = rater

	route := stream.WarpRoute(event.PRIVATE_POST_PAIR)
	trusted := mw.bucket(route, remote)
	require.Equal(t, rating.BandTrusted, trusted.band)

	rater.band = rating.BandFloor
	degraded := mw.bucket(route, remote)
	assert.Equal(t, rating.BandFloor, degraded.band)
	assert.Less(t, degraded.capacity, trusted.capacity,
		"a peer must not keep the allowance it earned in a better band")
}

func TestScaleForBandNeverStarvesAPeer(t *testing.T) {
	// The floor multiplier applied to the tightest route still has to
	// leave something: rating slows peers down, it never cuts them off.
	scaled := scaleForBand(limitPairing, rating.BandFloor)
	assert.GreaterOrEqual(t, scaled.burst, int64(1))
	assert.GreaterOrEqual(t, scaled.perMinute, int64(1))
}

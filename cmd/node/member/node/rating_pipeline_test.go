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

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package node

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"

	ratingdb "github.com/Warp-net/warpnet/core/crdt/rating"
	"github.com/Warp-net/warpnet/core/handler"
	"github.com/Warp-net/warpnet/core/middleware"
	corenode "github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/ratelimit"
	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// localBroadcaster keeps the rating CRDT on this replica alone.
type localBroadcaster struct{}

func (localBroadcaster) Broadcast(context.Context, []byte) error { return nil }
func (localBroadcaster) Next(ctx context.Context) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type noProviders struct{}

func (noProviders) FindProvidersAsync(context.Context, warpnet.WarpCID, int) <-chan warpnet.WarpAddrInfo {
	ch := make(chan warpnet.WarpAddrInfo)
	close(ch)
	return ch
}

// ratingStack is the rating as NewMemberNode and Start wire it: the
// Badger-backed CRDT store, the engine, the read-model, and the middleware
// that feeds it and reads it back.
type ratingStack struct {
	self    warpnet.WarpPeerID
	priv    ed25519.PrivateKey
	db      *local_store.DB
	store   *ratingdb.Store
	engine  *rating.Engine
	ratings *rating.PeersRatings
	mw      *middleware.WarpMiddleware
	chain   warpnet.WarpHandlerFunc
	served  int
}

func newRatingStack(t *testing.T, db *local_store.DB, priv ed25519.PrivateKey, limits ratelimit.Settings) *ratingStack {
	t.Helper()
	self, err := warpnet.IDFromPublicKey(priv.Public().(ed25519.PublicKey))
	require.NoError(t, err)

	host, err := libp2p.New(corenode.WarpIdentity(priv), libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = host.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	store, err := ratingdb.New(ctx, localBroadcaster{}, database.NewRatingRepo(db), host, noProviders{})
	require.NoError(t, err)

	s := &ratingStack{self: self, priv: priv, db: db, store: store, ratings: rating.NewPeersRatings()}
	hour := time.Now().UTC().Truncate(time.Hour) // a charge costs its full weight at the top of the hour
	s.engine, err = rating.NewEngine(ctx, store, host.Network(), priv, warpnet.MemberNode,
		rating.WithRatings(s.ratings), rating.WithFlushInterval(20*time.Millisecond),
		rating.WithClock(func() time.Time { return hour }))
	require.NoError(t, err)

	s.mw = middleware.NewWarpMiddleware(self, nil, ratelimit.NewStreamLimiter(limits, s.ratings))
	s.engine.Listen(s.mw.Event())
	s.chain = s.mw.RateLimiterMiddleware(s.mw.AuthMiddleware(func([]byte, warpnet.WarpStream) (any, error) {
		s.served++
		return event.Accepted, nil
	}))
	t.Cleanup(s.close)
	return s
}

func (s *ratingStack) close() {
	_ = s.engine.Close()
	_ = s.store.Close()
	s.mw.Close()
}

type caller struct {
	id   warpnet.WarpPeerID
	priv ed25519.PrivateKey
}

func newCaller(t *testing.T) caller {
	t.Helper()
	priv, id := testKeyAndID(t)
	return caller{id: id, priv: priv}
}

// call sends one request as a remote peer, signed with signer's key.
func (s *ratingStack) call(t *testing.T, from caller, route string, signer ed25519.PrivateKey, n int) {
	t.Helper()
	for i := range n {
		msg := event.Message{
			Body:        json.RawMessage(`{}`),
			MessageId:   fmt.Sprintf("%s-%d-%d", from.id, time.Now().UnixNano(), i),
			NodeId:      from.id.String(),
			Destination: route,
			Timestamp:   time.Now().UTC(),
		}
		msg.Signature = security.Sign(signer, msg.SigningBytes())
		payload, err := json.Marshal(msg)
		require.NoError(t, err)

		client, server := stream.NewLoopbackStream(s.self, from.id, warpnet.WarpProtocolID(route))
		_, _ = s.chain(payload, server)
		_ = client.Close()
		_ = server.Close()
	}
}

// servedOf is how many of n valid requests the handler got to serve.
func (s *ratingStack) servedOf(t *testing.T, from caller, route string, n int) int {
	t.Helper()
	before := s.served
	s.call(t, from, route, from.priv, n)
	return s.served - before
}

const writeRoute = "/public/post/tweet/0.0.0"

var pipelineLimits = ratelimit.Settings{StreamWriteBurst: 40, StreamWritePerMinute: 40}

// What the auth middleware refuses reaches the engine, what the engine
// concludes reaches the rate limiter, and a peer that forges its
// signatures is served a tenth of what an honest one is.
func TestAForgerIsFlooredAndServedATenth(t *testing.T) {
	s := newRatingStack(t, testDB(t), mustKey(t), pipelineLimits)
	forger, honest, stranger := newCaller(t), newCaller(t), newCaller(t)

	s.call(t, forger, writeRoute, stranger.priv, 5) // signed with somebody else's key
	require.Eventually(t, func() bool { return s.ratings.Tier(forger.id) == rating.TierFloor },
		5*time.Second, 10*time.Millisecond, "five bad signatures floor the peer")
	assert.Equal(t, rating.TierTrusted, s.ratings.Tier(honest.id))

	assert.Equal(t, 4, s.servedOf(t, forger, writeRoute, 40), "a tenth of a burst of forty")
	assert.Equal(t, 40, s.servedOf(t, honest, writeRoute, 40), "the whole burst")

	require.Eventually(t, func() bool {
		recs, err := s.store.List(forger.id.String())
		return err == nil && len(recs) == 2
	}, 5*time.Second, 10*time.Millisecond, "a signed record per axis, in the Badger-backed CRDT")
	recs, err := s.store.List(forger.id.String())
	require.NoError(t, err)
	byDim := map[string][]domain.OffenceCount{}
	for _, rec := range recs {
		assert.Equal(t, s.self.String(), rec.ObserverID)
		byDim[rec.Dimension] = rec.Offences
	}
	assert.Equal(t, []domain.OffenceCount{{Kind: "bad_signature", Count: 5}, {Kind: "rate_limit_hit", Count: 36}},
		byDim["net"], "the 36 writes the limiter turned away are charged as well")
	assert.Equal(t, []domain.OffenceCount{{Kind: "write_flood", Count: 1}}, byDim["app"])

	resp, err := handler.StreamGetRatingHandler(s.engine)(
		[]byte(fmt.Sprintf(`{"node_id":%q}`, forger.id)), nil)
	require.NoError(t, err)
	view := resp.(domain.NodeRating)
	assert.EqualValues(t, 0, view.Overall)
	assert.Equal(t, "floor", view.Tier)
	assert.Equal(t, 1, view.Observers)
}

// Hammering the rate limit is pressure, not malice: it costs at most the
// hits' ceiling, so the limiter and the rating cannot grind an eager but
// honest peer down to the floor between them.
func TestRateLimitingAloneNeverFloorsAPeer(t *testing.T) {
	s := newRatingStack(t, testDB(t), mustKey(t), pipelineLimits)
	eager := newCaller(t)

	for range 10 {
		s.call(t, eager, writeRoute, eager.priv, 60)
	}
	require.Eventually(t, func() bool { return s.ratings.Tier(eager.id) == rating.TierWatched },
		5*time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond) // a few more flush passes
	assert.Equal(t, rating.TierWatched, s.ratings.Tier(eager.id), "300 for the hits, 20 for the flood: watched at worst")
	assert.EqualValues(t, 700, s.engine.Score(eager.id))
}

// Probing someone else's private routes is charged on the spot.
func TestProbingPrivateRoutesFloorsAPeer(t *testing.T) {
	s := newRatingStack(t, testDB(t), mustKey(t), pipelineLimits)
	prober := newCaller(t)

	s.call(t, prober, "/private/get/notifications/0.0.0", prober.priv, 5)
	require.Eventually(t, func() bool { return s.ratings.Tier(prober.id) == rating.TierFloor },
		5*time.Second, 10*time.Millisecond, "five denied private calls are worth 1000")
}

// A node that restarts still holds what it wrote; the peers it had
// floored must not come back trusted just because the process did.
func TestARestartedNodeStillHoldsItsVerdicts(t *testing.T) {
	db := testDB(t)
	key := mustKey(t)
	forger, stranger := newCaller(t), newCaller(t)

	first := newRatingStack(t, db, key, pipelineLimits)
	first.call(t, forger, writeRoute, stranger.priv, 5)
	require.Eventually(t, func() bool { return first.ratings.Tier(forger.id) == rating.TierFloor },
		5*time.Second, 10*time.Millisecond)
	first.close()

	restarted := newRatingStack(t, db, key, pipelineLimits)
	time.Sleep(200 * time.Millisecond) // ten flush-loop passes
	assert.Equal(t, rating.TierFloor, restarted.ratings.Tier(forger.id),
		"the flush loop must re-rate what the records say without waiting for new evidence")
	assert.Equal(t, 4, restarted.servedOf(t, forger, writeRoute, 40), "and the limiter must go on serving a tenth")
	assert.EqualValues(t, 0, restarted.engine.Score(forger.id), "the records did survive the restart")
}

func mustKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	priv, _ := testKeyAndID(t)
	return priv
}

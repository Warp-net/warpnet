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

 WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
 Use at your own risk. The maintainers shall not be liable for any damages or data loss
 resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package handler

import (
	"crypto/ed25519"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memRecords is an in-memory rating store with the CRDT store's hooks.
type memRecords struct {
	mu   sync.Mutex
	recs map[string]domain.RatingRecord
	puts []func(domain.RatingRecord)
}

func (m *memRecords) Put(rec domain.RatingRecord) error {
	m.mu.Lock()
	m.recs[rec.PeerID+"/"+rec.ObserverID+"/"+rec.Dimension+"/"+rec.Generation] = rec
	hooks := m.puts
	m.mu.Unlock()
	for _, h := range hooks {
		h(rec)
	}
	return nil
}

func (m *memRecords) List(peerID string) ([]domain.RatingRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.RatingRecord
	for _, rec := range m.recs {
		if rec.PeerID == peerID {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (m *memRecords) PeerIDs() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, rec := range m.recs {
		out = append(out, rec.PeerID)
	}
	return out, nil
}

func (m *memRecords) DeleteExpired(string, int64) error { return nil }
func (m *memRecords) OnPut(h func(domain.RatingRecord)) {
	m.mu.Lock()
	m.puts = append(m.puts, h)
	m.mu.Unlock()
}
func (m *memRecords) OnDelete(func(domain.RatingRecord)) {}

type everyoneKnown struct{}

func (everyoneKnown) ConnsToPeer(warpnet.WarpPeerID) []warpnet.WarpConn { return nil }

func liveRatingEngine(t *testing.T) (*rating.Engine, warpnet.PeerEmitter, warpnet.WarpPeerID) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	self, err := warpnet.IDFromPublicKey(pub)
	require.NoError(t, err)

	// At the top of the hour a charge costs its full weight: evidence
	// decays from the start of the hour it was charged in.
	hour := time.Now().UTC().Truncate(time.Hour)
	e, err := rating.NewEngine(t.Context(), &memRecords{recs: map[string]domain.RatingRecord{}}, everyoneKnown{},
		priv, warpnet.MemberNode, rating.WithFlushInterval(10*time.Millisecond),
		rating.WithClock(func() time.Time { return hour }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Close() })

	events := warpnet.NewPeerEmitter()
	e.Listen(events)
	return e, events, self
}

func newPeerID(t *testing.T) warpnet.WarpPeerID {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	id, err := warpnet.IDFromPublicKey(pub)
	require.NoError(t, err)
	return id
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// The route answers with exactly the fields the settings page reads, filled
// from what the engine really charged.
func TestRatingRouteAnswersWithTheWireShapeTheUIReads(t *testing.T) {
	engine, events, _ := liveRatingEngine(t)
	peer := newPeerID(t)
	for range 2 {
		events.Emit(warpnet.PeerEvent{PeerID: peer.String(), Type: warpnet.PeerBadSignature})
	}
	require.Eventually(t, func() bool {
		v, err := engine.View(peer)
		return err == nil && v.Overall == 500
	}, 5*time.Second, 10*time.Millisecond)

	req, _ := json.Marshal(map[string]string{"node_id": peer.String()})
	resp, err := StreamGetRatingHandler(engine)(req, nil)
	require.NoError(t, err)
	raw, err := json.Marshal(resp)
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(raw, &wire))
	assert.Equal(t, []string{"dimensions", "node_id", "observers", "overall", "tier", "updated_at"}, keysOf(wire))
	assert.Equal(t, peer.String(), wire["node_id"])
	assert.EqualValues(t, 500, wire["overall"])
	assert.Equal(t, "watched", wire["tier"])
	assert.EqualValues(t, 1, wire["observers"])

	dims := wire["dimensions"].([]any)
	require.Len(t, dims, 1)
	dim := dims[0].(map[string]any)
	assert.Equal(t, []string{"name", "recent", "score", "tier"}, keysOf(dim))
	assert.Equal(t, "net", dim["name"])
	assert.EqualValues(t, 500, dim["score"])
	assert.Equal(t, "watched", dim["tier"])

	recent := dim["recent"].([]any)
	require.Len(t, recent, 1)
	tally := recent[0].(map[string]any)
	assert.Equal(t, []string{"count", "kind", "last_at"}, keysOf(tally))
	assert.Equal(t, "bad_signature", tally["kind"])
	assert.EqualValues(t, 2, tally["count"])

	_, err = time.Parse(time.RFC3339Nano, wire["updated_at"].(string))
	assert.NoError(t, err, "updated_at is an RFC 3339 time")
}

// A node nobody has anything against answers with a clean slate the page
// shows as "Nothing to report".
func TestOwnRatingOfAnUnobservedNodeIsACleanSlate(t *testing.T) {
	engine, _, self := liveRatingEngine(t)

	resp, err := StreamGetOwnRatingHandler(engine)(nil, nil)
	require.NoError(t, err)
	raw, err := json.Marshal(resp)
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(raw, &wire))
	assert.Equal(t, self.String(), wire["node_id"])
	assert.EqualValues(t, 1000, wire["overall"])
	assert.Equal(t, "trusted", wire["tier"])
	assert.EqualValues(t, 0, wire["observers"])
	assert.Nil(t, wire["dimensions"], "no dimension is reported, and the page reads null as nothing")
}

// Whatever a remote peer sends the public route, the handler answers or
// refuses; it never panics and never answers for a node it could not parse.
func FuzzRatingRoute(f *testing.F) {
	f.Add([]byte(`{"node_id":"12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo"}`))
	f.Add([]byte(`{"node_id":""}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"node_id":"not-a-peer"}`))
	f.Add([]byte(`{"node_id":123}`))
	f.Add([]byte(``))
	f.Add([]byte(`null`))
	f.Add([]byte(`[]`))

	pub, priv, _ := ed25519.GenerateKey(nil)
	self, _ := warpnet.IDFromPublicKey(pub)
	engine, err := rating.NewEngine(f.Context(), &memRecords{recs: map[string]domain.RatingRecord{}}, everyoneKnown{},
		priv, warpnet.MemberNode)
	require.NoError(f, err)
	f.Cleanup(func() { _ = engine.Close() })
	handle := StreamGetRatingHandler(engine)

	f.Fuzz(func(t *testing.T, buf []byte) {
		resp, err := handle(buf, nil)
		if err != nil {
			return
		}
		got, ok := resp.(domain.NodeRating)
		require.True(t, ok, "a successful answer is a node rating, got %T", resp)
		var ev struct {
			NodeID string `json:"node_id"`
		}
		_ = json.Unmarshal(buf, &ev)
		if ev.NodeID == "" {
			assert.Equal(t, self.String(), got.NodeID, "no node asked for is this node")
			return
		}
		want := warpnet.FromStringToPeerID(ev.NodeID)
		require.NotEmpty(t, want, "answered for an unparseable node id %q", ev.NodeID)
		assert.Equal(t, want.String(), got.NodeID)
	})
}

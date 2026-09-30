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

package rating

import (
	"crypto/ed25519"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	lru "github.com/hashicorp/golang-lru/v2"
	log "github.com/sirupsen/logrus"
)

const fuzzPeer = "12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo"

var fuzzEpoch = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func fuzzKey(seed byte) ed25519.PrivateKey {
	var s [ed25519.SeedSize]byte
	s[0] = seed
	return ed25519.NewKeyFromSeed(s[:])
}

func fuzzID(priv ed25519.PrivateKey) string {
	id, _ := warpnet.IDFromPublicKey(priv.Public().(ed25519.PublicKey))
	return id.String()
}

// Whatever a replica delivers, authentication never panics, never accepts
// a record it cannot verify, and blames an author only for what the
// author verifiably signed.
func FuzzAuthenticate(f *testing.F) {
	f.Add(fuzzPeer, byte(1), "net", int64(0), genA, "bad_signature", uint32(1), true, false)
	f.Add(fuzzPeer, byte(2), "app", int64(-5), genB, "write_flood", uint32(7), true, false)
	f.Add("", byte(3), "mod", int64(0), genA, "audit_wrong", uint32(0), true, false)
	f.Add(fuzzPeer, byte(4), "net", int64(3), "zz", "bad_signature", uint32(1), true, true)
	f.Add(fuzzPeer, byte(5), "xyz", int64(-200), genA, "", uint32(1<<31), false, false)
	f.Add("not-a-peer", byte(6), "net", int64(-10_000), genA, "moderation_upheld", uint32(1), true, false)

	verified, err := lru.New[[sha256.Size]byte, struct{}](maxVerifiedRecords)
	if err != nil {
		f.Fatal(err)
	}
	e := &Engine{now: func() time.Time { return fuzzEpoch }, verified: verified}
	f.Fuzz(func(t *testing.T, peer string, seed byte, dim string, bucketOffset int64,
		gen, kind string, count uint32, sign, tamper bool) {
		priv := fuzzKey(seed)
		r := record{
			PeerID:     peer,
			ObserverID: fuzzID(priv),
			Dimension:  dim,
			Bucket:     int64(bucketAt(fuzzEpoch)) + bucketOffset,
			Generation: gen,
			Offences:   []domain.OffenceCount{{Kind: kind, Count: count}},
			UpdatedAt:  fuzzEpoch,
		}
		if sign {
			r, _ = r.signed(priv)
		}
		if tamper && len(r.Offences) > 0 {
			r.Offences[0].Count++
		}

		en, err := e.authenticate(domain.RatingRecord(r))
		verified := record(r).verify() == nil
		if err == nil {
			if !verified {
				t.Fatalf("accepted a record whose signature does not verify: %+v", r)
			}
			if en.observer != r.ObserverID || en.generation != r.Generation || len(en.counts) != 1 {
				t.Fatalf("entry does not reflect the record: %+v from %+v", en, r)
			}
			if en.counts[0].kind.Dimension() != en.dim {
				t.Fatalf("accepted a kind foreign to its dimension: %+v", en)
			}
			return
		}
		if isForgery(err) && !verified {
			t.Fatalf("blamed an author for a record it did not sign: %v", err)
		}
	})
}

// Any observation a module could emit is either charged or refused with
// an error; none panics, a refused one charges nothing, and none ever
// charges this node itself.
func FuzzObserve(f *testing.F) {
	for _, ev := range []string{
		"bad_signature", "connected", "discovered", "rate_limit_hit", "moderation_upheld",
		"audit_invalid", "", "unknown", "forged_observation",
	} {
		f.Add(fuzzPeer, ev, "/public/post/tweet/0.0.0")
	}
	f.Add("", "bad_signature", "")
	f.Add("12D3KooW", "rate_limit_hit", "/public/get/info/0.0.0")

	quiet(f)
	self := fuzzKey(42)
	selfID := fuzzID(self)
	clock := newClock()
	e, err := NewEngine(f.Context(), newFakeStore(warpnet.FromStringToPeerID(selfID)), acquainted(clock), self,
		warpnet.MemberNode, WithClock(clock.Now), WithFlushInterval(time.Hour))
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { _ = e.Close() })

	pending := func() (total uint32, keys []pendingKey) {
		e.mu.Lock()
		defer e.mu.Unlock()
		for key, c := range e.counters {
			keys = append(keys, key)
			for _, n := range c {
				total += n
			}
		}
		return total, keys
	}

	f.Fuzz(func(t *testing.T, peer, evType, route string) {
		before, _ := pending()
		obsErr := e.observe(warpnet.PeerEvent{PeerID: peer, Type: warpnet.PeerEventType(evType), Route: route})
		after, keys := pending()

		for _, key := range keys {
			if key.peerID == selfID {
				t.Fatalf("charged itself for %q", evType)
			}
		}
		if obsErr != nil && after != before {
			t.Fatalf("a refused observation %q about %q was charged anyway: %v", evType, peer, obsErr)
		}
		if obsErr == nil && warpnet.FromStringToPeerID(peer) == "" {
			t.Fatalf("accepted an observation naming no peer: %q", peer)
		}
		charged := after - before
		if k, ok := ParseKind(evType); ok && obsErr == nil {
			// A rate-limited write may also cross into a write flood.
			if charged != 1 && (k != KindRateLimitHit || charged != 2) {
				t.Fatalf("an offence %q must cost exactly one count, got %d", evType, charged)
			}
		}
	})
}

// quiet silences the engine's info logging for a fuzz run.
func quiet(f *testing.F) {
	f.Helper()
	level := log.GetLevel()
	log.SetLevel(log.ErrorLevel)
	f.Cleanup(func() { log.SetLevel(level) })
}

// Parsing a wire name and printing it back is the identity on everything
// the catalogue knows, and parsing never invents a name.
func FuzzWireNames(f *testing.F) {
	for k := range catalogue {
		f.Add(k.String())
	}
	f.Add("net")
	f.Add("app")
	f.Add("mod")
	f.Add("unknown")
	f.Add("")
	f.Fuzz(func(t *testing.T, name string) {
		if k, ok := ParseKind(name); ok && k.String() != name {
			t.Fatalf("kind %q printed back as %q", name, k.String())
		}
		if d, ok := ParseDimension(name); ok && d.String() != name {
			t.Fatalf("dimension %q printed back as %q", name, d.String())
		}
	})
}

// A record decoded from arbitrary bytes and scored never takes a score
// out of range.
func FuzzScoreOfArbitraryRecords(f *testing.F) {
	seed, _ := json.Marshal(domain.RatingRecord{
		PeerID: fuzzPeer, Dimension: "net", Generation: genA,
		Offences: []domain.OffenceCount{{Kind: "bad_signature", Count: 3}},
	})
	f.Add(seed, int64(0), uint16(1))
	f.Add([]byte(`{"offences":[{"kind":"rate_limit_hit","count":4294967295}]}`), int64(-1), uint16(500))
	f.Add([]byte(`not json`), int64(0), uint16(0))

	quiet(f)
	self := fuzzKey(7)
	observer := fuzzKey(8)
	f.Fuzz(func(t *testing.T, raw []byte, bucketOffset int64, hours uint16) {
		var rec domain.RatingRecord
		if json.Unmarshal(raw, &rec) != nil {
			return
		}
		rec.PeerID = fuzzPeer
		rec.ObserverID = fuzzID(observer)
		rec.Bucket = int64(bucketAt(fuzzEpoch)) + bucketOffset%1000
		rec.Generation = genA
		r, err := record(rec).signed(observer)
		if err != nil {
			t.Fatal(err)
		}

		store := newFakeStore(warpnet.FromStringToPeerID(fuzzID(self)))
		store.seed(domain.RatingRecord(r))
		clock := &fixedClock{now: fuzzEpoch}
		e, err := NewEngine(t.Context(), store, fakeConns{opened: fuzzEpoch.Add(-24 * time.Hour)}, self,
			warpnet.MemberNode, WithClock(clock.Now), WithFlushInterval(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = e.Close() }()

		clock.advance(time.Duration(hours) * time.Hour)
		peerID := warpnet.FromStringToPeerID(fuzzPeer)
		if s := e.Score(peerID); s < MinScore || s > MaxScore {
			t.Fatalf("score %d out of range", s)
		}
		view, err := e.View(peerID)
		if err != nil {
			t.Fatal(err)
		}
		if view.Overall < int32(MinScore) || view.Overall > int32(MaxScore) {
			t.Fatalf("view %d out of range", view.Overall)
		}
		if s := e.Score(peerID); s.Tier() != TierTrusted {
			t.Fatalf("one remote observer moved the peer out of trusted: %d", s)
		}
	})
}

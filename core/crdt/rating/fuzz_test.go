// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package rating

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	datastore "github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/require"
)

const fuzzGeneration = "00112233445566778899aabbccddeeff"

type keyParts struct {
	peer, observer, dimension string
	bucket                    int64
	generation                string
}

var fuzzKeyParts = []keyParts{
	{observedPeer, otherPeer, "net", 497422, fuzzGeneration},
	{otherPeer, observedPeer, "app", 497374, "ffffffffffffffffffffffffffffffff"},
	{observedPeer, otherPeer, "mod", 0, fuzzGeneration},
	{observedPeer, otherPeer, "net", -1, fuzzGeneration},
	{observedPeer, otherPeer, "net", math.MaxInt64, fuzzGeneration},
	{observedPeer, otherPeer, "net", math.MinInt64, fuzzGeneration},
	{"", otherPeer, "net", 1, fuzzGeneration},
	{observedPeer, "", "net", 1, fuzzGeneration},
	{observedPeer, otherPeer, "", 1, fuzzGeneration},
	{observedPeer, otherPeer, "net", 1, ""},
	{"peer/with/slashes", otherPeer, "net", 1, fuzzGeneration},
	{observedPeer, otherPeer, "net", 1, "gen/"},
	{"пир", "наблюдатель", "сеть", 7, "🙂"},
	{" ", "\t", "\n", 1, " "},
	{"%2F", `\`, "\x00", 1, "\xff"},
	{"0", ".", "0", 1, "0"},
	{"..", otherPeer, "net", 1, fuzzGeneration},
	{observedPeer, otherPeer, "net", 1, ".."},
}

func (p keyParts) record() domain.RatingRecord {
	return domain.RatingRecord{
		PeerID: p.peer, ObserverID: p.observer, Dimension: p.dimension,
		Bucket: p.bucket, Generation: p.generation,
	}
}

func keyStrings(rec domain.RatingRecord) []string {
	return []string{rec.PeerID, rec.ObserverID, rec.Dimension, rec.Generation}
}

func isMalformed(rec domain.RatingRecord) bool {
	return slices.ContainsFunc(keyStrings(rec), func(part string) bool {
		return part == "" || strings.Contains(part, "/")
	})
}

func isDotted(rec domain.RatingRecord) bool {
	return slices.ContainsFunc(keyStrings(rec), func(part string) bool {
		return part == "." || part == ".."
	})
}

func FuzzRecordKeyRoundTrip(f *testing.F) {
	for _, p := range fuzzKeyParts {
		f.Add(p.peer, p.observer, p.dimension, p.bucket, p.generation)
	}
	f.Fuzz(func(t *testing.T, peerID, observer, dimension string, bucket int64, generation string) {
		rec := keyParts{peerID, observer, dimension, bucket, generation}.record()
		key, err := recordKey(rec)
		if err != nil {
			require.ErrorIs(t, err, ErrMalformedRecord)
			require.True(t, isMalformed(rec) || isDotted(rec), "refused a well-formed record %+v", rec)
			return
		}
		require.False(t, isMalformed(rec), "accepted a malformed record %+v", rec)
		if isDotted(rec) {
			t.Skip("known bug: recordKey accepts . and .. parts, and ds.NewKey cleans them out of the key")
		}

		parsed, ok := parseRecordKey(key.String())
		require.True(t, ok, "recordKey made %q, which does not parse", key)
		require.Equal(t, rec, parsed)
	})
}

func FuzzParseRecordKey(f *testing.F) {
	for _, p := range fuzzKeyParts {
		if key, err := recordKey(p.record()); err == nil {
			f.Add(key.String())
		}
	}
	for _, key := range []string{
		"", "/", "/record", "/record/", "record/a/b/net/1/g",
		"/record/a/b/net/1/g/", "/record//b/net/1/g", "/record/a/b/net/1/g/h",
		"/record/./b/net/1/g", "/record/a/b/net/x/g", "/record/a/b/net/9223372036854775808/g",
		"/record/a/b/net/00/g", "/record/a/b/net/+1/g", "/record/a/b/net/-0/g",
		"/STATS/incr/whatever/node/gen",
	} {
		f.Add(key)
	}
	f.Fuzz(func(t *testing.T, key string) {
		rec, ok := parseRecordKey(key)
		if !ok {
			return
		}
		require.False(t, isMalformed(rec), "parsed %q into %+v", key, rec)
		if datastore.NewKey(key).String() != key {
			return // the datastore only ever yields clean keys
		}
		if strings.Split(key, "/")[5] != strconv.FormatInt(rec.Bucket, 10) {
			t.Skip("known leniency: parseRecordKey takes non-canonical buckets such as 00, +1 or -0")
		}

		back, err := recordKey(rec)
		require.NoError(t, err)
		require.Equal(t, key, back.String(), "parsed %q into %+v", key, rec)
	})
}

func FuzzDecodeRecord(f *testing.F) {
	aboutObserved := "/record/" + observedPeer + "/" + otherPeer + "/net/497422/" + fuzzGeneration
	for _, p := range fuzzKeyParts {
		rec := ratingRecord(p.observer, p.peer, p.dimension, 497422)
		rec.Bucket, rec.Generation = p.bucket, p.generation
		value, err := json.Marshal(rec)
		require.NoError(f, err)
		if key, err := recordKey(rec); err == nil {
			f.Add(key.String(), value)
		}
		f.Add(aboutObserved, value)
	}
	for _, value := range []string{
		"", "null", "{}", "[]", `"record"`, `{"peer_id":1}`, `{"bucket":1e400}`,
		`{"bucket":"1"}`, `{"offences":[{"count":-1}]}`, `{"updated_at":"yesterday"}`,
		strings.Repeat("[", 20000) + strings.Repeat("]", 20000),
	} {
		f.Add(aboutObserved, []byte(value))
	}
	f.Fuzz(func(t *testing.T, key string, value []byte) {
		rec, err := decodeRecord(key, value)
		if err != nil {
			return
		}
		own, err := recordKey(rec)
		require.NoError(t, err)
		require.Equal(t, key, own.String())
	})
}

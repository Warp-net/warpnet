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
	"crypto/rand"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/security"
)

type ratingError string

func (e ratingError) Error() string {
	return "rating: " + string(e)
}

const (
	ErrRecordSelfRated     = ratingError("record peer equals observer")
	ErrRecordBadPeerId     = ratingError("record peer is not a peer id")
	ErrRecordBadObserver   = ratingError("record observer is not a peer id")
	ErrRecordBadDimension  = ratingError("record dimension unknown")
	ErrRecordBadGeneration = ratingError("record generation malformed")
	ErrRecordEmptyOffences = ratingError("record carries no offences")
	ErrRecordBadKind       = ratingError("record kind unknown or foreign to its dimension")
	ErrRecordBucketFuture  = ratingError("record bucket is in the future")
	ErrRecordBucketStale   = ratingError("record bucket is past retention")
	ErrRecordNoSignature   = ratingError("record is unsigned")
	ErrRecordNoPubKey      = ratingError("cannot derive pubkey from observer id")

	ErrPrivateKeyRequired = ratingError("private key is required")
	ErrNilStore           = ratingError("record store is nil")
	ErrNilConnections     = ratingError("connections provider is nil")
)

const (
	generationBytes  = 16
	generationHexLen = generationBytes * 2
)

// signingBytes is canonical: key fields, offences ascending by kind, then the timestamp.
func signingBytes(rec domain.RatingRecord) []byte {
	var b strings.Builder
	b.WriteString(rec.PeerId)
	b.WriteByte('|')
	b.WriteString(rec.ObserverId)
	b.WriteByte('|')
	b.WriteString(rec.Dimension)
	b.WriteByte('|')
	b.WriteString(strconv.FormatInt(rec.Bucket, 10))
	b.WriteByte('|')
	b.WriteString(rec.Generation)
	b.WriteByte('|')
	for _, o := range sortedOffences(rec.Offences) {
		b.WriteString(o.Kind)
		b.WriteByte('=')
		b.WriteString(strconv.FormatUint(uint64(o.Count), 10))
		b.WriteByte(',')
	}
	b.WriteByte('|')
	b.WriteString(strconv.FormatInt(rec.UpdatedAt.UnixMilli(), 10))
	return []byte(b.String())
}

func compareOffences(a, b domain.OffenceCount) int {
	return strings.Compare(a.Kind, b.Kind)
}

func sortedOffences(in []domain.OffenceCount) []domain.OffenceCount {
	if slices.IsSortedFunc(in, compareOffences) {
		return in
	}
	out := slices.Clone(in)
	slices.SortFunc(out, compareOffences)
	return out
}

func signRecord(rec *domain.RatingRecord, priv ed25519.PrivateKey) error {
	if len(priv) != ed25519.PrivateKeySize {
		return ErrPrivateKeyRequired
	}
	rec.Offences = sortedOffences(rec.Offences)
	rec.Signature = security.Sign(priv, signingBytes(*rec))
	return nil
}

// verifyRecord checks the signature against the key embedded in the observer's peer id.
func verifyRecord(rec domain.RatingRecord) error {
	if rec.Signature == "" {
		return ErrRecordNoSignature
	}
	observer := warpnet.FromStringToPeerID(rec.ObserverId)
	if observer == "" {
		return ErrRecordBadObserver
	}
	pubKey := warpnet.FromIDToPubKey(observer)
	if len(pubKey) == 0 {
		return ErrRecordNoPubKey
	}
	return security.VerifySignature(pubKey, signingBytes(rec), rec.Signature)
}

// validateRecord enforces the structural rules a signature cannot: no
// self-rating, kinds of the record's own dimension, a bucket inside the
// retention window with one bucket of clock skew.
func validateRecord(rec domain.RatingRecord, now time.Time) error {
	if warpnet.FromStringToPeerID(rec.PeerId) == "" {
		return ErrRecordBadPeerId
	}
	if warpnet.FromStringToPeerID(rec.ObserverId) == "" {
		return ErrRecordBadObserver
	}
	if rec.PeerId == rec.ObserverId {
		return ErrRecordSelfRated
	}
	dim, ok := ParseDimension(rec.Dimension)
	if !ok {
		return ErrRecordBadDimension
	}
	if len(rec.Generation) != generationHexLen {
		return ErrRecordBadGeneration
	}
	if _, err := hex.DecodeString(rec.Generation); err != nil {
		return ErrRecordBadGeneration
	}
	if len(rec.Offences) == 0 {
		return ErrRecordEmptyOffences
	}
	for _, o := range rec.Offences {
		kind, ok := KindByName(o.Kind)
		if !ok || kind.Dimension() != dim {
			return ErrRecordBadKind
		}
	}
	if rec.Bucket > BucketOf(now)+1 {
		return ErrRecordBucketFuture
	}
	if bucketTime(rec.Bucket).Before(now.Add(-retention(dim))) {
		return ErrRecordBucketStale
	}
	return nil
}

// entryOf flattens a validated record for the indexer.
func entryOf(rec domain.RatingRecord) entry {
	dim, _ := ParseDimension(rec.Dimension)
	counts := make([]kindCount, 0, len(rec.Offences))
	for _, o := range rec.Offences {
		kind, _ := KindByName(o.Kind)
		counts = append(counts, kindCount{kind: kind, count: o.Count})
	}
	return entry{
		observer:   rec.ObserverId,
		dim:        dim,
		bucket:     rec.Bucket,
		generation: rec.Generation,
		counts:     counts,
	}
}

func offencesOf(counts map[Kind]uint32) []domain.OffenceCount {
	out := make([]domain.OffenceCount, 0, len(counts))
	for kind, n := range counts {
		out = append(out, domain.OffenceCount{Kind: kind.String(), Count: n})
	}
	return sortedOffences(out)
}

// newGeneration mints the nonce that makes every key this process writes its own.
func newGeneration() (string, error) {
	var buf [generationBytes]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

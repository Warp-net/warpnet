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

// Errors a replicated record is dropped for.
const (
	ErrRecordSelfRated     = ratingError("record peer equals observer")
	ErrRecordBadPeerID     = ratingError("record peer is not a peer id")
	ErrRecordBadObserver   = ratingError("record observer is not a peer id")
	ErrRecordBadDimension  = ratingError("record dimension unknown")
	ErrRecordBadGeneration = ratingError("record generation malformed")
	ErrRecordEmptyOffences = ratingError("record carries no offences")
	ErrRecordBadKind       = ratingError("record kind unknown or foreign to its dimension")
	ErrRecordBucketFuture  = ratingError("record bucket is in the future")
	ErrRecordBucketStale   = ratingError("record bucket is past retention")
	ErrRecordNoSignature   = ratingError("record is unsigned")
	ErrRecordNoPubKey      = ratingError("cannot derive pubkey from observer id")
)

const (
	generationBytes  = 16
	generationHexLen = generationBytes * 2
)

// record is the persisted shape with the engine's behaviour attached.
type record domain.RatingRecord

// signingBytes is canonical: key fields, offences ascending by kind, then the timestamp.
func (r record) signingBytes() []byte {
	var b strings.Builder
	b.WriteString(r.PeerID)
	b.WriteByte('|')
	b.WriteString(r.ObserverID)
	b.WriteByte('|')
	b.WriteString(r.Dimension)
	b.WriteByte('|')
	b.WriteString(strconv.FormatInt(r.Bucket, 10))
	b.WriteByte('|')
	b.WriteString(r.Generation)
	b.WriteByte('|')
	for _, o := range sortedOffences(r.Offences) {
		b.WriteString(o.Kind)
		b.WriteByte('=')
		b.WriteString(strconv.FormatUint(uint64(o.Count), 10))
		b.WriteByte(',')
	}
	b.WriteByte('|')
	b.WriteString(strconv.FormatInt(r.UpdatedAt.UnixMilli(), 10))
	return []byte(b.String())
}

// signed is the record with canonical offences and the observer's signature.
func (r record) signed(priv ed25519.PrivateKey) (record, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return r, ErrPrivateKeyRequired
	}
	r.Offences = sortedOffences(r.Offences)
	r.Signature = security.Sign(priv, r.signingBytes())
	return r, nil
}

// verify checks the signature against the key embedded in the observer's peer id.
func (r record) verify() error {
	if r.Signature == "" {
		return ErrRecordNoSignature
	}
	observer := warpnet.FromStringToPeerID(r.ObserverID)
	if observer == "" {
		return ErrRecordBadObserver
	}
	pubKey := warpnet.FromIDToPubKey(observer)
	if len(pubKey) == 0 {
		return ErrRecordNoPubKey
	}
	return security.VerifySignature(pubKey, r.signingBytes(), r.Signature)
}

// validate enforces the structural rules a signature cannot: no self-rating,
// kinds of the record's own dimension, a bucket inside the retention window
// with one bucket of clock skew.
func (r record) validate(now time.Time) error {
	if warpnet.FromStringToPeerID(r.PeerID) == "" {
		return ErrRecordBadPeerID
	}
	if warpnet.FromStringToPeerID(r.ObserverID) == "" {
		return ErrRecordBadObserver
	}
	if r.PeerID == r.ObserverID {
		return ErrRecordSelfRated
	}
	dim, ok := ParseDimension(r.Dimension)
	if !ok {
		return ErrRecordBadDimension
	}
	if len(r.Generation) != generationHexLen {
		return ErrRecordBadGeneration
	}
	if _, err := hex.DecodeString(r.Generation); err != nil {
		return ErrRecordBadGeneration
	}
	if len(r.Offences) == 0 {
		return ErrRecordEmptyOffences
	}
	for _, o := range r.Offences {
		kind, ok := ParseKind(o.Kind)
		if !ok || kind.Dimension() != dim {
			return ErrRecordBadKind
		}
	}
	b := bucket(r.Bucket)
	if b > bucketAt(now)+1 {
		return ErrRecordBucketFuture
	}
	if b.start().Before(now.Add(-dim.Retention())) {
		return ErrRecordBucketStale
	}
	return nil
}

// entry flattens a validated record for the indexer.
func (r record) entry() entry {
	dim, _ := ParseDimension(r.Dimension)
	cs := make([]kindCount, 0, len(r.Offences))
	for _, o := range r.Offences {
		kind, _ := ParseKind(o.Kind)
		cs = append(cs, kindCount{kind: kind, count: o.Count})
	}
	return entry{
		observer:   r.ObserverID,
		dim:        dim,
		bucket:     bucket(r.Bucket),
		generation: r.Generation,
		counts:     cs,
	}
}

// counts is one bucket's running totals for a peer.
type counts map[Kind]uint32

func (c counts) offences() []domain.OffenceCount {
	out := make([]domain.OffenceCount, 0, len(c))
	for kind, n := range c {
		out = append(out, domain.OffenceCount{Kind: kind.String(), Count: n})
	}
	return sortedOffences(out)
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

// newGeneration mints the nonce that makes every key this process writes its own.
func newGeneration() (string, error) {
	var buf [generationBytes]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

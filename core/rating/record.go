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
	"encoding/hex"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/security"
)

// RepoName is the CRDT key namespace for rating records.
const RepoName = "RATING"

const generationHexLen = 32 // 16 random bytes

var (
	ErrRecordSelfRated     = errors.New("rating: record subject equals observer")
	ErrRecordBadSubject    = errors.New("rating: record subject is not a peer id")
	ErrRecordBadObserver   = errors.New("rating: record observer is not a peer id")
	ErrRecordBadDimension  = errors.New("rating: record dimension unknown")
	ErrRecordBadGeneration = errors.New("rating: record generation malformed")
	ErrRecordEmptyCounts   = errors.New("rating: record carries no counts")
	ErrRecordBadKind       = errors.New("rating: record kind unknown or foreign to its dimension")
	ErrRecordBucketFuture  = errors.New("rating: record bucket is in the future")
	ErrRecordBucketStale   = errors.New("rating: record bucket is past retention")
	ErrRecordNoSignature   = errors.New("rating: record is unsigned")
	ErrRecordNoPubKey      = errors.New("rating: cannot derive pubkey from observer id")
)

// CountEntry is one offence kind and how many times this observer saw
// it in this bucket. Records carry a slice, not a map, so the wire form
// and the signing bytes are canonically ordered without any extra
// normalisation step.
type CountEntry struct {
	Kind  Kind   `json:"k"`
	Count uint32 `json:"n"`
}

// Record is one observer's view of one subject on one
// dimension in one hour, as written by one process lifetime.
//
// The (subject, observer, dimension, bucket, generation) tuple has
// exactly one writer for its whole lifetime, which is what makes LWW
// inside the key safe: the in-memory count is always authoritative, so
// eventual-consistency lag can never lose an entry, and a
// restarted process cannot clobber the history the DAG is replaying
// back to it.
type Record struct {
	Subject    string       `json:"s"`
	Observer   string       `json:"o"`
	Dim        Dimension    `json:"d"`
	Bucket     int64        `json:"b"` // unix hour
	Generation string       `json:"g"` // hex(16 random bytes), one per process start
	Counts     []CountEntry `json:"c"` // ascending by Kind
	UpdatedAt  time.Time    `json:"u"`
	Signature  string       `json:"sig"`
}

// SigningBytes is canonical and identical on every architecture: no
// map iteration, no float formatting, no locale.
func (r Record) SigningBytes() []byte {
	var b strings.Builder
	b.WriteString(r.Subject)
	b.WriteByte('|')
	b.WriteString(r.Observer)
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(int(r.Dim)))
	b.WriteByte('|')
	b.WriteString(strconv.FormatInt(r.Bucket, 10))
	b.WriteByte('|')
	b.WriteString(r.Generation)
	b.WriteByte('|')
	for _, c := range sortedCounts(r.Counts) {
		b.WriteString(strconv.Itoa(int(c.Kind)))
		b.WriteByte('=')
		b.WriteString(strconv.FormatUint(uint64(c.Count), 10))
		b.WriteByte(',')
	}
	b.WriteByte('|')
	b.WriteString(strconv.FormatInt(r.UpdatedAt.UnixMilli(), 10))
	return []byte(b.String())
}

// sortedCounts returns the entries in ascending Kind order without
// mutating the caller's slice.
func sortedCounts(in []CountEntry) []CountEntry {
	if slices.IsSortedFunc(in, func(a, b CountEntry) int { return int(a.Kind) - int(b.Kind) }) {
		return in
	}
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b CountEntry) int { return int(a.Kind) - int(b.Kind) })
	return out
}

func (r *Record) Sign(priv ed25519.PrivateKey) {
	r.Counts = sortedCounts(r.Counts)
	r.Signature = security.Sign(priv, r.SigningBytes())
}

// Verify checks the signature against the pubkey derived from the
// Observer peer id itself, the same way a moderation verdict is
// verified in core/handler/moderation.go. A record is therefore
// self-authenticating no matter which path relayed it.
func (r Record) Verify() error {
	if r.Signature == "" {
		return ErrRecordNoSignature
	}
	observer := warpnet.FromStringToPeerID(r.Observer)
	if observer == "" {
		return ErrRecordBadObserver
	}
	pubKey := warpnet.FromIDToPubKey(observer)
	if len(pubKey) == 0 {
		return ErrRecordNoPubKey
	}
	return security.VerifySignature(pubKey, r.SigningBytes(), r.Signature)
}

// Validate enforces the structural rules, independent of the
// signature. A record that fails Verify names an observer that may
// have had nothing to do with it and can only be dropped; a record
// that passes Verify and fails Validate was provably authored by the
// observer it names, and that is attributable.
func (r Record) Validate(now time.Time) error {
	if warpnet.FromStringToPeerID(r.Subject) == "" {
		return ErrRecordBadSubject
	}
	if warpnet.FromStringToPeerID(r.Observer) == "" {
		return ErrRecordBadObserver
	}
	if r.Subject == r.Observer {
		return ErrRecordSelfRated
	}
	if !r.Dim.Valid() {
		return ErrRecordBadDimension
	}
	if len(r.Generation) != generationHexLen {
		return ErrRecordBadGeneration
	}
	if _, err := hex.DecodeString(r.Generation); err != nil {
		return ErrRecordBadGeneration
	}
	if len(r.Counts) == 0 {
		return ErrRecordEmptyCounts
	}
	for _, c := range r.Counts {
		if !c.Kind.Valid() || c.Kind.Dimension() != r.Dim {
			return ErrRecordBadKind
		}
	}
	nowBucket := BucketOf(now)
	// One bucket of slack absorbs ordinary clock skew between peers.
	if r.Bucket > nowBucket+1 {
		return ErrRecordBucketFuture
	}
	if bucketTime(r.Bucket).Before(now.Add(-retention(r.Dim))) {
		return ErrRecordBucketStale
	}
	return nil
}

func (r Record) Total() uint64 {
	var total uint64
	for _, c := range r.Counts {
		total += uint64(c.Count)
	}
	return total
}

// Key is the CRDT path this record lives at:
//
//	/RATING/obs/{subject}/{observer}/{dim}/{bucket}/{generation}
func (r Record) Key() string {
	return RecordKey(r.Subject, r.Observer, r.Dim, r.Bucket, r.Generation)
}

func RecordKey(subject, observer string, dim Dimension, bucket int64, generation string) string {
	return "/" + RepoName + "/obs/" +
		subject + "/" +
		observer + "/" +
		dim.String() + "/" +
		strconv.FormatInt(bucket, 10) + "/" +
		generation
}

// KeyPrefix is the root every rating record hangs off, used for the
// startup scan.
func KeyPrefix() string { return "/" + RepoName + "/obs" }

// SubjectPrefix scopes a query to one subject.
func SubjectPrefix(subject string) string { return KeyPrefix() + "/" + subject }

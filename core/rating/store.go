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
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const (
	defaultFlushInterval = 30 * time.Second
	gcInterval           = time.Hour
	generationBytes      = 16
)

// Datastore is the subset of the CRDT datastore the store needs.
type Datastore interface {
	Get(ctx context.Context, key ds.Key) ([]byte, error)
	Put(ctx context.Context, key ds.Key, value []byte) error
	Delete(ctx context.Context, key ds.Key) error
	Query(ctx context.Context, q ds.Query) (ds.Results, error)
}

// Hooks are handed to the datastore constructor so every merged CRDT
// delta lands in the index without a re-query.
type Hooks struct {
	Put    func(key string, value []byte)
	Delete func(key string)
}

// Opener builds the datastore once the store has hooks to give it.
// The indirection exists because the hooks and the datastore are
// mutually dependent at construction time.
type Opener func(Hooks) (Datastore, error)

// Acquaintance reports how long we have been continuously connected to
// a peer in this session.
type Acquaintance interface {
	ConnectedSince(id warpnet.WarpPeerID) (time.Time, bool)
}

type Config struct {
	Ctx     context.Context
	Self    warpnet.WarpPeerID
	PrivKey ed25519.PrivateKey
	// Dimensions this node is able to witness. Observations for any
	// other dimension are refused at Record.
	Dimensions []Dimension
	Flush      time.Duration
	Now        func() time.Time
	Acquainted Acquaintance
}

type pendingKey struct {
	subject string
	dim     Dimension
	bucket  int64
}

// pendingCounts is one bucket's running totals for this generation.
type pendingCounts map[Kind]uint32

// Store holds this node's view of everyone else's standing.
//
// It never holds an opinion of itself: Record refuses a subject equal
// to Self, and Validate refuses any record whose subject equals its
// observer. A node's own rating is assembled from what its neighbours
// wrote about it and nothing else.
type Store struct {
	ctx        context.Context
	cancel     context.CancelFunc
	self       string
	privKey    ed25519.PrivateKey
	dims       []Dimension
	now        func() time.Time
	store      Datastore
	idx        *index
	acquainted Acquaintance

	// generation is minted once per process start. Every key this
	// process writes carries it, so a restart — including one that
	// came back with an empty datastore, which is the normal case for
	// a relay — cannot overwrite the records the DAG is replaying
	// back. Readers sum across generations.
	generation string

	// mu guards counters and dirty. Folding an entry is a map
	// bump and nothing else, so Record can do it inline and stay
	// non-blocking; flush deliberately does its I/O outside this lock,
	// because a Put re-enters the CRDT put hook, which calls Record.
	mu       sync.Mutex
	counters map[pendingKey]pendingCounts
	dirty    map[pendingKey]struct{}

	// fallbackMx serialises the cold-path datastore query for a
	// subject that fell out of the index, so a burst of requests for
	// one evicted peer issues one query, not a hundred.
	fallbackMx sync.Mutex

	closeOnce sync.Once
	done      chan struct{}
}

func NewStore(cfg Config, open Opener) (*Store, error) {
	if cfg.Ctx == nil {
		return nil, errors.New("rating: nil context") //nolint:err113
	}
	if cfg.Self == "" {
		return nil, errors.New("rating: empty self node id") //nolint:err113
	}
	if len(cfg.PrivKey) == 0 {
		return nil, errors.New("rating: private key is required") //nolint:err113
	}
	if len(cfg.Dimensions) == 0 {
		cfg.Dimensions = []Dimension{Network}
	}
	if cfg.Flush <= 0 {
		cfg.Flush = defaultFlushInterval
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	idx, err := newIndex()
	if err != nil {
		return nil, fmt.Errorf("rating: index: %w", err)
	}

	generation, err := newGeneration()
	if err != nil {
		return nil, fmt.Errorf("rating: generation: %w", err)
	}

	ctx, cancel := context.WithCancel(cfg.Ctx)
	s := &Store{
		ctx:        ctx,
		cancel:     cancel,
		self:       cfg.Self.String(),
		privKey:    cfg.PrivKey,
		dims:       slices.Clone(cfg.Dimensions),
		now:        cfg.Now,
		idx:        idx,
		acquainted: cfg.Acquainted,
		generation: generation,
		counters:   make(map[pendingKey]pendingCounts),
		dirty:      make(map[pendingKey]struct{}),
		done:       make(chan struct{}),
	}

	store, err := open(Hooks{Put: s.onPut, Delete: s.onDelete})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("rating: open datastore: %w", err)
	}
	s.store = store

	if err := s.scan(); err != nil {
		// A failed scan costs accuracy until the DAG re-delivers, not
		// correctness: the hooks keep feeding the index either way.
		log.Warnf("rating: initial scan: %v", err)
	}

	go s.run(cfg.Flush)
	return s, nil
}

// Record records one offence against subject. It is called from the
// middleware chain and from stream handlers, so it does no I/O: the
// count is folded into the current hour's bucket in memory and the
// flush ticker persists it.
func (s *Store) Record(subject warpnet.WarpPeerID, k Kind) error {
	return s.RecordN(subject, k, 1)
}

func (s *Store) RecordN(subject warpnet.WarpPeerID, k Kind, n uint32) error {
	if s == nil || n == 0 {
		return nil
	}
	id := subject.String()
	if id == "" {
		return ErrEmptySubject
	}
	// A node cannot rate itself. Not a fault — the invariant holds by
	// construction on both sides, here on the write side and in
	// Validate on the read side — so it costs the caller nothing.
	if id == s.self {
		return nil
	}
	// A node may only report what its role can actually witness: a
	// relay claiming to have seen a bad moderation verdict is refused
	// at the door rather than written and ignored downstream.
	if !k.Valid() {
		return ErrUnknownKind
	}
	if !slices.Contains(s.dims, k.Dimension()) {
		return fmt.Errorf("%w: %s", ErrForeignDimension, k.Dimension())
	}

	key := pendingKey{subject: id, dim: k.Dimension(), bucket: BucketOf(s.now())}

	s.mu.Lock()
	counts, ok := s.counters[key]
	if !ok {
		counts = make(pendingCounts, 1)
		s.counters[key] = counts
	}
	counts[k] += n
	s.dirty[key] = struct{}{}
	s.mu.Unlock()
	return nil
}

// Score is the subject's standing: the minimum across every dimension
// anyone has actually observed it on. A moderator that is clean on the
// wire but forges verdicts is not a mostly-fine node.
// Score returns MaxScore alongside any error: a subject we failed to
// read is not thereby an offender, and every caller is an enforcement
// point that must fail open.
func (s *Store) Score(subject warpnet.WarpPeerID) (Score, error) {
	if s == nil {
		return MaxScore, nil
	}
	obs, err := s.entriesFor(subject.String())
	if err != nil {
		return MaxScore, err
	}
	if len(obs) == 0 {
		return MaxScore, nil
	}
	now := s.now()
	worst := MaxScore
	for _, dim := range dimensionsPresent(obs) {
		if sc := s.scoreDim(obs, dim, now); sc < worst {
			worst = sc
		}
	}
	return worst, nil
}

func (s *Store) ScoreDim(subject warpnet.WarpPeerID, dim Dimension) (Score, error) {
	if s == nil {
		return MaxScore, nil
	}
	obs, err := s.entriesFor(subject.String())
	if err != nil {
		return MaxScore, err
	}
	return s.scoreDim(obs, dim, s.now()), nil
}

func (s *Store) Band(subject warpnet.WarpPeerID) (Band, error) {
	score, err := s.Score(subject)
	return BandOf(score), err
}

func (s *Store) scoreDim(obs []entry, dim Dimension, now time.Time) Score {
	return subjectiveScore(obs, dim, s.self, now, s.weightOf, s.countsTowardScore)
}

// weightOf discounts a remote observer by how much this node trusts
// it, using only first-hand evidence about that observer so the
// recursion is one level deep and always terminates.
func (s *Store) weightOf(observer string) float64 {
	// A read failure here means we know nothing against the observer,
	// which is exactly the full-weight case; entriesFor has already
	// logged it.
	obs, err := s.entriesFor(observer)
	if err != nil || len(obs) == 0 {
		return 1
	}
	now := s.now()
	worst := MaxScore
	for _, dim := range dimensionsPresent(obs) {
		if sc := ownOnlyScore(obs, dim, s.self, now); sc < worst {
			worst = sc
		}
	}
	return float64(worst) / float64(MaxScore)
}

// countsTowardScore drops observers we barely know. When no
// acquaintance source is wired the guard is inactive and the caps in
// aggregate.go carry the whole load, which they are sized to do.
func (s *Store) countsTowardScore(observer string) bool {
	if s.acquainted == nil {
		return true
	}
	since, ok := s.acquainted.ConnectedSince(warpnet.FromStringToPeerID(observer))
	if !ok {
		return false
	}
	return s.now().Sub(since) >= MinAcquaintance
}

// Public is the unweighted view, for display only.
func (s *Store) Public(subject warpnet.WarpPeerID) (domain.NodeRating, error) {
	id := subject.String()
	result := domain.NodeRating{
		NodeID:    id,
		Overall:   int32(MaxScore),
		Band:      BandTrusted.String(),
		UpdatedAt: time.Now().UTC(),
	}
	if s == nil {
		return result, nil
	}

	obs, err := s.entriesFor(id)
	if err != nil {
		return result, err
	}
	now := s.now()
	overall := MaxScore
	observers := map[string]struct{}{}

	for _, dim := range dimensionsPresent(obs) {
		score, _ := publicScore(obs, dim, now)
		if score < overall {
			overall = score
		}
		result.Dimensions = append(result.Dimensions, domain.DimensionRating{
			Name:   dim.String(),
			Score:  int32(score),
			Band:   BandOf(score).String(),
			Recent: tallyDTOs(recentTallies(obs, dim)),
		})
	}
	for _, o := range obs {
		observers[o.observer] = struct{}{}
	}

	result.Overall = int32(overall)
	result.Band = BandOf(overall).String()
	result.Observers = len(observers)
	result.UpdatedAt = now.UTC()
	return result, nil
}

// Own is this node's own standing, assembled entirely from records
// other nodes wrote about it.
func (s *Store) Own() (domain.NodeRating, error) {
	if s == nil {
		return domain.NodeRating{Overall: int32(MaxScore), Band: BandTrusted.String()}, nil
	}
	return s.Public(warpnet.FromStringToPeerID(s.self))
}

func tallyDTOs(in []tally) []domain.OffenceTally {
	out := make([]domain.OffenceTally, 0, len(in))
	for _, t := range in {
		out = append(out, domain.OffenceTally{
			Kind:   t.kind.String(),
			Count:  t.count,
			LastAt: t.lastAt,
		})
	}
	return out
}

func dimensionsPresent(obs []entry) []Dimension {
	seen := make(map[Dimension]struct{}, 3) //nolint:mnd
	for _, o := range obs {
		seen[o.dim] = struct{}{}
	}
	out := make([]Dimension, 0, len(seen))
	for _, dim := range []Dimension{Network, Application, Moderation} {
		if _, ok := seen[dim]; ok {
			out = append(out, dim)
		}
	}
	return out
}

// entriesFor reads the index, falling back to a one-off datastore
// query for a subject the index evicted.
func (s *Store) entriesFor(subject string) ([]entry, error) {
	if subject == "" {
		return nil, ErrEmptySubject
	}
	if s.idx.has(subject) {
		return s.idx.entries(subject), nil
	}

	s.fallbackMx.Lock()
	defer s.fallbackMx.Unlock()
	if s.idx.has(subject) { // another caller filled it while we waited
		return s.idx.entries(subject), nil
	}
	err := s.loadSubject(subject)
	return s.idx.entries(subject), err
}

func (s *Store) loadSubject(subject string) error {
	// Mark it present even if empty, so a peer nobody ever observed
	// does not re-query on every request.
	s.idx.ensure(subject)
	if s.store == nil {
		return nil
	}
	results, err := s.store.Query(s.ctx, ds.Query{Prefix: SubjectPrefix(subject)})
	if err != nil {
		return fmt.Errorf("rating: query subject %s: %w", subject, err)
	}
	defer func() { _ = results.Close() }()
	for r := range results.Next() {
		if r.Error != nil {
			continue
		}
		if _, err := s.admit(r.Value); err != nil {
			log.Debugf("rating: dropping record for %s: %v", subject, err)
		}
	}
	return nil
}

// scan rebuilds the index at startup. For a node whose datastore is
// empty — every relay and moderator restart — this finds nothing and
// the index is filled by the put hook as the DAG replays instead.
func (s *Store) scan() error {
	if s.store == nil {
		return nil
	}
	results, err := s.store.Query(s.ctx, ds.Query{Prefix: KeyPrefix()})
	if err != nil {
		return err
	}
	defer func() { _ = results.Close() }()

	var admitted int
	for r := range results.Next() {
		if r.Error != nil {
			continue
		}
		ok, err := s.admit(r.Value)
		if err != nil {
			log.Debugf("rating: dropping record at startup: %v", err)
		}
		if ok {
			admitted++
		}
	}
	log.Infof("rating: indexed %d entry records at startup", admitted)
	return nil
}

// admit parses, authenticates and indexes one raw record.
//
// Two failures, handled differently. A record whose signature does not
// verify names an observer that may have had nothing to do with it, so
// there is nobody to charge — drop it. A record that verifies and is
// still structurally illegal was provably authored by the observer it
// names, and that is attributable.
func (s *Store) admit(value []byte) (bool, error) {
	if len(value) == 0 {
		return false, ErrEmptyRecord
	}
	var rec Record
	if err := json.Unmarshal(value, &rec); err != nil {
		return false, fmt.Errorf("rating: unmarshal record: %w", err)
	}
	if err := rec.Verify(); err != nil {
		return false, fmt.Errorf("rating: unverifiable record for %s: %w", rec.Subject, err)
	}
	if err := rec.Validate(s.now()); err != nil {
		log.Warnf("rating: observer %s authored an invalid record: %v", rec.Observer, err)
		if chargeErr := s.Record(warpnet.FromStringToPeerID(rec.Observer), KindForgedRecord); chargeErr != nil {
			log.Warnf("rating: charging %s for a forged record: %v", rec.Observer, chargeErr)
		}
		return false, err
	}
	s.idx.put(rec)
	return true, nil
}

func (s *Store) onPut(key string, value []byte) {
	if !strings.HasPrefix(key, KeyPrefix()) {
		return
	}
	// The CRDT hook has nowhere to return to: a delta that fails to
	// admit is one peer's bad record, not this node's failure.
	if _, err := s.admit(value); err != nil {
		log.Debugf("rating: dropping merged record: %v", err)
	}
}

func (s *Store) onDelete(key string) {
	subject, observer, dim, bucket, generation, ok := parseKey(key)
	if !ok {
		return
	}
	s.idx.drop(subject, observer, dim, bucket, generation)
}

// parseKey splits /RATING/obs/{subject}/{observer}/{dim}/{bucket}/{generation}.
func parseKey(key string) (subject, observer string, dim Dimension, bucket int64, generation string, ok bool) {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(key, "/"), RepoName+"/obs/")
	if trimmed == key {
		return "", "", 0, 0, "", false
	}
	parts := strings.Split(trimmed, "/")
	const wantParts = 5
	if len(parts) != wantParts {
		return "", "", 0, 0, "", false
	}
	dim, ok = ParseDimension(parts[2])
	if !ok {
		return "", "", 0, 0, "", false
	}
	bucket, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return "", "", 0, 0, "", false
	}
	return parts[0], parts[1], dim, bucket, parts[4], true
}

func (s *Store) run(flush time.Duration) {
	defer close(s.done)

	ticker := time.NewTicker(flush)
	defer ticker.Stop()
	lastGC := s.now()

	for {
		select {
		case <-s.ctx.Done():
			// Best effort: persist what this generation accrued.
			if err := s.flush(); err != nil {
				log.Errorf("rating: final flush: %v", err)
			}
			return
		case <-ticker.C:
			if err := s.flush(); err != nil {
				log.Errorf("rating: flush: %v", err)
			}
			if s.now().Sub(lastGC) >= gcInterval {
				if err := s.gcOwnExpired(); err != nil {
					log.Warnf("rating: gc: %v", err)
				}
				lastGC = s.now()
			}
		}
	}
}

// flush persists every dirty bucket. The datastore write happens
// outside s.mu on purpose: a Put re-enters the CRDT put hook, which
// calls admit, which can call Record — holding the lock across the
// write would deadlock the node on its own entry.
func (s *Store) flush() error {
	s.mu.Lock()
	pending := make(map[pendingKey][]CountEntry, len(s.dirty))
	for key := range s.dirty {
		pending[key] = entriesOf(s.counters[key])
	}
	s.mu.Unlock()

	if len(pending) == 0 {
		return nil
	}

	// One bad bucket must not strand the rest: every failure keeps its
	// bucket dirty for the next tick and the flush carries on.
	var errs []error
	now := s.now()
	for key, counts := range pending {
		rec := Record{
			Subject:    key.subject,
			Observer:   s.self,
			Dim:        key.dim,
			Bucket:     key.bucket,
			Generation: s.generation,
			Counts:     counts,
			UpdatedAt:  now.UTC(),
		}
		if err := rec.Sign(s.privKey); err != nil {
			errs = append(errs, fmt.Errorf("sign record for %s: %w", key.subject, err))
			continue
		}

		payload, err := json.Marshal(rec)
		if err != nil {
			errs = append(errs, fmt.Errorf("marshal record for %s: %w", key.subject, err))
			continue
		}
		if s.store != nil {
			if err := s.store.Put(s.ctx, ds.NewKey(rec.Key()), payload); err != nil {
				errs = append(errs, fmt.Errorf("write record %s: %w", rec.Key(), err))
				continue
			}
		}
		// Index our own entry immediately rather than waiting
		// for the delta to come back round through the DAG.
		s.idx.put(rec)
		s.clearIfUnchanged(key, counts)
	}
	return errors.Join(errs...)
}

// clearIfUnchanged drops the dirty flag only when nothing was observed
// for that bucket while the write was in flight. If something was, the
// bucket stays dirty and the next flush writes the newer absolute
// total.
func (s *Store) clearIfUnchanged(key pendingKey, written []CountEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sameCounts(entriesOf(s.counters[key]), written) {
		delete(s.dirty, key)
	}
}

func sameCounts(a, b []CountEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func entriesOf(counts pendingCounts) []CountEntry {
	out := make([]CountEntry, 0, len(counts))
	for kind, n := range counts {
		out = append(out, CountEntry{Kind: kind, Count: n})
	}
	return sortedCounts(out)
}

// gcOwnExpired deletes this node's own records past retention. Only
// the author ever deletes, so no node can erase another's evidence —
// and nothing prunes foreign records for memory, because a CRDT delete
// is a tombstone that propagates.
func (s *Store) gcOwnExpired() error {
	if s.store == nil {
		return nil
	}
	results, err := s.store.Query(s.ctx, ds.Query{Prefix: KeyPrefix(), KeysOnly: true})
	if err != nil {
		return fmt.Errorf("rating: gc query: %w", err)
	}
	defer func() { _ = results.Close() }()

	var errs []error
	now := s.now()
	var removed int
	for r := range results.Next() {
		if r.Error != nil {
			continue
		}
		subject, observer, dim, bucket, generation, ok := parseKey(r.Key)
		if !ok || observer != s.self {
			continue
		}
		if !bucketTime(bucket).Before(now.Add(-retention(dim))) {
			continue
		}
		if err := s.store.Delete(s.ctx, ds.NewKey(r.Key)); err != nil {
			errs = append(errs, fmt.Errorf("gc delete %s: %w", r.Key, err))
			continue
		}
		s.idx.drop(subject, observer, dim, bucket, generation)
		removed++
	}
	if removed > 0 {
		log.Infof("rating: gc removed %d expired own records", removed)
	}
	return errors.Join(errs...)
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.cancel()
		select {
		case <-s.done:
		case <-time.After(5 * time.Second): //nolint:mnd
			log.Warnln("rating: flush goroutine did not stop in time")
		}
	})
	return nil
}

func newGeneration() (string, error) {
	var buf [generationBytes]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

var _ Rater = (*Store)(nil)

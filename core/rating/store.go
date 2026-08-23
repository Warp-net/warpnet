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

// Storer is the subset of the node's CRDT replica this store needs.
type Storer interface {
	Get(ctx context.Context, key ds.Key) ([]byte, error)
	Put(ctx context.Context, key ds.Key, value []byte) error
	Delete(ctx context.Context, key ds.Key) error
	Query(ctx context.Context, q ds.Query) (ds.Results, error)
}

type Hooks struct {
	Put    func(key string, value []byte)
	Delete func(key string)
}

type Opener func(Hooks) (Storer, error)

type Acquaintance interface {
	ConnectedSince(id warpnet.WarpPeerID) (time.Time, bool)
}

type Config struct {
	Ctx        context.Context
	Self       warpnet.WarpPeerID
	PrivKey    ed25519.PrivateKey
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

type Store struct {
	ctx        context.Context
	cancel     context.CancelFunc
	self       string
	privKey    ed25519.PrivateKey
	dims       []Dimension
	now        func() time.Time
	store      Storer
	idx        *index
	acquainted Acquaintance

	generation string

	mu       sync.Mutex
	counters map[pendingKey]pendingCounts
	dirty    map[pendingKey]struct{}

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
		return nil, fmt.Errorf("rating: open store: %w", err)
	}
	s.store = store

	if err := s.scan(); err != nil {
		log.Warnf("rating: initial scan: %v", err)
	}

	go s.run(cfg.Flush)
	return s, nil
}

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
	if id == s.self {
		return nil
	}
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

func (s *Store) weightOf(observer string) float64 {
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
		s.idx.put(rec)
		s.clearIfUnchanged(key, counts)
	}
	return errors.Join(errs...)
}

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

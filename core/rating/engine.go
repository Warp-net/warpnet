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
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/libp2p/go-libp2p/core/network"
	log "github.com/sirupsen/logrus"
)

const (
	defaultFlushInterval = 30 * time.Second
	gcInterval           = time.Hour
	closeTimeout         = 5 * time.Second
)

// Storer is the replicated record store; crdt.CRDTRatingStore satisfies it.
type Storer interface {
	Put(rec domain.RatingRecord) error
	List(peerId string) ([]domain.RatingRecord, error)
	DeleteExpired(dimension string, beforeBucket int64) error
	OnPut(hook func(domain.RatingRecord))
	OnDelete(hook func(domain.RatingRecord))
}

// ConnectionsProvider tells how long this node has been connected to a
// peer; the node's libp2p network satisfies it.
type ConnectionsProvider interface {
	ConnsToPeer(id warpnet.WarpPeerID) []network.Conn
}

type Option func(*Engine)

// WithClock replaces the wall clock: buckets, decay and retention follow it.
func WithClock(now func() time.Time) Option {
	return func(e *Engine) {
		if now != nil {
			e.now = now
		}
	}
}

// WithFlushInterval sets how often buffered offences are signed and written.
func WithFlushInterval(d time.Duration) Option {
	return func(e *Engine) {
		if d > 0 {
			e.flushInterval = d
		}
	}
}

type pendingKey struct {
	peerId string
	dim    Dimension
	bucket int64
}

// Engine is a node's rating of its peers: it records what this node
// witnesses, replicates it signed, and scores peers from everything the
// network has replicated back.
type Engine struct {
	ctx    context.Context
	cancel context.CancelFunc

	self          string
	privKey       ed25519.PrivateKey
	dims          []Dimension
	generation    string
	now           func() time.Time
	flushInterval time.Duration

	store Storer
	conns ConnectionsProvider
	index *indexer

	mu       sync.Mutex
	counters map[pendingKey]map[Kind]uint32
	dirty    map[pendingKey]struct{}

	loadMx sync.Mutex

	closeOnce sync.Once
	done      chan struct{}
}

// NewEngine starts the rating engine of a node; the dimensions it may
// witness follow from the node type.
func NewEngine(
	ctx context.Context,
	store Storer,
	conns ConnectionsProvider,
	privKey ed25519.PrivateKey,
	nodeType string,
	opts ...Option,
) (*Engine, error) {
	if store == nil {
		return nil, ErrNilStore
	}
	if conns == nil {
		return nil, ErrNilConnections
	}
	if len(privKey) != ed25519.PrivateKeySize {
		return nil, ErrPrivateKeyRequired
	}
	self, err := warpnet.IDFromPublicKey(privKey.Public().(ed25519.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("rating: own node id: %w", err)
	}
	generation, err := newGeneration()
	if err != nil {
		return nil, fmt.Errorf("rating: generation: %w", err)
	}
	index, err := newIndexer()
	if err != nil {
		return nil, fmt.Errorf("rating: index: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	e := &Engine{
		ctx:           ctx,
		cancel:        cancel,
		self:          self.String(),
		privKey:       privKey,
		dims:          DimensionsFor(nodeType),
		generation:    generation,
		now:           time.Now,
		flushInterval: defaultFlushInterval,
		store:         store,
		conns:         conns,
		index:         index,
		counters:      make(map[pendingKey]map[Kind]uint32),
		dirty:         make(map[pendingKey]struct{}),
		done:          make(chan struct{}),
	}
	for _, opt := range opts {
		opt(e)
	}

	store.OnPut(e.onPut)
	store.OnDelete(e.onDelete)

	go e.run()
	return e, nil
}

// Record charges one offence to a peer. It never blocks on the store. A
// kind this node's role cannot witness is a bug at the call site, not
// misbehaviour by the peer, so it is logged and dropped.
func (e *Engine) Record(peerId warpnet.WarpPeerID, kind Kind) {
	if e == nil {
		return
	}
	id := peerId.String()
	if id == "" || id == e.self {
		return
	}
	if !kind.Valid() {
		log.Warnf("rating: unknown offence kind %d for %s", kind, id)
		return
	}
	if !slices.Contains(e.dims, kind.Dimension()) {
		log.Warnf("rating: this node cannot witness %s (%s dimension)", kind, kind.Dimension())
		return
	}

	key := pendingKey{peerId: id, dim: kind.Dimension(), bucket: BucketOf(e.now())}

	e.mu.Lock()
	counts, ok := e.counters[key]
	if !ok {
		counts = make(map[Kind]uint32, 1)
		e.counters[key] = counts
	}
	counts[kind]++
	e.dirty[key] = struct{}{}
	e.mu.Unlock()
}

// Score is this node's own view of a peer: the minimum over the
// dimensions it has evidence in. Fail-open: a peer whose records cannot
// be read costs nothing, because an enforcement point must not act on
// evidence it has not seen.
func (e *Engine) Score(peerId warpnet.WarpPeerID) Score {
	if e == nil {
		return MaxScore
	}
	id := peerId.String()
	if id == "" {
		return MaxScore
	}
	p, err := e.peer(id)
	if err != nil {
		log.Warnf("rating: reading standing of %s: %v", id, err)
		return MaxScore
	}
	now := e.now()
	if score, ok := p.cachedScore(now); ok {
		return score
	}

	obs, rev := p.entries()
	worst := MaxScore
	for _, dim := range dimensionsPresent(obs) {
		if sc := localScore(obs, dim, e.self, now, e.weightOf, e.countsTowardScore); sc < worst {
			worst = sc
		}
	}
	p.setScore(worst, now, rev)
	return worst
}

func (e *Engine) Tier(peerId warpnet.WarpPeerID) Tier {
	return TierOf(e.Score(peerId))
}

// View is the public aggregate of a peer, for display: the unweighted
// median over observers per dimension, with raw recent counts.
func (e *Engine) View(peerId warpnet.WarpPeerID) (domain.NodeRating, error) {
	id := peerId.String()
	result := domain.NodeRating{
		NodeId:    id,
		Overall:   int32(MaxScore),
		Tier:      TierTrusted.String(),
		UpdatedAt: time.Now().UTC(),
	}
	if e == nil {
		return result, nil
	}

	p, err := e.peer(id)
	if err != nil {
		return result, err
	}
	obs, _ := p.entries()
	now := e.now()
	overall := MaxScore
	observers := make(map[string]struct{})

	for _, dim := range dimensionsPresent(obs) {
		score, _ := publicScore(obs, dim, now)
		if score < overall {
			overall = score
		}
		result.Dimensions = append(result.Dimensions, domain.DimensionRating{
			Name:   dim.String(),
			Score:  int32(score),
			Tier:   TierOf(score).String(),
			Recent: tallyDTOs(recentTallies(obs, dim)),
		})
	}
	for _, o := range obs {
		observers[o.observer] = struct{}{}
	}

	result.Overall = int32(overall)
	result.Tier = TierOf(overall).String()
	result.Observers = len(observers)
	result.UpdatedAt = now.UTC()
	return result, nil
}

// Own is what the network says about this node; it holds no view of itself.
func (e *Engine) Own() (domain.NodeRating, error) {
	if e == nil {
		return domain.NodeRating{Overall: int32(MaxScore), Tier: TierTrusted.String()}, nil
	}
	return e.View(warpnet.FromStringToPeerID(e.self))
}

// Close flushes what this generation still holds and stops the engine.
// The store outlives it: whoever built the store closes it afterwards.
func (e *Engine) Close() error {
	if e == nil {
		return nil
	}
	e.closeOnce.Do(func() {
		e.cancel()
		select {
		case <-e.done:
		case <-time.After(closeTimeout):
			log.Warnln("rating: flush loop did not stop in time")
		}
	})
	return nil
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

// peer returns a peer's indexed record set, loading it whole from the
// store on first use. A failed load is not remembered, so it is retried
// on the next read instead of reading as an empty peer.
func (e *Engine) peer(id string) (*indexedPeer, error) {
	if p, ok := e.index.get(id); ok {
		return p, nil
	}

	e.loadMx.Lock()
	defer e.loadMx.Unlock()
	if p, ok := e.index.get(id); ok {
		return p, nil
	}

	p := e.index.add(id) // present from here on, so a concurrent merge lands in it
	records, err := e.store.List(id)
	if err != nil {
		e.index.forget(id)
		return nil, fmt.Errorf("rating: load records of %s: %w", id, err)
	}
	for _, rec := range records {
		en, err := e.authenticate(rec)
		if err != nil {
			log.Debugf("rating: dropping stored record about %s: %v", id, err)
			continue
		}
		p.fill(en)
	}
	return p, nil
}

// authenticate checks one replicated record: its signature against the
// observer's peer id, then the structural rules.
func (e *Engine) authenticate(rec domain.RatingRecord) (entry, error) {
	if err := verifyRecord(rec); err != nil {
		return entry{}, err
	}
	if err := validateRecord(rec, e.now()); err != nil {
		return entry{}, err
	}
	return entryOf(rec), nil
}

// forged is a record that verifies but breaks the structural rules: its
// observer really authored it. An unverifiable record names an observer
// that may be innocent, and a record outside the time window is merely
// late, so neither is anyone's fault.
func forged(err error) bool {
	for _, structural := range []error{
		ErrRecordSelfRated, ErrRecordBadPeerId, ErrRecordBadDimension,
		ErrRecordBadGeneration, ErrRecordEmptyOffences, ErrRecordBadKind,
	} {
		if errors.Is(err, structural) {
			return true
		}
	}
	return false
}

// onPut indexes a merged record for a peer the index already holds. This
// is the one place a forgery is charged: on arrival, once.
func (e *Engine) onPut(rec domain.RatingRecord) {
	en, err := e.authenticate(rec)
	if err != nil {
		if forged(err) {
			log.Warnf("rating: observer %s authored an invalid record: %v", rec.ObserverId, err)
			e.Record(warpnet.FromStringToPeerID(rec.ObserverId), KindForgedRecord)
			return
		}
		log.Debugf("rating: dropping merged record about %s: %v", rec.PeerId, err)
		return
	}
	e.index.update(rec.PeerId, en)
}

func (e *Engine) onDelete(rec domain.RatingRecord) {
	e.index.forget(rec.PeerId)
}

// weightOf discounts an observer by its own first-hand standing with us.
// First-hand only, so the recursion stops here.
func (e *Engine) weightOf(observer string) float64 {
	p, err := e.peer(observer)
	if err != nil {
		return 1
	}
	obs, _ := p.entries()
	if len(obs) == 0 {
		return 1
	}
	now := e.now()
	worst := MaxScore
	for _, dim := range dimensionsPresent(obs) {
		if sc := ownOnlyScore(obs, dim, e.self, now); sc < worst {
			worst = sc
		}
	}
	return float64(worst) / float64(MaxScore)
}

func (e *Engine) countsTowardScore(observer string) bool {
	id := warpnet.FromStringToPeerID(observer)
	if id == "" {
		return false
	}
	var oldest time.Time
	for _, conn := range e.conns.ConnsToPeer(id) {
		if opened := conn.Stat().Opened; oldest.IsZero() || opened.Before(oldest) {
			oldest = opened
		}
	}
	if oldest.IsZero() {
		return false
	}
	return e.now().Sub(oldest) >= MinAcquaintance
}

func (e *Engine) run() {
	defer close(e.done)

	ticker := time.NewTicker(e.flushInterval)
	defer ticker.Stop()
	lastGC := e.now()

	for {
		select {
		case <-e.ctx.Done():
			if err := e.flush(); err != nil {
				log.Errorf("rating: final flush: %v", err)
			}
			return
		case <-ticker.C:
			if err := e.flush(); err != nil {
				log.Errorf("rating: flush: %v", err)
			}
			if e.now().Sub(lastGC) >= gcInterval {
				e.gc()
				lastGC = e.now()
			}
		}
	}
}

// flush signs and writes every bucket that changed since the last flush.
// A failed write stays dirty and is retried next time.
func (e *Engine) flush() error {
	e.mu.Lock()
	pending := make(map[pendingKey][]domain.OffenceCount, len(e.dirty))
	for key := range e.dirty {
		pending[key] = offencesOf(e.counters[key])
	}
	e.mu.Unlock()

	if len(pending) == 0 {
		return nil
	}

	var errs []error
	now := e.now().UTC()
	for key, offences := range pending {
		rec := domain.RatingRecord{
			PeerId:     key.peerId,
			ObserverId: e.self,
			Dimension:  key.dim.String(),
			Bucket:     key.bucket,
			Generation: e.generation,
			Offences:   offences,
			UpdatedAt:  now,
		}
		if err := signRecord(&rec, e.privKey); err != nil {
			errs = append(errs, fmt.Errorf("sign record for %s: %w", key.peerId, err))
			continue
		}
		if err := e.store.Put(rec); err != nil {
			errs = append(errs, fmt.Errorf("write record for %s: %w", key.peerId, err))
			continue
		}
		e.index.update(rec.PeerId, entryOf(rec))
		e.clearIfUnchanged(key, offences)
	}
	e.dropSettledBuckets()
	return errors.Join(errs...)
}

func (e *Engine) clearIfUnchanged(key pendingKey, written []domain.OffenceCount) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if slices.Equal(offencesOf(e.counters[key]), written) {
		delete(e.dirty, key)
	}
}

// dropSettledBuckets frees the counters of past hours: Record only ever
// writes the current bucket, so a flushed past bucket never changes again.
func (e *Engine) dropSettledBuckets() {
	current := BucketOf(e.now())
	e.mu.Lock()
	defer e.mu.Unlock()
	for key := range e.counters {
		if key.bucket >= current {
			continue
		}
		if _, dirty := e.dirty[key]; dirty {
			continue
		}
		delete(e.counters, key)
	}
}

// gc drops this node's own records that fell out of retention. Foreign
// records are the store's to keep: only their author may delete them.
func (e *Engine) gc() {
	now := e.now()
	for _, dim := range e.dims {
		cutoff := BucketOf(now.Add(-retention(dim)))
		if err := e.store.DeleteExpired(dim.String(), cutoff); err != nil {
			log.Warnf("rating: gc %s: %v", dim, err)
		}
	}
}

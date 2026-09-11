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

// Package rating rates peers by the offences their neighbours witness and replicate.
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
	log "github.com/sirupsen/logrus"
)

const (
	defaultFlushInterval = 30 * time.Second
	gcInterval           = time.Hour
	closeTimeout         = 5 * time.Second

	// capPerObserver and capRemoteTotal bound what remote evidence can do:
	// on its own it never pushes a peer below TierWatched.
	capPerObserver Score = 150
	capRemoteTotal Score = 400

	// minAcquaintance is how long this node must have been connected to an
	// observer before its records count. A drive-by accuser has no voice.
	minAcquaintance = time.Hour
)

// Errors NewEngine returns for missing dependencies.
const (
	ErrNilStore           = ratingError("record store is nil")
	ErrNilConnections     = ratingError("connections provider is nil")
	ErrPrivateKeyRequired = ratingError("private key is required")
)

// Storer is the replicated record store; ratingstore.Store satisfies it.
type Storer interface {
	Put(rec domain.RatingRecord) error
	List(peerID string) ([]domain.RatingRecord, error)
	DeleteExpired(dimension string, beforeBucket int64) error
	OnPut(hook func(domain.RatingRecord))
	OnDelete(hook func(domain.RatingRecord))
}

// ConnectionsProvider tells how long this node has been connected to a
// peer; the node's libp2p network satisfies it.
type ConnectionsProvider interface {
	ConnsToPeer(id warpnet.WarpPeerID) []warpnet.WarpConn
}

// Option configures an Engine at construction.
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
	peerID string
	dim    Dimension
	bucket bucket
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

	flaps       *burst
	discoveries *burst
	writes      *burst

	listeners sync.WaitGroup

	mu       sync.Mutex
	counters map[pendingKey]counts
	dirty    map[pendingKey]struct{}

	loadMu sync.Mutex

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
		dims:          Dimensions(nodeType),
		generation:    generation,
		now:           time.Now,
		flushInterval: defaultFlushInterval,
		store:         store,
		conns:         conns,
		index:         index,
		counters:      make(map[pendingKey]counts),
		dirty:         make(map[pendingKey]struct{}),
		done:          make(chan struct{}),
		flaps:         newBurst(flapWindow, flapThreshold),
		discoveries:   newBurst(discoveryWindow, discoveryThreshold),
		writes:        newBurst(writeFloodWindow, writeFloodThreshold),
	}
	for _, opt := range opts {
		opt(e)
	}

	store.OnPut(e.onPut)
	store.OnDelete(e.onDelete)

	go e.run()
	return e, nil
}

// record charges one offence to a peer. It never blocks on the store. A
// kind this node's role cannot witness is not an offence it can charge:
// a relay hears about moderation, and says nothing about it.
func (e *Engine) record(peerID warpnet.WarpPeerID, kind Kind) {
	if e == nil {
		return
	}
	id := peerID.String()
	if id == "" || id == e.self {
		return
	}
	if !kind.Valid() {
		log.Warnf("rating: unknown offence kind %d for %s", kind, id)
		return
	}
	if !slices.Contains(e.dims, kind.Dimension()) {
		return
	}

	key := pendingKey{peerID: id, dim: kind.Dimension(), bucket: bucketAt(e.now())}

	e.mu.Lock()
	c, ok := e.counters[key]
	if !ok {
		c = make(counts, 1)
		e.counters[key] = c
	}
	c[kind]++
	e.dirty[key] = struct{}{}
	e.mu.Unlock()
}

// Score is this node's own view of a peer: the minimum over the
// dimensions it has evidence in. Fail-open: a peer whose records cannot
// be read costs nothing, because an enforcement point must not act on
// evidence it has not seen.
func (e *Engine) Score(peerID warpnet.WarpPeerID) Score {
	if e == nil {
		return MaxScore
	}
	id := peerID.String()
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

	es, rev := p.entries()
	worst := MaxScore
	for _, dim := range es.dimensions() {
		worst = min(worst, e.score(es, dim, now))
	}
	p.setScore(worst, now, rev)
	return worst
}

// View is the public aggregate of a peer, for display: the unweighted
// median over observers per dimension, with raw recent counts.
func (e *Engine) View(peerID warpnet.WarpPeerID) (domain.NodeRating, error) {
	id := peerID.String()
	result := domain.NodeRating{
		NodeID:    id,
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
	es, _ := p.entries()
	now := e.now()
	overall := MaxScore
	observers := make(map[string]struct{}, len(es))

	for _, dim := range es.dimensions() {
		score, _ := es.median(dim, now)
		overall = min(overall, score)
		result.Dimensions = append(result.Dimensions, domain.DimensionRating{
			Name:   dim.String(),
			Score:  int32(score),
			Tier:   score.Tier().String(),
			Recent: es.tallies(dim),
		})
	}
	for _, en := range es {
		observers[en.observer] = struct{}{}
	}

	result.Overall = int32(overall)
	result.Tier = overall.Tier().String()
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
		e.listeners.Wait()
	})
	return nil
}

// peer returns a peer's indexed record set, loading it whole from the
// store on first use. A failed load is not remembered, so it is retried
// on the next read instead of reading as an empty peer.
func (e *Engine) peer(id string) (*indexedPeer, error) {
	if p, ok := e.index.peer(id); ok {
		return p, nil
	}

	e.loadMu.Lock()
	defer e.loadMu.Unlock()
	if p, ok := e.index.peer(id); ok {
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
	r := record(rec)
	if err := r.verify(); err != nil {
		return entry{}, err
	}
	if err := r.validate(e.now()); err != nil {
		return entry{}, err
	}
	return r.entry(), nil
}

// isForgery reports a record that verifies but breaks the structural
// rules: its observer really authored it. An unverifiable record names
// an observer that may be innocent, and a record outside the time window
// is merely late, so neither is anyone's fault.
func isForgery(err error) bool {
	for _, structural := range []error{
		ErrRecordSelfRated, ErrRecordBadPeerID, ErrRecordBadDimension,
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
		if isForgery(err) {
			log.Warnf("rating: observer %s authored an invalid record: %v", rec.ObserverID, err)
			e.record(warpnet.FromStringToPeerID(rec.ObserverID), KindForgedRecord)
			return
		}
		log.Debugf("rating: dropping merged record about %s: %v", rec.PeerID, err)
		return
	}
	e.index.update(rec.PeerID, en)
}

func (e *Engine) onDelete(rec domain.RatingRecord) {
	e.index.forget(rec.PeerID)
}

// score is this node's own view of one dimension: first-hand evidence at
// full weight, remote observers weighted by their standing and capped, so
// remote evidence alone never reaches TierDegraded.
func (e *Engine) score(es entries, dim Dimension, now time.Time) Score {
	byObserver := es.byObserver(dim)

	own := byObserver[e.self].penalty(dim, now)

	var remote Score
	for observer, group := range byObserver {
		if observer == e.self || !e.acquainted(observer) {
			continue
		}
		weighted := Score(float64(group.penalty(dim, now)) * e.weight(observer))
		remote += min(weighted, capPerObserver)
	}
	remote = min(remote, capRemoteTotal)

	return (MaxScore - own - remote).clamp()
}

// firstHand is the score from this node's own evidence alone.
func (e *Engine) firstHand(es entries, dim Dimension, now time.Time) Score {
	return (MaxScore - es.byObserver(dim)[e.self].penalty(dim, now)).clamp()
}

// weight discounts an observer by its first-hand standing with us.
// First-hand only, so the recursion stops here.
func (e *Engine) weight(observer string) float64 {
	p, err := e.peer(observer)
	if err != nil {
		return 1
	}
	es, _ := p.entries()
	if len(es) == 0 {
		return 1
	}
	now := e.now()
	worst := MaxScore
	for _, dim := range es.dimensions() {
		worst = min(worst, e.firstHand(es, dim, now))
	}
	return float64(worst) / float64(MaxScore)
}

// acquainted reports whether this node has been connected to an observer
// long enough for its records to count.
func (e *Engine) acquainted(observer string) bool {
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
	return e.now().Sub(oldest) >= minAcquaintance
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
		pending[key] = e.counters[key].offences()
	}
	e.mu.Unlock()

	if len(pending) == 0 {
		return nil
	}

	var errs []error
	now := e.now().UTC()
	for key, offences := range pending {
		rec := record{
			PeerID:     key.peerID,
			ObserverID: e.self,
			Dimension:  key.dim.String(),
			Bucket:     int64(key.bucket),
			Generation: e.generation,
			Offences:   offences,
			UpdatedAt:  now,
		}
		rec, err := rec.signed(e.privKey)
		if err != nil {
			errs = append(errs, fmt.Errorf("sign record for %s: %w", key.peerID, err))
			continue
		}
		if err := e.store.Put(domain.RatingRecord(rec)); err != nil {
			errs = append(errs, fmt.Errorf("write record for %s: %w", key.peerID, err))
			continue
		}
		e.index.update(rec.PeerID, rec.entry())
		e.clearIfUnchanged(key, offences)
	}
	e.dropSettledBuckets()
	return errors.Join(errs...)
}

func (e *Engine) clearIfUnchanged(key pendingKey, written []domain.OffenceCount) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if slices.Equal(e.counters[key].offences(), written) {
		delete(e.dirty, key)
	}
}

// dropSettledBuckets frees the counters of past hours: Record only ever
// writes the current bucket, so a flushed past bucket never changes again.
func (e *Engine) dropSettledBuckets() {
	current := bucketAt(e.now())
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
		cutoff := bucketAt(now.Add(-dim.Retention()))
		if err := e.store.DeleteExpired(dim.String(), int64(cutoff)); err != nil {
			log.Warnf("rating: gc %s: %v", dim, err)
		}
	}
}

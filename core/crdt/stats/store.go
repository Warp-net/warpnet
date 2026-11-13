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

// Package stats replicates network-wide counters over a CRDT.
package stats

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/crdt/store"
	"github.com/Warp-net/warpnet/core/warpnet"
	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/hashicorp/golang-lru/v2/expirable"
	crdt "github.com/ipfs/go-ds-crdt"
	"github.com/oklog/ulid/v2"
	log "github.com/sirupsen/logrus"
)

// GossipTopic is the pubsub topic this store's replicas converge on.
const GossipTopic = "/warpnet/stats/1.0.0"

// bitswapPrefix keeps this store's block exchange off the rating store's.
const bitswapPrefix = "/warpnet/stats"

const (
	// counterPrefix separates the counters from the datastore's own keys.
	counterPrefix = "/STATS"

	incrNamespace = "incr"
	decrNamespace = "decr"

	// generationIDBytes is the size of the random nonce that tags
	// every value this process writes. 128 bits make
	// collisions across the lifetime of the network infeasible, so
	// no two process lifetimes ever share a sub-counter — even after
	// total local-data loss followed by a re-bootstrap with the same
	// nodeID.
	generationIDBytes = 16

	flushInterval = 30 * time.Second
	purgeInterval = time.Hour

	// dagPrefix names the day DAGs: one per day of tweet creation.
	dagPrefix = "stats"
	// legacyDAG is the unnamed DAG every counter went to before day DAGs.
	legacyDAG = ""
	// retentionDays is how long a tweet's counters travel; older tweets
	// are counted from local data only.
	retentionDays = 30

	counterSize      = 8
	signedValueSize  = counterSize + ed25519.SignatureSize
	verifiedCapacity = 1 << 16
	verifiedTTL      = time.Hour

	ErrNoSigningKey = warpnet.WarpError("crdt stats: node has no ed25519 private key")
)

type counter struct {
	total   uint64
	flushed uint64
	dag     string
}

// Store is a PN-counter replicated over go-ds-crdt: every process owns
// the keys of its own generation and signs them with its node key, and a
// read sums each node's own increments less its own decrements.
type Store struct {
	crdt       *store.MerkleCRDT
	ctx        context.Context
	cancel     context.CancelFunc
	nodeID     string
	privKey    ed25519.PrivateKey
	generation string
	verified   *expirable.LRU[[sha256.Size]byte, bool]
	stopChan   chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup

	flushMu sync.Mutex

	mu       sync.Mutex
	counters map[string]*counter
}

// New creates a new CRDT-based statistics store
func New(
	ctx context.Context,
	broadcaster store.Broadcaster,
	datastore store.Datastore,
	node warpnet.P2PNode,
	router store.Router,
) (*Store, error) {
	privKey, err := nodePrivateKey(node)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)

	generation, err := newGenerationID()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to generate stats generation: %w", err)
	}

	crdtStore, err := store.New(ctx, store.Config{
		Broadcaster: broadcaster,
		Datastore:   datastore,
		Node:        node,
		Router:      router,
		Prefix:      bitswapPrefix,
		PurgeDAGs:   []string{legacyDAG},
		DAGPrefixes: []string{dagPrefix},
	})
	if err != nil {
		cancel()
		return nil, err
	}
	crdtStore.Expire(dagPrefix, firstLiveDay(time.Now()))

	s := &Store{
		crdt:       crdtStore,
		ctx:        ctx,
		cancel:     cancel,
		nodeID:     node.ID().String(),
		privKey:    privKey,
		generation: generation,
		verified:   expirable.NewLRU[[sha256.Size]byte, bool](verifiedCapacity, nil, verifiedTTL),
		stopChan:   make(chan struct{}),
		counters:   make(map[string]*counter),
	}

	s.wg.Add(1)
	go s.run()

	return s, nil
}

func nodePrivateKey(node warpnet.P2PNode) (ed25519.PrivateKey, error) {
	key := node.Peerstore().PrivKey(node.ID())
	if key == nil {
		return nil, ErrNoSigningKey
	}
	raw, err := key.Raw()
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, ErrNoSigningKey
	}
	return raw, nil
}

func (s *Store) run() {
	defer s.wg.Done()

	flushTicker := time.NewTicker(flushInterval)
	defer flushTicker.Stop()
	purgeTicker := time.NewTicker(purgeInterval)
	defer purgeTicker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-s.ctx.Done():
			return
		case <-flushTicker.C:
			if err := s.flush(); err != nil {
				log.Warnf("crdt stats: %v", err)
			}
		case <-purgeTicker.C:
			if err := s.purge(time.Now()); err != nil {
				log.Warnf("crdt stats: %v", err)
			}
		}
	}
}

// GetAggregatedStat sums, over every node with a validly signed counter,
// its increments less its decrements, at most perNodeCap per node when the
// cap is not zero, so no node can lower another's count or stand for more
// than one owner.
func (s *Store) GetAggregatedStat(key ds.Key, perNodeCap uint64) (uint64, error) {
	byNode, err := s.netByNode(key)
	if err != nil {
		return 0, err
	}
	var total uint64
	for _, count := range byNode {
		if perNodeCap > 0 {
			count = min(count, perNodeCap)
		}
		total += count
	}
	return total, nil
}

func (s *Store) netByNode(key ds.Key) (map[string]uint64, error) {
	positive, err := s.sumByNode(incrNamespace, key)
	if err != nil {
		return nil, err
	}
	negative, err := s.sumByNode(decrNamespace, key)
	if err != nil {
		return nil, err
	}
	for node, plus := range positive {
		if minus := negative[node]; plus > minus {
			positive[node] = plus - minus
			continue
		}
		delete(positive, node)
	}
	return positive, nil
}

// Increment counts one event on the tweet whose ULID is in key. A tweet
// past retention, or one without a ULID id, is counted only by the
// caller's local data.
func (s *Store) Increment(key ds.Key) error {
	s.bump(incrNamespace, key)
	return nil
}

func (s *Store) Decrement(key ds.Key) error {
	s.bump(decrNamespace, key)
	return nil
}

func (s *Store) bump(namespace string, dataKey ds.Key) {
	dag, ok := tweetDAG(dataKey)
	if !ok || !s.crdt.IsLive(dag) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cacheKey := s.counterKey(namespace, dataKey).String()
	c := s.counters[cacheKey]
	if c == nil {
		c = &counter{dag: dag}
		s.counters[cacheKey] = c
	}
	c.total++
}

func (s *Store) flush() error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	s.mu.Lock()
	pending := make(map[string]counter, len(s.counters))
	for key, c := range s.counters {
		if c.total != c.flushed {
			pending[key] = *c
		}
	}
	s.mu.Unlock()

	if len(pending) == 0 {
		return nil
	}

	deltas := make([]crdt.Delta, 0, len(pending))
	for key, c := range pending {
		delta, err := s.crdt.Set().Add(s.ctx, key, s.signedValue(key, c.total))
		if err != nil {
			return fmt.Errorf("write counter %s: %w", key, err)
		}
		delta.SetDagName(c.dag)
		deltas = append(deltas, delta)
	}
	if err := s.crdt.Publish(s.ctx, deltas...); err != nil {
		return fmt.Errorf("commit %d counters: %w", len(pending), err)
	}

	s.mu.Lock()
	for key, c := range pending {
		s.counters[key].flushed = c.total
	}
	s.mu.Unlock()
	return nil
}

// purge drops the day DAGs of tweets past retention and forgets their
// counters once flushed.
func (s *Store) purge(now time.Time) error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	s.crdt.Expire(dagPrefix, firstLiveDay(now))
	s.mu.Lock()
	for key, c := range s.counters {
		if c.total == c.flushed && !s.crdt.IsLive(c.dag) {
			delete(s.counters, key)
		}
	}
	s.mu.Unlock()
	return s.crdt.PurgeExpired(s.ctx)
}

func (s *Store) counterKey(namespace string, dataKey ds.Key) ds.Key {
	return ds.NewKey(fmt.Sprintf(
		"%s/%s/%s/%s/%s",
		counterPrefix, namespace, dataKey.String(), s.nodeID, s.generation,
	))
}

// sumByNode adds up the validly signed components of one counter per
// writing node, this process's unflushed count included.
func (s *Store) sumByNode(namespace string, key ds.Key) (map[string]uint64, error) {
	prefix := ds.NewKey(
		fmt.Sprintf("%s/%s/%s", counterPrefix, namespace, key.String()),
	)
	results, err := s.crdt.Query(s.ctx, ds.Query{Prefix: prefix.String()})
	if err != nil {
		return nil, fmt.Errorf("crdt stats: query %s: %w", prefix, err)
	}
	defer func() { _ = results.Close() }()

	byNode := make(map[string]uint64)
	for r := range results.Next() {
		if r.Error != nil {
			return nil, fmt.Errorf("crdt stats: iterate %s: %w", prefix, r.Error)
		}
		node, ok := s.verifiedNode(r.Key, r.Value)
		if !ok {
			continue
		}
		byNode[node] += binary.BigEndian.Uint64(r.Value[:counterSize])
	}

	s.mu.Lock()
	if c := s.counters[s.counterKey(namespace, key).String()]; c != nil {
		byNode[s.nodeID] += c.total - c.flushed
	}
	s.mu.Unlock()

	return byNode, nil
}

// signedValue is the big-endian total followed by the node's signature
// over the key and the total. The total leads, so the CRDT's byte-wise
// tie-break still keeps the larger of two totals.
func (s *Store) signedValue(key string, total uint64) []byte {
	value := binary.BigEndian.AppendUint64(make([]byte, 0, signedValueSize), total)
	return append(value, ed25519.Sign(s.privKey, signingBytes(key, value[:counterSize]))...)
}

// verifiedNode returns the node a component belongs to if the component
// is signed by that node's key.
func (s *Store) verifiedNode(key string, value []byte) (string, bool) {
	parts := strings.Split(key, "/")
	if len(parts) < 2 {
		return "", false
	}
	node := parts[len(parts)-2]

	digest := sha256.Sum256(append(append([]byte(key), 0), value...))
	if valid, ok := s.verified.Get(digest); ok {
		return node, valid
	}
	valid := isSigned(node, key, value)
	s.verified.Add(digest, valid)
	return node, valid
}

func isSigned(node, key string, value []byte) bool {
	if len(value) != signedValueSize {
		return false
	}
	peerID := warpnet.FromStringToPeerID(node)
	if peerID == "" {
		return false
	}
	pubKey := warpnet.FromIDToPubKey(peerID)
	if len(pubKey) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pubKey, signingBytes(key, value[:counterSize]), value[counterSize:])
}

func signingBytes(key string, total []byte) []byte {
	return append(append([]byte(key), 0), total...)
}

// tweetDAG is the day DAG of the tweet a counter key belongs to: the only
// ULID among the key's parts is the tweet id.
func tweetDAG(key ds.Key) (string, bool) {
	for _, part := range key.List() {
		if id, err := ulid.ParseStrict(part); err == nil {
			return store.DayAt(ulid.Time(id.Time())).DAG(dagPrefix), true
		}
	}
	return "", false
}

func firstLiveDay(now time.Time) store.Day {
	return store.DayAt(now) - retentionDays
}

// newGenerationID returns a hex-encoded 128-bit random nonce that
// tags every value this process writes. Survival of total
// local-data loss depends on this nonce being unique per process
// lifetime; crypto/rand without any persistent local state is what
// makes that hold.
func newGenerationID() (string, error) {
	var buf [generationIDBytes]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// Close stops the CRDT store
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	if err := s.flush(); err != nil {
		log.Warnf("crdt stats: final flush: %v", err)
	}
	s.stopOnce.Do(func() { close(s.stopChan) })
	s.wg.Wait()
	s.cancel()
	return s.crdt.Close()
}

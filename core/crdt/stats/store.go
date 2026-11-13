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
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/crdt/store"
	"github.com/Warp-net/warpnet/core/warpnet"
	ds "github.com/Warp-net/warpnet/database/datastore"
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
	// retentionDays is how long a tweet's counters travel; older tweets
	// are counted from local data only.
	retentionDays = 30

	counterSize     = 8
	signedValueSize = counterSize + ed25519.SignatureSize

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
	key := node.Peerstore().PrivKey(node.ID())
	if key == nil {
		return nil, ErrNoSigningKey
	}
	privKey, err := key.Raw()
	if err != nil || len(privKey) != ed25519.PrivateKeySize {
		return nil, ErrNoSigningKey
	}

	ctx, cancel := context.WithCancel(ctx)

	generation, err := newGenerationID()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to generate stats generation: %w", err)
	}

	s := &Store{
		ctx:        ctx,
		cancel:     cancel,
		nodeID:     node.ID().String(),
		privKey:    privKey,
		generation: generation,
		stopChan:   make(chan struct{}),
		counters:   make(map[string]*counter),
	}

	crdtStore, err := store.New(ctx, store.Config{
		Broadcaster: broadcaster,
		Datastore:   datastore,
		Node:        node,
		Router:      router,
		Prefix:      bitswapPrefix,
		DAGPrefixes: []string{dagPrefix},
		IsValid:     s.isValid,
		IsOlder:     isOlderTotal,
	})
	if err != nil {
		cancel()
		return nil, err
	}
	crdtStore.Expire(dagPrefix, firstLiveDay(time.Now()))
	s.crdt = crdtStore

	s.wg.Add(1)
	go s.run()

	return s, nil
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
	positive, err := s.sumByNode(incrNamespace, key)
	if err != nil {
		return 0, err
	}
	negative, err := s.sumByNode(decrNamespace, key)
	if err != nil {
		return 0, err
	}
	var total uint64
	for node, plus := range positive {
		minus := negative[node]
		if plus <= minus {
			continue
		}
		count := plus - minus
		if perNodeCap > 0 {
			count = min(count, perNodeCap)
		}
		total += count
	}
	return total, nil
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

// sumByNode adds up the components of one counter per writing node, this
// process's unflushed count included. Only checked writes reach the set.
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
		if len(r.Value) < counterSize {
			continue
		}
		byNode[counterNode(r.Key)] += binary.BigEndian.Uint64(r.Value[:counterSize])
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

// isValid accepts a received counter only if the node in its key signed
// it and it sits in the DAG of its tweet's day.
func (s *Store) isValid(dagName, key string, value []byte) bool {
	if dag, ok := tweetDAG(ds.NewKey(key)); !ok || dag != dagName {
		return false
	}
	if len(value) != signedValueSize {
		return false
	}
	pubKey := warpnet.FromIDToPubKey(warpnet.FromStringToPeerID(counterNode(key)))
	if len(pubKey) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pubKey, signingBytes(key, value[:counterSize]), value[counterSize:])
}

// isOlderTotal reports a counter value below the one already chosen: a node's
// running total only grows, so a smaller one is a replay.
func isOlderTotal(_ string, value, current []byte) bool {
	if len(value) < counterSize || len(current) < counterSize {
		return false
	}
	return binary.BigEndian.Uint64(value[:counterSize]) < binary.BigEndian.Uint64(current[:counterSize])
}

// counterNode is the node a counter key belongs to: keys end in
// /<node>/<generation>.
func counterNode(key string) string {
	parts := strings.Split(key, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2]
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

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
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

const (
	maxIndexedPeers = 16384

	// scoreTTL bounds how stale a memoised score may be while a peer's
	// records are unchanged; half-lives are measured in hours and days.
	scoreTTL = 15 * time.Second
)

type slot struct {
	observer   string
	dim        Dimension
	bucket     int64
	generation string
}

func slotOf(e entry) slot {
	return slot{observer: e.observer, dim: e.dim, bucket: e.bucket, generation: e.generation}
}

// indexedPeer is one peer's complete record set plus its memoised score.
type indexedPeer struct {
	mu    sync.Mutex
	slots map[slot][]kindCount
	rev   uint64

	score    Score
	scoredAt time.Time
	scoreRev uint64
}

// set replaces one record's counts.
func (p *indexedPeer) set(s slot, counts []kindCount) {
	p.mu.Lock()
	p.slots[s] = counts
	p.rev++
	p.mu.Unlock()
}

// fill adds a loaded record unless a merge already delivered a newer one.
func (p *indexedPeer) fill(e entry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := slotOf(e)
	if _, ok := p.slots[s]; ok {
		return
	}
	p.slots[s] = e.counts
	p.rev++
}

func (p *indexedPeer) entries() ([]entry, uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]entry, 0, len(p.slots))
	for s, counts := range p.slots {
		out = append(out, entry{
			observer:   s.observer,
			dim:        s.dim,
			bucket:     s.bucket,
			generation: s.generation,
			counts:     counts,
		})
	}
	return out, p.rev
}

func (p *indexedPeer) cachedScore(now time.Time) (Score, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.scoredAt.IsZero() || p.scoreRev != p.rev || now.Sub(p.scoredAt) >= scoreTTL {
		return 0, false
	}
	return p.score, true
}

// setScore memoises a score computed from the entries of revision rev.
func (p *indexedPeer) setScore(score Score, at time.Time, rev uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if rev != p.rev {
		return
	}
	p.score, p.scoredAt, p.scoreRev = score, at, rev
}

// indexer holds the record sets of recently scored peers so a score is
// arithmetic, not a datastore query. Eviction is index-only: the store
// keeps everything, and an evicted peer is reloaded on its next read.
type indexer struct {
	peers *lru.Cache[string, *indexedPeer]
}

func newIndexer() (*indexer, error) {
	peers, err := lru.New[string, *indexedPeer](maxIndexedPeers)
	if err != nil {
		return nil, err
	}
	return &indexer{peers: peers}, nil
}

func (i *indexer) get(peerId string) (*indexedPeer, bool) {
	return i.peers.Get(peerId)
}

func (i *indexer) has(peerId string) bool {
	return i.peers.Contains(peerId)
}

func (i *indexer) add(peerId string) *indexedPeer {
	p := &indexedPeer{slots: make(map[slot][]kindCount)}
	i.peers.Add(peerId, p)
	return p
}

// update replaces one record of a peer the index already holds. Creating
// a peer from a single record would shadow the rest of its history in
// the store, so an unknown peer is left to be loaded whole on its next read.
func (i *indexer) update(peerId string, e entry) {
	if p, ok := i.peers.Peek(peerId); ok {
		p.set(slotOf(e), e.counts)
	}
}

func (i *indexer) forget(peerId string) {
	i.peers.Remove(peerId)
}

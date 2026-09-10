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

	lru "github.com/hashicorp/golang-lru/v2"
)

const maxIndexedSubjects = 16384

type slot struct {
	observer   string
	dim        Dimension
	bucket     int64
	generation string
}

type indexer struct {
	mu   sync.RWMutex
	data map[string]map[slot][]CountEntry
	// rev holds a value unique to the peerId's current entry set, so a
	// cached score knows it is stale without comparing entries. Values
	// come from a counter and are never reused, so a peerId evicted
	// and re-indexered can never collide with a score cached before.
	rev     map[string]uint64
	lastRev uint64
	lru     *lru.Cache[string, struct{}]
}

func newIndexer() (*indexer, error) {
	idx := &indexer{
		data: make(map[string]map[slot][]CountEntry),
		rev:  make(map[string]uint64),
	}
	cache, err := lru.NewWithEvict[string, struct{}](
		maxIndexedSubjects,
		func(peerId string, _ struct{}) {
			idx.mu.Lock()
			delete(idx.data, peerId)
			delete(idx.rev, peerId)
			idx.mu.Unlock()
		},
	)
	if err != nil {
		return nil, err
	}
	idx.lru = cache
	return idx, nil
}

func (i *indexer) maxIndexedSubjects() int {
	return maxIndexedSubjects
}

// put inserts or replaces one record's counts, creating the peerId if
// needed. Only the full-load paths (scan, loadSubject) may call it: they
// have just read everything the datastore holds for the peerId, so the
// entry set they build is complete.
func (i *indexer) put(rec Record) {
	i.apply(rec, true)
}

// update replaces one record's counts only if the peerId is already
// indexered, and reports whether it applied. The incremental paths — the
// CRDT put hook and the flush — must use it: creating a peerId from a
// single record would shadow the rest of its history in the datastore,
// and scoring would run on that sliver until the next eviction.
func (i *indexer) update(rec Record) bool {
	return i.apply(rec, false)
}

func (i *indexer) apply(rec Record, create bool) bool {
	key := slot{
		observer:   rec.Observer,
		dim:        rec.Dim,
		bucket:     rec.Bucket,
		generation: rec.Generation,
	}

	i.mu.Lock()
	slots, ok := i.data[rec.Subject]
	if !ok {
		if !create {
			i.mu.Unlock()
			return false
		}
		slots = make(map[slot][]CountEntry, 1)
		i.data[rec.Subject] = slots
	}
	slots[key] = rec.Counts
	i.lastRev++
	i.rev[rec.Subject] = i.lastRev
	i.mu.Unlock()

	// Outside the lock: eviction takes the same mutex.
	i.lru.Add(rec.Subject, struct{}{})
	return true
}

func (i *indexer) drop(peerId, observer string, dim Dimension, bucket int64, generation string) {
	key := slot{observer: observer, dim: dim, bucket: bucket, generation: generation}

	i.mu.Lock()
	defer i.mu.Unlock()
	slots, ok := i.data[peerId]
	if !ok {
		return
	}
	delete(slots, key)
	if len(slots) == 0 {
		delete(i.data, peerId)
		delete(i.rev, peerId)
		return
	}
	i.lastRev++
	i.rev[peerId] = i.lastRev
}

func (i *indexer) entries(peerId string) []entry {
	i.mu.RLock()
	slots, ok := i.data[peerId]
	if !ok {
		i.mu.RUnlock()
		return nil
	}
	out := make([]entry, 0, len(slots))
	for key, counts := range slots {
		out = append(out, entry{
			observer:   key.observer,
			dim:        key.dim,
			bucket:     key.bucket,
			generation: key.generation,
			counts:     counts,
		})
	}
	i.mu.RUnlock()

	i.lru.Get(peerId) // refresh recency
	return out
}

func (i *indexer) ensure(peerId string) {
	i.mu.Lock()
	if _, ok := i.data[peerId]; !ok {
		i.data[peerId] = make(map[slot][]CountEntry)
	}
	i.mu.Unlock()
	i.lru.Add(peerId, struct{}{})
}

func (i *indexer) has(peerId string) bool {
	i.mu.RLock()
	_, ok := i.data[peerId]
	i.mu.RUnlock()
	return ok
}

// revision is 0 for a peerId with no indexered records; a score cached
// against 0 is the empty-peerId fast path.
func (i *indexer) revision(peerId string) uint64 {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.rev[peerId]
}

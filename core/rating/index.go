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

// maxIndexedSubjects bounds the in-memory index. Past it the
// least-recently-touched subject is dropped from the index — and only
// from the index. Nothing is ever deleted from the CRDT to save
// memory: a CRDT delete is a tombstone that propagates and would
// destroy other nodes' evidence. An evicted subject simply falls back
// to a prefix query on its next scoring and re-enters.
const maxIndexedSubjects = 16384

// slot identifies one CRDT key within a subject. One writer owns each
// slot for its whole lifetime, so the value is replaced wholesale.
type slot struct {
	observer   string
	dim        Dimension
	bucket     int64
	generation string
}

// index is the read side of the store. Scoring runs once per inbound
// request, so it must never touch the datastore; the index is kept
// current by the CRDT put/delete hooks instead.
type index struct {
	mu   sync.RWMutex
	data map[string]map[slot][]CountEntry
	lru  *lru.Cache[string, struct{}]
}

func newIndex() (*index, error) {
	idx := &index{data: make(map[string]map[slot][]CountEntry)}
	cache, err := lru.NewWithEvict[string, struct{}](
		maxIndexedSubjects,
		func(subject string, _ struct{}) {
			idx.mu.Lock()
			delete(idx.data, subject)
			idx.mu.Unlock()
		},
	)
	if err != nil {
		return nil, err
	}
	idx.lru = cache
	return idx, nil
}

// put inserts or replaces one record's counts.
func (i *index) put(rec Record) {
	key := slot{
		observer:   rec.Observer,
		dim:        rec.Dim,
		bucket:     rec.Bucket,
		generation: rec.Generation,
	}

	i.mu.Lock()
	slots, ok := i.data[rec.Subject]
	if !ok {
		slots = make(map[slot][]CountEntry, 1)
		i.data[rec.Subject] = slots
	}
	slots[key] = rec.Counts
	i.mu.Unlock()

	// Outside the lock: eviction takes the same mutex.
	i.lru.Add(rec.Subject, struct{}{})
}

// drop removes one record, addressed the way a CRDT delete hook
// reports it.
func (i *index) drop(subject, observer string, dim Dimension, bucket int64, generation string) {
	key := slot{observer: observer, dim: dim, bucket: bucket, generation: generation}

	i.mu.Lock()
	defer i.mu.Unlock()
	slots, ok := i.data[subject]
	if !ok {
		return
	}
	delete(slots, key)
	if len(slots) == 0 {
		delete(i.data, subject)
	}
}

// entries snapshots a subject. The returned slice is safe to use
// without the lock; the CountEntry slices inside are never mutated in
// place, only replaced wholesale by put.
func (i *index) entries(subject string) []entry {
	i.mu.RLock()
	slots, ok := i.data[subject]
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

	i.lru.Get(subject) // refresh recency
	return out
}

// ensure marks a subject present even with no records, so a peer
// nobody ever observed does not re-query the datastore on every
// request.
func (i *index) ensure(subject string) {
	i.mu.Lock()
	if _, ok := i.data[subject]; !ok {
		i.data[subject] = make(map[slot][]CountEntry)
	}
	i.mu.Unlock()
	i.lru.Add(subject, struct{}{})
}

// has reports whether the subject is currently indexed, so the store
// knows when to fall back to a datastore query.
func (i *index) has(subject string) bool {
	i.mu.RLock()
	_, ok := i.data[subject]
	i.mu.RUnlock()
	return ok
}

func (i *index) subjects() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]string, 0, len(i.data))
	for subject := range i.data {
		out = append(out, subject)
	}
	return out
}

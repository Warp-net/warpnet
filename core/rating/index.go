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

type index struct {
	mu   sync.RWMutex
	data map[string]map[slot][]CountEntry
	// rev holds a value unique to the subject's current entry set, so a
	// cached score knows it is stale without comparing entries. Values
	// come from a counter and are never reused, so a subject evicted
	// and re-indexed can never collide with a score cached before.
	rev     map[string]uint64
	lastRev uint64
	lru     *lru.Cache[string, struct{}]
}

func newIndex() (*index, error) {
	idx := &index{
		data: make(map[string]map[slot][]CountEntry),
		rev:  make(map[string]uint64),
	}
	cache, err := lru.NewWithEvict[string, struct{}](
		maxIndexedSubjects,
		func(subject string, _ struct{}) {
			idx.mu.Lock()
			delete(idx.data, subject)
			delete(idx.rev, subject)
			idx.mu.Unlock()
		},
	)
	if err != nil {
		return nil, err
	}
	idx.lru = cache
	return idx, nil
}

// put inserts or replaces one record's counts, creating the subject if
// needed. Only the full-load paths (scan, loadSubject) may call it: they
// have just read everything the datastore holds for the subject, so the
// entry set they build is complete.
func (i *index) put(rec Record) {
	i.apply(rec, true)
}

// update replaces one record's counts only if the subject is already
// indexed, and reports whether it applied. The incremental paths — the
// CRDT put hook and the flush — must use it: creating a subject from a
// single record would shadow the rest of its history in the datastore,
// and scoring would run on that sliver until the next eviction.
func (i *index) update(rec Record) bool {
	return i.apply(rec, false)
}

func (i *index) apply(rec Record, create bool) bool {
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
		delete(i.rev, subject)
		return
	}
	i.lastRev++
	i.rev[subject] = i.lastRev
}

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

func (i *index) ensure(subject string) {
	i.mu.Lock()
	if _, ok := i.data[subject]; !ok {
		i.data[subject] = make(map[slot][]CountEntry)
	}
	i.mu.Unlock()
	i.lru.Add(subject, struct{}{})
}

func (i *index) has(subject string) bool {
	i.mu.RLock()
	_, ok := i.data[subject]
	i.mu.RUnlock()
	return ok
}

// revision is 0 for a subject with no indexed records; a score cached
// against 0 is the empty-subject fast path.
func (i *index) revision(subject string) uint64 {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.rev[subject]
}

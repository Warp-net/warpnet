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

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	crdt "github.com/ipfs/go-ds-crdt"
	"github.com/ipfs/go-ds-crdt/pb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

const (
	// LegacyDAG is the unnamed DAG every write went to before day DAGs.
	LegacyDAG = ""

	// maxDeltaSize keeps a block well below the 1 MiB value limit of the
	// echo node's in-memory Badger.
	maxDeltaSize = 1 << 18

	day = 24 * time.Hour
)

// Epochs names one DAG per prefix and UTC day and tells live days from
// expired ones, so a store drops whole days and refuses their heads.
type Epochs struct {
	known map[string]struct{}

	mu        sync.RWMutex
	firstLive map[string]int64
}

// NewEpochs limits the live DAGs to the given prefixes; with none, any
// prefix is live until it gets a cutoff.
func NewEpochs(prefixes ...string) *Epochs {
	e := &Epochs{firstLive: make(map[string]int64)}
	if len(prefixes) > 0 {
		e.known = make(map[string]struct{}, len(prefixes))
		for _, p := range prefixes {
			e.known[p] = struct{}{}
		}
	}
	return e
}

// DayOf is the UTC day number of t.
func DayOf(t time.Time) int64 {
	return t.UTC().Unix() / int64(day/time.Second)
}

// DayDAG is the name of the DAG holding one prefix's writes of one day.
func DayDAG(prefix string, dayNum int64) string {
	return prefix + "-" + strconv.FormatInt(dayNum, 10)
}

// Expire makes every day of prefix before firstLiveDay expired.
func (e *Epochs) Expire(prefix string, firstLiveDay int64) {
	e.mu.Lock()
	e.firstLive[prefix] = firstLiveDay
	e.mu.Unlock()
}

// IsLive reports a well-formed day DAG of a known prefix that is neither
// expired nor more than a day ahead of this clock.
func (e *Epochs) IsLive(dagName string) bool {
	i := strings.LastIndexByte(dagName, '-')
	if i <= 0 {
		return false
	}
	prefix := dagName[:i]
	dayNum, err := strconv.ParseInt(dagName[i+1:], 10, 64)
	if err != nil || dayNum < 0 || dayNum > DayOf(time.Now())+1 {
		return false
	}
	if e.known != nil {
		if _, ok := e.known[prefix]; !ok {
			return false
		}
	}
	e.mu.RLock()
	firstLive, ok := e.firstLive[prefix]
	e.mu.RUnlock()
	return !ok || dayNum >= firstLive
}

// Purge drops every DAG with a head that is no longer live.
func (e *Epochs) Purge(ctx context.Context, store *crdt.MerkleCRDT) error {
	seen := make(map[string]struct{})
	var errs []error
	for _, head := range store.InternalStats(ctx).Heads {
		if _, ok := seen[head.DAGName]; ok || e.IsLive(head.DAGName) {
			continue
		}
		seen[head.DAGName] = struct{}{}
		removed, err := store.PurgeDAG(ctx, head.DAGName)
		if err != nil {
			errs = append(errs, fmt.Errorf("purge %q: %w", head.DAGName, err))
			continue
		}
		log.Infof("crdt: dropped expired DAG %q of %d blocks", head.DAGName, removed)
	}
	return errors.Join(errs...)
}

// Broadcaster wraps b so that heads of DAGs that are not live travel in
// neither direction.
func (e *Epochs) Broadcaster(b Broadcaster) Broadcaster {
	return epochBroadcaster{Broadcaster: b, epochs: e}
}

func (e *Epochs) liveHeads(data []byte) ([]byte, bool) {
	var msg pb.CRDTBroadcast
	if err := proto.Unmarshal(data, &msg); err != nil {
		return nil, false
	}
	heads := slices.DeleteFunc(slices.Clone(msg.GetHeads()), func(h *pb.Head) bool {
		return !e.IsLive(h.GetDagName())
	})
	if len(heads) == 0 {
		return nil, false
	}
	if len(heads) == len(msg.GetHeads()) {
		return data, true
	}
	msg.Heads = heads
	live, err := proto.Marshal(&msg)
	return live, err == nil
}

type epochBroadcaster struct {
	Broadcaster

	epochs *Epochs
}

func (b epochBroadcaster) Broadcast(ctx context.Context, data []byte) error {
	live, ok := b.epochs.liveHeads(data)
	if !ok {
		return nil
	}
	return b.Broadcaster.Broadcast(ctx, live)
}

func (b epochBroadcaster) Next(ctx context.Context) ([]byte, error) {
	for {
		data, err := b.Broadcaster.Next(ctx)
		if err != nil {
			return nil, err
		}
		if live, ok := b.epochs.liveHeads(data); ok {
			return live, nil
		}
	}
}

// Publish writes deltas in as few blocks as maxDeltaSize allows, one DAG per
// block. It never shares a delta with crdt.Batch, whose pending delta
// survives a failed commit.
func Publish(ctx context.Context, store *crdt.MerkleCRDT, deltas []crdt.Delta) error {
	slices.SortStableFunc(deltas, func(a, b crdt.Delta) int {
		return strings.Compare(a.GetDagName(), b.GetDagName())
	})
	var (
		block crdt.Delta
		size  int
	)
	for _, delta := range deltas {
		if block != nil && (block.GetDagName() != delta.GetDagName() || size+delta.Size() > maxDeltaSize) {
			if _, err := store.Publish(ctx, block); err != nil {
				return err
			}
			block = nil
		}
		if block == nil {
			block, size = delta, delta.Size()
			continue
		}
		elems, err := block.GetElements()
		if err != nil {
			return err
		}
		moreElems, err := delta.GetElements()
		if err != nil {
			return err
		}
		block.SetElements(append(elems, moreElems...))
		size += delta.Size()
	}
	if block == nil {
		return nil
	}
	_, err := store.Publish(ctx, block)
	return err
}

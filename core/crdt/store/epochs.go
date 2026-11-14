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
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ds-crdt/pb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

const day = 24 * time.Hour

// Day is a UTC day number since the Unix epoch.
type Day int64

// DayAt is the UTC day t falls on.
func DayAt(t time.Time) Day {
	return Day(t.UTC().Unix() / int64(day/time.Second))
}

// DAG is the name of the DAG holding one prefix's writes of this day.
func (d Day) DAG(prefix string) string {
	return prefix + "-" + strconv.FormatInt(int64(d), 10)
}

// Expire makes every day of prefix before firstLive expired.
func (m *MerkleCRDT) Expire(prefix string, firstLive Day) {
	m.mu.Lock()
	m.firstLive[prefix] = firstLive
	m.mu.Unlock()
}

// IsLive reports a well-formed day DAG of an allowed prefix that is neither
// expired nor more than a day ahead of this clock.
func (m *MerkleCRDT) IsLive(dagName string) bool {
	i := strings.LastIndexByte(dagName, '-')
	if i <= 0 {
		return false
	}
	prefix := dagName[:i]
	dayNum, err := strconv.ParseInt(dagName[i+1:], 10, 64)
	if err != nil || dayNum < 0 || strconv.FormatInt(dayNum, 10) != dagName[i+1:] || Day(dayNum) > DayAt(time.Now())+1 {
		return false
	}
	if m.prefixes != nil {
		if _, ok := m.prefixes[prefix]; !ok {
			return false
		}
	}
	m.mu.RLock()
	firstLive, ok := m.firstLive[prefix]
	m.mu.RUnlock()
	return !ok || Day(dayNum) >= firstLive
}

// PurgeExpired drops every DAG with a head that is no longer live.
func (m *MerkleCRDT) PurgeExpired(ctx context.Context) error {
	seen := make(map[string]struct{})
	var errs []error
	for _, head := range m.InternalStats(ctx).Heads {
		if _, ok := seen[head.DAGName]; ok || m.IsLive(head.DAGName) {
			continue
		}
		seen[head.DAGName] = struct{}{}
		removed, err := m.PurgeDAG(ctx, head.DAGName)
		if err != nil {
			errs = append(errs, fmt.Errorf("purge %q: %w", head.DAGName, err))
			continue
		}
		log.Infof("crdt: dropped expired DAG %q of %d blocks", head.DAGName, removed)
	}
	return errors.Join(errs...)
}

func (m *MerkleCRDT) liveHeads(data []byte) ([]byte, bool) {
	var msg pb.CRDTBroadcast
	if err := proto.Unmarshal(data, &msg); err != nil {
		return nil, false
	}
	heads := slices.DeleteFunc(slices.Clone(msg.GetHeads()), func(h *pb.Head) bool {
		return !m.IsLive(h.GetDagName())
	})
	for _, h := range heads {
		if c, err := cid.Cast(h.GetCid()); err == nil {
			m.lineage.announce(c, h.GetDagName())
		}
	}
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

// liveBroadcaster lets heads of DAGs that are not live travel in neither
// direction.
type liveBroadcaster struct {
	Broadcaster

	crdt *MerkleCRDT
}

func (b liveBroadcaster) Broadcast(ctx context.Context, data []byte) error {
	live, ok := b.crdt.liveHeads(data)
	if !ok {
		return nil
	}
	return b.Broadcaster.Broadcast(ctx, live)
}

func (b liveBroadcaster) Next(ctx context.Context) ([]byte, error) {
	for {
		data, err := b.Broadcaster.Next(ctx)
		if err != nil {
			return nil, err
		}
		if live, ok := b.crdt.liveHeads(data); ok {
			return live, nil
		}
	}
}

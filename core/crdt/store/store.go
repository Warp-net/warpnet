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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	ds "github.com/Warp-net/warpnet/database/datastore"
	datastore "github.com/ipfs/go-datastore"
	crdt "github.com/ipfs/go-ds-crdt"
	"github.com/ipfs/go-ds-crdt/pb"
	ipld "github.com/ipfs/go-ipld-format"
	log "github.com/sirupsen/logrus"
)

type Broadcaster interface {
	Broadcast(ctx context.Context, data []byte) error
	Next(ctx context.Context) ([]byte, error)
}

type Datastore interface {
	Get(ctx context.Context, key ds.Key) ([]byte, error)
	Has(ctx context.Context, key ds.Key) (bool, error)
	GetSize(ctx context.Context, key ds.Key) (int, error)
	Query(ctx context.Context, q ds.Query) (ds.Results, error)
	Put(ctx context.Context, key ds.Key, value []byte) error
	Delete(ctx context.Context, key ds.Key) error
	Sync(ctx context.Context, prefix ds.Key) error
	Close() error
}

type Router interface {
	FindProvidersAsync(context.Context, warpnet.WarpCID, int) <-chan warpnet.WarpAddrInfo
}

const (
	rebroadcastInterval = 5 * time.Minute
	dagSyncerTimeout    = 1 * time.Minute
	numWorkers          = 16

	// maxDeltaSize keeps a block well below the 1 MiB value limit of the
	// echo node's in-memory Badger.
	maxDeltaSize = 1 << 18
)

type Config struct {
	Broadcaster Broadcaster
	Datastore   Datastore
	Node        warpnet.P2PNode
	Router      Router
	Prefix      string
	PutHook     func(k ds.Key, v []byte)
	DeleteHook  func(k ds.Key)
	// DAGPrefixes limits the live day DAGs to these prefixes; with none,
	// any prefix is live until it is expired.
	DAGPrefixes []string
	// IsValid decides, before a received block is merged, whether one of
	// its writes belongs in the store: signed by its author and kept in
	// the DAG of its own day.
	IsValid func(dagName, key string, value []byte) bool
	// IsOlder reports a received value that an earlier write by the same
	// author superseded: a replay, never merged.
	IsOlder func(key string, value, current []byte) bool
}

// MerkleCRDT is a Merkle-CRDT kept in day DAGs: it publishes writes in
// capped blocks, drops expired days and refuses their heads.
type MerkleCRDT struct {
	*crdt.MerkleCRDT

	prefixes  map[string]struct{}
	mu        sync.RWMutex
	firstLive map[string]Day

	data         datastore.Datastore
	lineage      *lineage
	isValid      func(dagName, key string, value []byte) bool
	isOlder      func(key string, value, current []byte) bool
	setNamespace atomic.Pointer[datastore.Key]
}

const ErrNoValidator = warpnet.WarpError("crdt store: no write validator")

func New(ctx context.Context, cfg Config) (*MerkleCRDT, error) {
	if cfg.IsValid == nil || cfg.IsOlder == nil {
		return nil, ErrNoValidator
	}
	m := &MerkleCRDT{firstLive: make(map[string]Day), isValid: cfg.IsValid, isOlder: cfg.IsOlder}
	if len(cfg.DAGPrefixes) > 0 {
		m.prefixes = make(map[string]struct{}, len(cfg.DAGPrefixes))
		for _, p := range cfg.DAGPrefixes {
			m.prefixes[p] = struct{}{}
		}
	}

	baseStore := ds.MutexWrap(cfg.Datastore)
	blockstore := ds.NewIdStore(ds.NewBlockstore(baseStore, ds.WriteThrough(true)))

	bitswapNetwork := warpnet.NewBitswapNetwork(cfg.Node, warpnet.BitswapPrefix(cfg.Prefix))
	bitswapExchange := warpnet.NewBitswapExchange(ctx, bitswapNetwork, cfg.Router, blockstore)

	for _, p := range cfg.Node.Network().Peers() {
		bitswapExchange.PeerConnected(p)
	}

	dagService := warpnet.NewDAGService(warpnet.NewBlockService(blockstore, bitswapExchange))

	stdLog := log.WithContext(ctx)

	opts := crdt.DefaultOptions()
	opts.Logger = newDedupLogger(stdLog.WithField("store", cfg.Prefix))
	opts.PutHook = cfg.PutHook
	opts.DeleteHook = cfg.DeleteHook
	opts.RebroadcastInterval = rebroadcastInterval
	opts.DAGSyncerTimeout = dagSyncerTimeout
	opts.NumWorkers = numWorkers
	opts.RepairInterval = 0
	opts.MultiHeadProcessing = true

	if err := m.purgeNotLive(ctx, baseStore, dagService, opts); err != nil {
		log.Errorf("failed to purge DAGs that are not live: %v", err)
	}

	m.data = baseStore
	m.lineage = newLineage(dagService)
	crdtStore, err := crdt.NewMerkleCRDT(
		baseStore,
		ds.NewKey(""),
		m.lineage,
		liveBroadcaster{Broadcaster: cfg.Broadcaster, crdt: m},
		opts,
		&crdt.MerkleCRDTOptions{DeltaFactory: func() crdt.Delta {
			return &checkedDelta{Delta: &pb.Delta{}, crdt: m}
		}},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CRDT store: %w", err)
	}

	m.MerkleCRDT = crdtStore
	if ns, ok := m.readSetNamespace(); ok {
		m.setNamespace.Store(&ns)
	} else {
		log.Warnf("crdt store %s: go-ds-crdt layout changed, replayed old values are not refused", cfg.Prefix)
	}
	for _, head := range crdtStore.InternalStats(ctx).Heads {
		m.lineage.announce(head.Cid, head.DAGName)
	}

	go func() {
		// Unconditional: a clean Close() mid-sync can leave a hole with the dirty bit unset.
		if err := crdtStore.Repair(ctx); err != nil {
			log.Errorf("failed to repair store: %v", err)
		}
	}()

	return m, nil
}

// purgeNotLive drops, before the checked store starts, every DAG that is
// not live: its writes may predate the checks, and only the stock delta
// sees all of them, so nothing it merged is left behind. It reads local
// blocks only and talks to no one.
func (m *MerkleCRDT) purgeNotLive(ctx context.Context, data datastore.Datastore, dags ipld.DAGService, opts *crdt.Options) error {
	plain, err := crdt.NewMerkleCRDT(data, ds.NewKey(""), dags, idleBroadcaster{}, opts, nil)
	if err != nil {
		return err
	}
	defer func() { _ = plain.Close() }()

	var errs []error
	for _, head := range plain.InternalStats(ctx).Heads {
		if m.IsLive(head.DAGName) {
			continue
		}
		if _, err := plain.PurgeDAG(ctx, head.DAGName); err != nil {
			errs = append(errs, fmt.Errorf("purge %q: %w", head.DAGName, err))
		}
	}
	return errors.Join(errs...)
}

type idleBroadcaster struct{}

func (idleBroadcaster) Broadcast(context.Context, []byte) error { return nil }

func (idleBroadcaster) Next(ctx context.Context) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// Publish writes deltas in as few blocks as maxDeltaSize allows, one DAG per
// block. It never shares a delta with crdt.Batch, whose pending delta
// survives a failed commit.
func (m *MerkleCRDT) Publish(ctx context.Context, deltas ...crdt.Delta) error {
	slices.SortStableFunc(deltas, func(a, b crdt.Delta) int {
		return strings.Compare(a.GetDagName(), b.GetDagName())
	})
	var (
		block crdt.Delta
		size  int
	)
	for _, delta := range deltas {
		if block != nil && (block.GetDagName() != delta.GetDagName() || size+delta.Size() > maxDeltaSize) {
			if _, err := m.MerkleCRDT.Publish(ctx, block); err != nil {
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
	_, err := m.MerkleCRDT.Publish(ctx, block)
	return err
}

type dedupLogger struct {
	base *log.Entry
	mu   sync.Mutex
	seen map[string]bool
}

func newDedupLogger(base *log.Entry) *dedupLogger {
	return &dedupLogger{
		base: base,
		seen: make(map[string]bool),
	}
}

func (l *dedupLogger) isDup(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if ok := l.seen[key]; ok {
		return true
	}
	for k := range l.seen {
		delete(l.seen, k)
	}
	l.seen[key] = true

	return false
}

func (l *dedupLogger) Info(args ...any) {
	if !l.base.Logger.IsLevelEnabled(log.InfoLevel) {
		return
	}
	if !l.isDup(fmt.Sprint(args...)) {
		l.base.Info(args...)
	}
}

func (l *dedupLogger) Infof(format string, args ...any) {
	if !l.base.Logger.IsLevelEnabled(log.InfoLevel) {
		return
	}
	if !l.isDup(format) {
		l.base.Infof(format, args...)
	}
}

func (l *dedupLogger) Debug(args ...any)                 { l.base.Debug(args...) }
func (l *dedupLogger) Debugf(format string, args ...any) { l.base.Debugf(format, args...) }
func (l *dedupLogger) Warn(args ...any)                  { l.base.Warn(args...) }
func (l *dedupLogger) Warnf(format string, args ...any)  { l.base.Warnf(format, args...) }
func (l *dedupLogger) Error(args ...any)                 { l.base.Error(args...) }
func (l *dedupLogger) Errorf(format string, args ...any) { l.base.Errorf(format, args...) }
func (l *dedupLogger) Fatal(args ...any)                 { l.base.Fatal(args...) }
func (l *dedupLogger) Fatalf(format string, args ...any) { l.base.Fatalf(format, args...) }
func (l *dedupLogger) Panic(args ...any)                 { l.base.Panic(args...) }
func (l *dedupLogger) Panicf(format string, args ...any) { l.base.Panicf(format, args...) }

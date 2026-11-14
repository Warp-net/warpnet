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
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
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
	// Validate refuses, before a received block is merged, a write that
	// does not belong in the store: not signed by its author or not kept
	// in the DAG of its own day.
	Validate func(dagName, key string, value []byte) error
	// Compare orders two versions of one key by the same author, like
	// cmp.Compare: a received version below the current one is a replay
	// and is never merged.
	Compare func(value, current []byte) int
}

// MerkleCRDT is a Merkle-CRDT kept in day DAGs: it publishes writes in
// capped blocks, drops expired days and refuses their heads.
type MerkleCRDT struct {
	*crdt.MerkleCRDT

	prefixes  map[string]struct{}
	mu        sync.RWMutex
	firstLive map[string]Day

	lineage  *lineage
	validate func(dagName, key string, value []byte) error
}

func New(ctx context.Context, cfg Config) (*MerkleCRDT, error) {
	m := &MerkleCRDT{firstLive: make(map[string]Day), validate: cfg.Validate}
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

	if err := m.purgeNotLive(ctx, cfg.Datastore, baseStore, dagService, opts); err != nil {
		log.Errorf("failed to purge DAGs that are not live: %v", err)
	}

	if putHook := opts.PutHook; putHook != nil {
		opts.PutHook = func(k ds.Key, v []byte) {
			if current, err := baseStore.Get(ctx, setValueKey(k.String())); err == nil && bytes.Equal(current, v) {
				putHook(k, v)
			}
		}
	}

	m.lineage = newLineage(dagService)
	crdtStore, err := crdt.NewMerkleCRDT(
		versionedStore{Datastore: baseStore, compare: cfg.Compare},
		ds.NewKey(""),
		m.lineage,
		liveBroadcaster{Broadcaster: cfg.Broadcaster, crdt: m},
		opts,
		&crdt.MerkleCRDTOptions{
			DeltaFactory: func() crdt.Delta {
				return &checkedDelta{Delta: &pb.Delta{}, crdt: m}
			},
			Namespaces: crdt.InternalNamespaces{Set: setNamespace},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CRDT store: %w", err)
	}

	m.MerkleCRDT = crdtStore
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

// go-ds-crdt keeps, under the set namespace, each key's chosen value at
// k/<key>/v and the priority it was chosen with at k/<key>/p (vendored
// set.go).
const (
	setNamespace   = "s"
	setValuePrefix = "/" + setNamespace + "/k"
	setValueSuffix = "/v"
	setPrioSuffix  = "/p"
)

var unrankedPriority = binary.AppendUvarint(nil, 1)

func setValueKey(key string) datastore.Key {
	return datastore.NewKey(setValuePrefix + key + setValueSuffix)
}

// versionedStore is the datastore go-ds-crdt keeps the set in. The
// priority a block claims is not signed, so every stored priority reads as
// the lowest: each write then reaches the value it would replace, and the
// value is kept only if compare does not put it below the current one. The
// set takes this decision under its own lock, so two blocks cannot both
// pass a check against the same old value.
type versionedStore struct {
	datastore.Datastore

	compare func(value, current []byte) int
}

func (v versionedStore) Get(ctx context.Context, key datastore.Key) ([]byte, error) {
	if k := key.String(); strings.HasPrefix(k, setValuePrefix+"/") && strings.HasSuffix(k, setPrioSuffix) {
		return unrankedPriority, nil
	}
	return v.Datastore.Get(ctx, key)
}

func (v versionedStore) Put(ctx context.Context, key datastore.Key, value []byte) error {
	if k := key.String(); strings.HasPrefix(k, setValuePrefix+"/") && strings.HasSuffix(k, setValueSuffix) {
		current, err := v.Datastore.Get(ctx, key)
		if err == nil && v.compare(value, current) < 0 {
			return nil
		}
	}
	return v.Datastore.Put(ctx, key, value)
}

// purgeNotLive drops, before the checked store starts, every DAG that is
// not live: its writes may predate the checks, and only the stock delta
// sees all of them, so nothing it merged is left behind. It reads local
// blocks only and talks to no one. Walking the unnamed DAG from before day
// DAGs takes minutes, so when it is there, or no DAG is live at all, the
// whole store is cleared at once instead: whatever was live is fetched
// again.
func (m *MerkleCRDT) purgeNotLive(ctx context.Context, owner Datastore, data datastore.Datastore, dags ipld.DAGService, opts *crdt.Options) error {
	plain, err := crdt.NewMerkleCRDT(data, ds.NewKey(""), dags, idleBroadcaster{}, opts, nil)
	if err != nil {
		return err
	}
	heads := plain.InternalStats(ctx).Heads

	store, canClear := owner.(clearer)
	if canClear && m.isWholeStoreExpired(heads) {
		if err := plain.Close(); err != nil {
			return err
		}
		log.Infof("crdt: clearing a store with no live DAG or with the unnamed one")
		return store.Clear()
	}
	defer func() { _ = plain.Close() }()

	var errs []error
	for _, head := range heads {
		if m.IsLive(head.DAGName) {
			continue
		}
		if _, err := plain.PurgeDAG(ctx, head.DAGName); err != nil {
			errs = append(errs, fmt.Errorf("purge %q: %w", head.DAGName, err))
		}
	}
	return errors.Join(errs...)
}

// clearer is a datastore that drops all of its keys at once.
type clearer interface {
	Clear() error
}

func (m *MerkleCRDT) isWholeStoreExpired(heads []crdt.Head) bool {
	if len(heads) == 0 {
		return false
	}
	for _, head := range heads {
		if head.DAGName == "" {
			return true
		}
	}
	return !slices.ContainsFunc(heads, func(h crdt.Head) bool { return m.IsLive(h.DAGName) })
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

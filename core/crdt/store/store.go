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
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	ds "github.com/Warp-net/warpnet/database/datastore"
	crdt "github.com/ipfs/go-ds-crdt"
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
	PurgeDAGs   []string
	Epochs      *Epochs
}

// Store is a Merkle-CRDT kept in day DAGs: it publishes writes in capped
// blocks, drops expired days and refuses their heads.
type Store struct {
	*crdt.MerkleCRDT

	epochs *Epochs
}

func New(ctx context.Context, cfg Config) (*Store, error) {
	if cfg.Epochs == nil {
		cfg.Epochs = NewEpochs()
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

	crdtStore, err := crdt.NewMerkleCRDT(
		baseStore,
		ds.NewKey(""),
		dagService,
		cfg.Epochs.Broadcaster(cfg.Broadcaster),
		opts,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CRDT store: %w", err)
	}

	go func() {
		for _, name := range cfg.PurgeDAGs {
			if _, err := crdtStore.PurgeDAG(ctx, name); err != nil {
				log.Errorf("failed to purge DAG %q: %v", name, err)
			}
		}
		// Unconditional: a clean Close() mid-sync can leave a hole with the dirty bit unset.
		if err := crdtStore.Repair(ctx); err != nil {
			log.Errorf("failed to repair store: %v", err)
		}
	}()

	return &Store{MerkleCRDT: crdtStore, epochs: cfg.Epochs}, nil
}

// Publish writes deltas in as few blocks as maxDeltaSize allows, one DAG per
// block. It never shares a delta with crdt.Batch, whose pending delta
// survives a failed commit.
func (s *Store) Publish(ctx context.Context, deltas ...crdt.Delta) error {
	slices.SortStableFunc(deltas, func(a, b crdt.Delta) int {
		return strings.Compare(a.GetDagName(), b.GetDagName())
	})
	var (
		block crdt.Delta
		size  int
	)
	for _, delta := range deltas {
		if block != nil && (block.GetDagName() != delta.GetDagName() || size+delta.Size() > maxDeltaSize) {
			if _, err := s.MerkleCRDT.Publish(ctx, block); err != nil {
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
	_, err := s.MerkleCRDT.Publish(ctx, block)
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

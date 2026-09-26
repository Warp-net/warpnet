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
)

type Config struct {
	Broadcaster Broadcaster
	Datastore   Datastore
	Node        warpnet.P2PNode
	Router      Router
	Prefix      string
	PutHook     func(k ds.Key, v []byte)
	DeleteHook  func(k ds.Key)
}

func New(ctx context.Context, cfg Config) (*crdt.Datastore, error) {
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
	opts.Logger = newDedupLogger(stdLog.WithField("store", cfg.Prefix), time.Minute*10) // nolint:mnd
	opts.PutHook = cfg.PutHook
	opts.DeleteHook = cfg.DeleteHook
	opts.RebroadcastInterval = rebroadcastInterval
	opts.DAGSyncerTimeout = dagSyncerTimeout
	opts.NumWorkers = numWorkers
	opts.RepairInterval = 0
	opts.MultiHeadProcessing = true

	crdtStore, err := crdt.New(
		baseStore,
		ds.NewKey(""),
		dagService,
		cfg.Broadcaster,
		opts,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CRDT store: %w", err)
	}

	go func() {
		if err := crdtStore.Repair(ctx); err != nil {
			log.Errorf("failed to repair store: %v", err)
		}
	}()

	return crdtStore, nil
}

type dedupLogger struct {
	base      *log.Entry
	window    time.Duration
	mu        sync.Mutex
	seen      map[string]time.Time
	lastSweep time.Time
}

func newDedupLogger(base *log.Entry, window time.Duration) *dedupLogger {
	return &dedupLogger{
		base:      base,
		window:    window,
		seen:      make(map[string]time.Time),
		lastSweep: time.Now(),
	}
}

func (l *dedupLogger) isDup(key string) bool {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if last, ok := l.seen[key]; ok && now.Sub(last) < l.window {
		return true
	}
	l.seen[key] = now

	if now.Sub(l.lastSweep) >= l.window {
		for k, t := range l.seen {
			if now.Sub(t) >= l.window {
				delete(l.seen, k)
			}
		}
		l.lastSweep = now
	}

	return false
}

func (l *dedupLogger) Info(args ...interface{}) {
	if !l.base.Logger.IsLevelEnabled(log.InfoLevel) {
		return
	}
	if !l.isDup(fmt.Sprint(args...)) {
		l.base.Info(args...)
	}
}

func (l *dedupLogger) Infof(format string, args ...interface{}) {
	if !l.base.Logger.IsLevelEnabled(log.InfoLevel) {
		return
	}
	if !l.isDup(format) {
		l.base.Infof(format, args...)
	}
}

func (l *dedupLogger) Debug(args ...interface{})                 { l.base.Debug(args...) }
func (l *dedupLogger) Debugf(format string, args ...interface{}) { l.base.Debugf(format, args...) }
func (l *dedupLogger) Warn(args ...interface{})                  { l.base.Warn(args...) }
func (l *dedupLogger) Warnf(format string, args ...interface{})  { l.base.Warnf(format, args...) }
func (l *dedupLogger) Error(args ...interface{})                 { l.base.Error(args...) }
func (l *dedupLogger) Errorf(format string, args ...interface{}) { l.base.Errorf(format, args...) }
func (l *dedupLogger) Fatal(args ...interface{})                 { l.base.Fatal(args...) }
func (l *dedupLogger) Fatalf(format string, args ...interface{}) { l.base.Fatalf(format, args...) }
func (l *dedupLogger) Panic(args ...interface{})                 { l.base.Panic(args...) }
func (l *dedupLogger) Panicf(format string, args ...interface{}) { l.base.Panicf(format, args...) }

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

package db

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/Warp-net/warpnet/security"
	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
	"github.com/docker/go-units"
	ds "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	log "github.com/sirupsen/logrus"
)

/*
BadgerDB is a high-performance, embedded key-value database
written in Go, utilizing LSM-trees (Log-Structured Merge-Trees) for efficient
data storage and processing. It is designed for high-load scenarios that require
minimal latency and high throughput.

Key Features:
  - Embedded: Operates within the application without the need for a separate database: server.
  - Key-Value Store: Enables storing and retrieving data by key using efficient indexing.
  - LSM Architecture: Provides high write speed due to log-structured data storage.
  - Zero GC Overhead: Minimizes garbage collection impact by directly working with mmap and byte slices.
  - ACID Transactions: Supports transactions with snapshot isolation.
  - In-Memory and Disk Mode: Allows storing data in RAM or on SSD/HDD.
  - Low Resource Consumption: Suitable for embedded systems and server applications with limited memory.

BadgerDB is used in cases where:
  - High Performance is required: It is faster than traditional disk-based databases (e.g., BoltDB) due to the LSM structure.
  - Embedded Storage is needed: No need to run a separate database: server (unlike Redis, PostgreSQL, etc.).
  - Efficient Streaming Writes are required: Suitable for logs, caches, message brokers, and other write-intensive workloads.
  - Transaction Support is necessary: Allows safely executing multiple operations within a single transaction.
  - Large Data Volumes are handled: Supports sharding and disk offloading, useful for processing massive datasets.
  - Flexibility is key: Easily integrates into distributed systems and P2P applications.

BadgerDB is especially useful for systems where high write speed, low overhead, and the ability to operate without an external database: server are critical.
https://github.com/dgraph-io/badger
*/

type DBError string

func (e DBError) Error() string { return string(e) }

const (
	defaultDiscardRatioGC = 0.5
	defaultIntervalGC     = time.Hour
	defaultSleepGC        = time.Second
	firstRunLockFile      = "run.lock"
	sequenceKey           = "SEQUENCE"

	ErrNotRunning    = DBError("DB is not running")
	ErrWrongPassword = DBError("wrong username or password")

	PermanentTTL = math.MaxInt64
)

var DefaultIteratorOptions = badger.DefaultIteratorOptions

type (
	DatastoreKey = ds.Key
	Datastore    = ds.Datastore
	Batching     = ds.Batching
)

type Options struct {
	discardRatioGC float64
	intervalGC     time.Duration
	sleepGC        time.Duration
	isInMemory     bool
}

func DefaultOptions() *Options {
	return &Options{
		discardRatioGC: defaultDiscardRatioGC,
		intervalGC:     defaultIntervalGC,
		sleepGC:        defaultSleepGC,
		isInMemory:     false,
	}
}

func (opt Options) WithDiscardRatioGC(ratio float64) Options {
	opt.discardRatioGC = ratio
	return opt
}

func (opt Options) WithIntervalGC(interval time.Duration) Options {
	opt.intervalGC = interval
	return opt
}

func (opt Options) WithSleepGC(sleep time.Duration) Options {
	opt.sleepGC = sleep
	return opt
}

func (opt Options) WithInMemory(v bool) Options {
	opt.isInMemory = v
	return opt
}

func NewKey(s string) DatastoreKey {
	return ds.NewKey(s)
}

type LocalDatastore struct {
	badger   *badger.DB
	sequence *badger.Sequence

	dbPath          string
	hasFirstRunFlag bool
	isRunning       *atomic.Bool

	badgerOpts     badger.Options
	discardRatioGC float64
	intervalGC     time.Duration
	sleepGC        time.Duration

	stopChan chan struct{}
}

func NewLocalDatastore(dbPath string, o *Options) (*LocalDatastore, error) {
	badgerOpts := badger.
		DefaultOptions(dbPath).
		WithSyncWrites(false).
		WithIndexCacheSize(256 << 20).
		WithCompression(options.ZSTD).
		WithNumCompactors(2).
		WithLoggingLevel(badger.ERROR).
		WithBlockCacheSize(512 << 20)
	if o.isInMemory {
		badgerOpts = badgerOpts.WithDir("")
		badgerOpts = badgerOpts.WithValueDir("")
		badgerOpts = badgerOpts.WithInMemory(true)
	}

	if o.intervalGC == 0 {
		o.intervalGC = defaultIntervalGC
	}
	if o.discardRatioGC == 0 {
		o.discardRatioGC = defaultDiscardRatioGC
	}
	if o.sleepGC == 0 {
		o.sleepGC = defaultSleepGC
	}

	storage := &LocalDatastore{
		badger: nil, stopChan: make(chan struct{}), isRunning: new(atomic.Bool),
		sequence: nil, badgerOpts: badgerOpts, dbPath: dbPath, hasFirstRunFlag: findFirstRunFlag(dbPath),
		discardRatioGC: o.discardRatioGC, intervalGC: o.intervalGC, sleepGC: o.sleepGC,
	}

	return storage, nil
}

func findFirstRunFlag(dbPath string) (found bool) {
	_, err := os.Stat(filepath.Join(dbPath, firstRunLockFile))
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		panic(err.Error())
	}
	return true
}

func (d *LocalDatastore) IsFirstRun() bool {
	return !d.hasFirstRunFlag
}

func (d *LocalDatastore) writeFirstRunFlag() {
	path := filepath.Join(d.dbPath, firstRunLockFile)
	log.Infof("database: lock file created: %s", path)
	f, _ := os.Create(path)
	if f != nil {
		_ = f.Close()
	}
	d.hasFirstRunFlag = true
}

func (d *LocalDatastore) Run(username, password string) (err error) {
	if username == "" || password == "" {
		return DBError("database: username or password is empty")
	}
	hashSum := security.ConvertToSHA256([]byte(username + "@" + password))
	execOpts := d.badgerOpts.WithEncryptionKey(hashSum)

	d.badger, err = badger.Open(execOpts)
	if errors.Is(err, badger.ErrEncryptionKeyMismatch) {
		return ErrWrongPassword
	}
	if err != nil {
		return err
	}
	d.isRunning.Store(true)
	d.sequence, err = d.badger.GetSequence([]byte(sequenceKey), 100)
	if err != nil {
		return err
	}

	if !d.hasFirstRunFlag {
		d.writeFirstRunFlag()
	}

	if !d.badgerOpts.InMemory {
		go d.runEventualGC()
	}

	return nil
}

func (d *LocalDatastore) runEventualGC() {
	log.Infoln("database: garbage collection started")
	gcTicker := time.NewTicker(d.intervalGC)
	defer gcTicker.Stop()

	dirTicker := time.NewTicker(time.Second)
	defer dirTicker.Stop()

	_ = d.badger.RunValueLogGC(d.discardRatioGC)
	for {
		select {
		case <-dirTicker.C:
			isFound := findFirstRunFlag(d.dbPath)
			if !isFound {
				log.Errorln("database: folder was emptied")
				os.Exit(1)
			}
		case <-gcTicker.C:
			for {
				err := d.badger.RunValueLogGC(d.discardRatioGC)
				if errors.Is(err, badger.ErrNoRewrite) ||
					errors.Is(err, badger.ErrRejected) {
					break
				}
				time.Sleep(d.sleepGC)
			}
			log.Infoln("database: garbage collection complete")
		case <-d.stopChan:
			return
		}
	}
}

func (d *LocalDatastore) Stats() map[string]string {
	lsm, vlog := d.badger.Size()
	size := lsm + vlog

	cacheMetrics := d.badger.BlockCacheMetrics()

	maxVersion := strconv.FormatInt(int64(d.badger.MaxVersion()), 10)

	return map[string]string{
		"size":           units.HumanSize(float64(size)),
		"cache_hit_miss": fmt.Sprintf("%d/%d", cacheMetrics.Hits(), cacheMetrics.Misses()),
		"max_version":    maxVersion,
	}
}

func (d *LocalDatastore) NewTransaction(ctx context.Context, readOnly bool) (ds.Txn, error) {
	if !d.isRunning.Load() {
		return nil, ErrNotRunning
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return &txn{d, d.badger.NewTransaction(!readOnly), false}, nil
}

func (d *LocalDatastore) newImplicitTransaction(readOnly bool) *txn {
	return &txn{d, d.badger.NewTransaction(!readOnly), true}
}

func (d *LocalDatastore) Put(ctx context.Context, key DatastoreKey, value []byte) error {
	if !d.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	txn := d.newImplicitTransaction(false)
	defer txn.Discard(ctx)

	if err := txn.Put(ctx, key, value); err != nil {
		return err
	}

	return txn.Commit(ctx)
}

func (d *LocalDatastore) Sync(ctx context.Context, _ DatastoreKey) error {
	if !d.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return d.badger.Sync()
}

func (d *LocalDatastore) PutWithTTL(ctx context.Context, key DatastoreKey, value []byte, ttl time.Duration) error {
	if !d.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	txn := d.newImplicitTransaction(false)
	defer txn.Discard(ctx)

	if err := txn.PutWithTTL(ctx, key, value, ttl); err != nil {
		return err
	}

	return txn.Commit(ctx)
}

func (d *LocalDatastore) Get(ctx context.Context, key DatastoreKey) (value []byte, err error) {
	if !d.isRunning.Load() {
		return nil, ErrNotRunning
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	txn := d.newImplicitTransaction(true)
	defer txn.Discard(ctx)

	return txn.Get(ctx, key)
}

func (d *LocalDatastore) Has(ctx context.Context, key DatastoreKey) (bool, error) {
	if !d.isRunning.Load() {
		return false, ErrNotRunning
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	txn := d.newImplicitTransaction(true)
	defer txn.Discard(ctx)

	return txn.Has(ctx, key)
}

func (d *LocalDatastore) GetSize(ctx context.Context, key DatastoreKey) (size int, err error) {
	if !d.isRunning.Load() {
		return 0, ErrNotRunning
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	txn := d.newImplicitTransaction(true)
	defer txn.Discard(ctx)

	return txn.GetSize(ctx, key)
}

func (d *LocalDatastore) Delete(ctx context.Context, key DatastoreKey) error {
	if !d.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	txn := d.newImplicitTransaction(false)
	defer txn.Discard(ctx)

	err := txn.Delete(ctx, key)
	if err != nil {
		return err
	}

	return txn.Commit(ctx)
}

func (d *LocalDatastore) Query(ctx context.Context, q dsq.Query) (dsq.Results, error) {
	if !d.isRunning.Load() {
		return nil, ErrNotRunning
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	txn := d.newImplicitTransaction(true)
	// We cannot defer txn.Discard() here, as the txn must remain active while the iterator is open.
	// https://github.com/dgraph-io/badger/commit/b1ad1e93e483bbfef123793ceedc9a7e34b09f79
	// The closing logic in the query function takes care of discarding the implicit transaction.
	return txn.query(q)
}

func (d *LocalDatastore) DiskUsage(ctx context.Context) (uint64, error) {
	if !d.isRunning.Load() {
		return 0, ErrNotRunning
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	lsm, vlog := d.badger.Size()

	return uint64(lsm + vlog), nil
}

func (d *LocalDatastore) Batch(ctx context.Context) (Batch, error) {
	if !d.isRunning.Load() {
		return nil, ErrNotRunning
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	b := &batch{d, d.badger.NewWriteBatch()}
	// Ensure that incomplete transaction resources are cleaned up in case
	// batch is abandoned.
	runtime.SetFinalizer(b, func(b *batch) {
		_ = b.Cancel()
		log.Error("batch not committed or canceled")
	})

	return b, nil
}

func (d *LocalDatastore) NewCustomTransaction(ctx context.Context, readOnly bool) (CustomTransaction, error) {
	if !d.isRunning.Load() {
		return nil, ErrNotRunning
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	tx := d.badger.NewTransaction(!readOnly)
	return &txn{d, tx, false}, nil
}

func (d *LocalDatastore) NewDagStore(ctx context.Context) (*dagStore, error) {
	if !d.isRunning.Load() {
		return nil, ErrNotRunning
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return &dagStore{d}, nil
}

func (d *LocalDatastore) Close() error {
	if d == nil {
		return nil
	}
	log.Infoln("closing database...")
	if d.sequence != nil {
		_ = d.sequence.Release()
	}
	if d.badger == nil {
		return nil
	}
	if !d.isRunning.Load() {
		return nil
	}
	close(d.stopChan)

	_ = d.Sync(context.TODO(), DatastoreKey{})
	if err := d.badger.Close(); err != nil {
		log.Infof("database: close: %v", err)
		return err
	}
	d.isRunning.Store(false)
	return nil
}

func IsNotFoundError(err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, badger.ErrKeyNotFound):
		return true
	case errors.Is(err, ds.ErrNotFound):
		return true
	default:
		return false
	}
}

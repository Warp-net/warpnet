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

package local_store

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Warp-net/warpnet/security"
	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
	"github.com/docker/go-units"
	ds "github.com/ipfs/go-datastore"
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

const (
	discardRatio     = 0.5
	firstRunLockFile = "run.lock"
	sequenceKey      = "/SEQUENCE"

	defaultDiscardRatioGC = 0.5
	defaultIntervalGC     = time.Hour
	defaultSleepGC        = time.Second

	ErrNotRunning    = DBError("DB is not running")
	ErrWrongPassword = DBError("wrong username or password")

	PermanentTTL = math.MaxInt64
)

var DefaultIteratorOptions = badger.DefaultIteratorOptions

type (
	Item       = badger.Item
	Entry      = badger.Entry
	WriteBatch = badger.WriteBatch
	WarpDB     = badger.DB
	Txn        = badger.Txn

	DBError string
)

func (e DBError) Error() string {
	return string(e)
}

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

func (opt *Options) WithDiscardRatioGC(ratio float64) *Options {
	opt.discardRatioGC = ratio
	return opt
}

func (opt *Options) WithIntervalGC(interval time.Duration) *Options {
	opt.intervalGC = interval
	return opt
}

func (opt *Options) WithSleepGC(sleep time.Duration) *Options {
	opt.sleepGC = sleep
	return opt
}

func (opt *Options) WithInMemory(v bool) *Options {
	opt.isInMemory = v
	return opt
}

type DB struct {
	badger   *badger.DB
	sequence *badger.Sequence

	isRunning *atomic.Bool

	dbPath          string
	hasFirstRunFlag bool

	badgerOpts     badger.Options
	discardRatioGC float64
	intervalGC     time.Duration
	sleepGC        time.Duration

	stopChan chan struct{}
}

type WarpDBLogger interface {
	Errorf(string, ...any)
	Warningf(string, ...any)
	Infof(string, ...any)
	Debugf(string, ...any)
}

func New(
	dbPath string,
	o *Options,
) (*DB, error) {
	badgerOpts := badger.
		DefaultOptions(dbPath).
		WithSyncWrites(false).
		WithIndexCacheSize(256 << 20).
		WithCompression(options.ZSTD).
		WithNumCompactors(4).
		WithLoggingLevel(badger.ERROR).
		WithBlockCacheSize(512 << 20)
	if o.isInMemory {
		badgerOpts = badger.
			DefaultOptions("").
			WithDir("").
			WithValueDir("").
			WithInMemory(true).
			WithSyncWrites(false).
			WithIndexCacheSize(256 << 20).
			WithNumCompactors(2).
			WithLoggingLevel(badger.ERROR).
			WithBlockCacheSize(512 << 20)
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

	storage := &DB{
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

func (db *DB) IsFirstRun() bool {
	return !db.hasFirstRunFlag
}

func (db *DB) writeFirstRunFlag() {
	if db.badgerOpts.InMemory {
		return
	}
	path := filepath.Join(db.dbPath, firstRunLockFile)
	log.Infof("database: lock file created: %s", path)
	f, _ := os.Create(path) //#nosec
	if f != nil {
		_ = f.Close()
	}
	db.hasFirstRunFlag = true
}

func (db *DB) Run(username, password string) (err error) {
	if username == "" || password == "" {
		return DBError("database: username or password is empty")
	}
	hashSum := security.ConvertToSHA256([]byte(username + "@" + password))
	execOpts := db.badgerOpts.WithEncryptionKey(hashSum)

	db.badger, err = badger.Open(execOpts)
	if errors.Is(err, badger.ErrEncryptionKeyMismatch) {
		return ErrWrongPassword
	}
	if err != nil {
		return err
	}
	db.isRunning.Store(true)
	db.sequence, err = db.badger.GetSequence([]byte(sequenceKey), 100)
	if err != nil {
		return err
	}

	if !db.hasFirstRunFlag {
		db.writeFirstRunFlag()
	}

	if !db.badgerOpts.InMemory {
		go db.runEventualGC()
	}

	return nil
}

func (db *DB) runEventualGC() {
	if db.badgerOpts.InMemory {
		return
	}
	log.Infoln("database: garbage collection started")
	gcTicker := time.NewTicker(db.intervalGC)
	defer gcTicker.Stop()

	dirTicker := time.NewTicker(time.Second)
	defer dirTicker.Stop()

	_ = db.badger.RunValueLogGC(db.discardRatioGC)
	for {
		select {
		case <-dirTicker.C:
			isFound := findFirstRunFlag(db.dbPath)
			if !isFound {
				panic("database: folder was emptied")
			}
		case <-gcTicker.C:
			for {
				err := db.badger.RunValueLogGC(db.discardRatioGC)
				if errors.Is(err, badger.ErrNoRewrite) ||
					errors.Is(err, badger.ErrRejected) {
					break
				}
				time.Sleep(db.sleepGC)
			}
			log.Infoln("database: garbage collection complete")
		case <-db.stopChan:
			return
		}
	}
}

func (db *DB) Stats() map[string]string {
	lsm, vlog := db.badger.Size()
	size := lsm + vlog

	cacheMetrics := db.badger.BlockCacheMetrics()

	maxVersion := strconv.FormatUint(db.badger.MaxVersion(), 10)

	return map[string]string{
		"size":           units.HumanSize(float64(size)),
		"cache_hit_miss": fmt.Sprintf("%d/%d", cacheMetrics.Hits(), cacheMetrics.Misses()),
		"max_version":    maxVersion,
	}
}

func (db *DB) Set(key DatabaseKey, value []byte) error {
	if db == nil {
		return ErrNotRunning
	}
	if !db.isRunning.Load() {
		return ErrNotRunning
	}

	return db.badger.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry(key.Bytes(), value)
		return txn.SetEntry(e)
	})
}

func (db *DB) SetWithTTL(key DatabaseKey, value []byte, ttl time.Duration) error {
	if db == nil {
		return ErrNotRunning
	}
	if !db.isRunning.Load() {
		return ErrNotRunning
	}
	return db.badger.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry(key.Bytes(), value)
		e.WithTTL(ttl)
		return txn.SetEntry(e)
	})
}

func (db *DB) Get(key DatabaseKey) ([]byte, error) {
	if db == nil {
		return nil, ErrNotRunning
	}
	if !db.isRunning.Load() {
		return nil, ErrNotRunning
	}

	var result []byte
	err := db.badger.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key.Bytes())
		if err != nil {
			return err
		}

		val, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		result = append([]byte{}, val...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (db *DB) GetExpiration(key DatabaseKey) (uint64, error) {
	if db == nil {
		return 0, ErrNotRunning
	}
	if !db.isRunning.Load() {
		return 0, ErrNotRunning
	}

	var result uint64
	err := db.badger.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key.Bytes())
		if err != nil {
			return err
		}

		result = item.ExpiresAt()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return result, nil
}

func (db *DB) GetSize(key DatabaseKey) (int64, error) {
	if db == nil {
		return 0, ErrNotRunning
	}
	if !db.isRunning.Load() {
		return 0, ErrNotRunning
	}

	var result int64
	err := db.badger.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key.Bytes())
		if err != nil {
			return err
		}

		result = item.ValueSize()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return result, nil
}

func (db *DB) Delete(key DatabaseKey) error {
	if db == nil {
		return ErrNotRunning
	}
	if !db.isRunning.Load() {
		return ErrNotRunning
	}

	return db.badger.Update(func(txn *badger.Txn) error {
		if err := txn.Delete(key.Bytes()); err != nil {
			return err
		}
		return txn.Delete([]byte(key))
	})
}

type WarpTransactioner interface {
	Set(key DatabaseKey, value []byte) error
	Get(key DatabaseKey) ([]byte, error)
	GetExpiration(key DatabaseKey) (uint64, error)
	SetWithTTL(key DatabaseKey, value []byte, ttl time.Duration) error
	BatchSet(data []ListItem) (err error)
	Delete(key DatabaseKey) error
	Increment(key DatabaseKey) (uint64, error)
	Decrement(key DatabaseKey) (uint64, error)
	Commit() error
	Discard() error
	Rollback()
	IterateKeys(prefix DatabaseKey, handler IterKeysFunc) error
	ReverseIterateKeys(prefix DatabaseKey, handler IterKeysFunc) error
	List(prefix DatabaseKey, limit *uint64, cursor *string) ([]ListItem, string, error)
	ListKeys(prefix DatabaseKey, limit *uint64, cursor *string) ([]string, string, error)
	BatchGet(keys ...DatabaseKey) ([]ListItem, error)
}

type warpTxn struct {
	txn *badger.Txn
}

func (db *DB) NewTxn() (WarpTransactioner, error) {
	if db == nil {
		return nil, ErrNotRunning
	}
	if !db.isRunning.Load() {
		return nil, ErrNotRunning
	}
	wtx := &warpTxn{db.badger.NewTransaction(true)}
	runtime.SetFinalizer(wtx, func(tx *warpTxn) { tx.Rollback() })
	return wtx, nil
}

func (t *warpTxn) Set(key DatabaseKey, value []byte) error {
	err := t.txn.Set(key.Bytes(), value)
	if err != nil {
		return err
	}
	return nil
}

func (t *warpTxn) Get(key DatabaseKey) ([]byte, error) {
	var result []byte
	item, err := t.txn.Get(key.Bytes())
	if err != nil {
		return nil, err
	}

	val, err := item.ValueCopy(nil)
	if err != nil {
		return nil, err
	}
	result = append([]byte{}, val...)

	return result, nil
}

func (t *warpTxn) GetExpiration(key DatabaseKey) (uint64, error) {
	item, err := t.txn.Get(key.Bytes())
	if err != nil {
		return 0, err
	}

	return item.ExpiresAt(), nil
}

func (t *warpTxn) SetWithTTL(key DatabaseKey, value []byte, ttl time.Duration) error {
	e := badger.NewEntry(key.Bytes(), value)
	e.WithTTL(ttl)

	err := t.txn.SetEntry(e)
	if err != nil {
		return err
	}
	return nil
}

func (t *warpTxn) BatchSet(data []ListItem) (err error) {
	var (
		lastIndex int
		isTooBig  bool
	)
	for i, item := range data {
		key := item.Key
		value := item.Value
		err := t.txn.Set([]byte(key), value)
		if errors.Is(err, badger.ErrTxnTooBig) {
			isTooBig = true
			lastIndex = i
			_ = t.txn.Commit() // force commit in the middle of iteration
			break
		}
		if err != nil {
			return err
		}
	}
	if isTooBig {
		leftovers := data[lastIndex:]
		data = nil //nolint:wastedassign
		err = t.BatchSet(leftovers)
	}
	return err
}

func (t *warpTxn) Delete(key DatabaseKey) error {
	if err := t.txn.Delete(key.Bytes()); err != nil {
		return err
	}
	return t.txn.Delete([]byte(key))
}

const (
	incr int8 = 1 << iota
	decr
)

func (t *warpTxn) Increment(key DatabaseKey) (uint64, error) {
	return increment(t.txn, key.Bytes(), incr)
}

func (t *warpTxn) Decrement(key DatabaseKey) (uint64, error) {
	return increment(t.txn, key.Bytes(), decr)
}

func increment(txn *badger.Txn, key []byte, incType int8) (uint64, error) {
	var newValue uint64

	item, err := txn.Get(key)
	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return 0, err
	}
	if item == nil {
		item = &badger.Item{}
	}

	val, err := item.ValueCopy(nil)
	if err != nil {
		return 0, err
	}
	switch incType {
	case incr:
		newValue = decodeUint64(val) + 1
	case decr:
		decodedVal := decodeUint64(val)
		if decodedVal > 0 {
			newValue = decodedVal - 1
		}
	}

	return newValue, txn.Set(key, encodeUint64(newValue))
}

const endCursor = "end"

type IterKeysFunc func(key string) error

func (t *warpTxn) IterateKeys(prefix DatabaseKey, handler IterKeysFunc) error {
	if strings.Contains(prefix.String(), FixedKey) {
		return DBError("cannot iterate thru fixed key")
	}
	opts := badger.DefaultIteratorOptions
	it := t.txn.NewIterator(opts)
	defer it.Close()
	p := []byte(prefix)

	for it.Seek(p); it.ValidForPrefix(p); it.Next() {
		item := it.Item()
		key := string(item.KeyCopy(nil))
		if strings.Contains(key, FixedKey) {
			continue
		}
		err := handler(string(item.KeyCopy(nil)))
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *warpTxn) ReverseIterateKeys(prefix DatabaseKey, handler IterKeysFunc) error {
	if strings.Contains(prefix.String(), FixedKey) {
		return DBError("cannot iterate thru fixed key")
	}
	opts := badger.DefaultIteratorOptions
	opts.Reverse = true
	it := t.txn.NewIterator(opts)
	defer it.Close()
	p := []byte(prefix)

	for it.Seek(p); it.ValidForPrefix(p); it.Next() {
		item := it.Item()
		key := string(item.KeyCopy(nil))
		if strings.Contains(key, FixedKey) {
			continue
		}
		err := handler(key)
		if err != nil {
			return err
		}
	}
	return nil
}

type ListItem struct {
	Key   string
	Value []byte
}

func (t *warpTxn) List(prefix DatabaseKey, limit *uint64, cursor *string) ([]ListItem, string, error) {
	var startCursor DatabaseKey
	if cursor != nil && *cursor != "" {
		startCursor = DatabaseKey(*cursor)
	}
	if startCursor.String() == endCursor {
		return []ListItem{}, endCursor, nil
	}

	if limit == nil {
		defaultLimit := uint64(20)
		limit = &defaultLimit
	}

	items := make([]ListItem, 0, *limit) //
	cur, err := iterate(
		t.txn, prefix, startCursor, limit, true,
		func(key string, value []byte) error {
			items = append(items, ListItem{
				Key:   key,
				Value: value,
			})
			return nil
		},
	)
	return items, cur, err
}

func (t *warpTxn) ListKeys(prefix DatabaseKey, limit *uint64, cursor *string) ([]string, string, error) {
	var startCursor DatabaseKey
	if cursor != nil && *cursor != "" {
		startCursor = DatabaseKey(*cursor)
	}
	if startCursor.String() == endCursor {
		return []string{}, endCursor, nil
	}

	if limit == nil {
		defaultLimit := uint64(20)
		limit = &defaultLimit
	}

	items := make([]string, 0, *limit) //
	cur, err := iterate(
		t.txn, prefix, startCursor, limit, false,
		func(key string, _ []byte) error {
			items = append(items, key)
			return nil
		},
	)
	return items, cur, err
}

type iterKeysValuesFunc func(key string, val []byte) error

func iterate(
	txn *badger.Txn,
	prefix DatabaseKey,
	startCursor DatabaseKey,
	limit *uint64,
	includeValues bool,
	handler iterKeysValuesFunc,
) (cursor string, err error) {
	if strings.Contains(prefix.String(), FixedKey) {
		return "", DBError("cannot iterate thru fixed keys")
	}
	if startCursor.String() == endCursor {
		return endCursor, nil
	}
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = includeValues
	opts.PrefetchSize = 20

	it := txn.NewIterator(opts)
	defer it.Close()

	p := prefix.Bytes()
	if !startCursor.IsEmpty() {
		p = startCursor.Bytes()
	}

	var (
		lastKey DatabaseKey
		iterNum uint64
	)

	for it.Seek(p); it.ValidForPrefix(p); it.Next() {
		item := it.Item()
		key := string(item.Key())

		if strings.Contains(key, FixedKey) {
			continue
		}

		if iterNum >= *limit {
			lastKey = DatabaseKey(key)
			break
		}

		val, err := item.ValueCopy(nil)
		if err != nil {
			return "", err
		}
		if err := handler(key, val); err != nil {
			return "", err
		}
		iterNum++
	}

	if iterNum < *limit {
		lastKey = endCursor
	}
	return lastKey.DropId(), nil
}

func (t *warpTxn) BatchGet(keys ...DatabaseKey) ([]ListItem, error) {
	result := make([]ListItem, 0, len(keys))

	for _, key := range keys {
		item, err := t.txn.Get([]byte(key))
		if errors.Is(err, badger.ErrKeyNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		val, err := item.ValueCopy(nil)
		if err != nil {
			return nil, err
		}
		it := ListItem{
			Key:   key.String(),
			Value: val,
		}
		result = append(result, it)
	}

	return result, nil
}

func (t *warpTxn) Commit() error {
	return t.txn.Commit()
}

func (t *warpTxn) Discard() error {
	t.txn.Discard()
	return nil
}

func (t *warpTxn) Rollback() {
	t.txn.Discard()
}

// =====================================================

func (db *DB) NextSequence() (uint64, error) {
	if db == nil {
		return 0, ErrNotRunning
	}
	if !db.isRunning.Load() {
		return 0, ErrNotRunning
	}
	num, err := db.sequence.Next()
	if err != nil {
		return 0, err
	}
	if num != 0 {
		return num, nil
	}

	return db.sequence.Next()
}

func (db *DB) GC() {
	if db == nil {
		panic(ErrNotRunning)
	}
	if !db.isRunning.Load() {
		return
	}
	for {
		err := db.badger.RunValueLogGC(discardRatio)
		if errors.Is(err, badger.ErrNoRewrite) {
			return
		}
	}
}

func (db *DB) InnerDB() *badger.DB {
	if db == nil {
		panic(ErrNotRunning)
	}
	return db.badger
}

func (db *DB) Sync() error {
	if db == nil || db.badger == nil {
		return ErrNotRunning
	}
	if db.badgerOpts.InMemory {
		return nil
	}
	return db.badger.Sync()
}

func (db *DB) Path() string {
	if db == nil {
		return ""
	}
	return db.dbPath
}

func (db *DB) IsClosed() bool {
	if db == nil {
		return true
	}
	return !db.isRunning.Load()
}

func encodeUint64(n uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, n)
	return b
}

func decodeUint64(b []byte) uint64 {
	if len(b) == 0 {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}

func (db *DB) Close() {
	if db == nil {
		return
	}
	log.Infoln("closing database...")
	if db.sequence != nil {
		_ = db.sequence.Release()
	}
	if db.badger == nil {
		return
	}
	if !db.isRunning.Load() {
		return
	}
	close(db.stopChan)

	_ = db.Sync()
	if err := db.badger.Close(); err != nil {
		log.Infof("database: close: %v", err)
		return
	}
	db.isRunning.Store(false)
	db.badger = nil
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

func ToDatatoreErrNotFound(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, badger.ErrKeyNotFound):
		return ds.ErrNotFound
	case errors.Is(err, ds.ErrNotFound):
		return ds.ErrNotFound
	default:
		return err
	}
}

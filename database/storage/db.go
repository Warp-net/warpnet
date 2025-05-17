/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/security"
	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/badger/v3/options"
	"github.com/docker/go-units"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
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

const discardRatio = 0.5

var (
	ErrNotRunning    = errors.New("DB is not running")
	ErrKeyNotFound   = badger.ErrKeyNotFound
	ErrWrongPassword = errors.New("wrong username or password")
	ErrStopIteration = errors.New("stop iteration")
)

type (
	WarpDB = badger.DB
)

type DB struct {
	badger   *badger.DB
	sequence *badger.Sequence

	isRunning *atomic.Bool
	stopChan  chan struct{}

	storedOpts badger.Options
	dbPath     string
	isDirEmpty bool
}

type WarpDBLogger interface {
	Errorf(string, ...interface{})
	Warningf(string, ...interface{})
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
}

func New(
	path string,
	isInMemory bool,
	dataFolder string,
) (*DB, error) {
	dbPath := filepath.Join(path, dataFolder)
	opts := badger.
		DefaultOptions(dbPath).
		WithSyncWrites(false).
		WithIndexCacheSize(256 << 20).
		WithCompression(options.Snappy).
		WithNumCompactors(2).
		WithLoggingLevel(badger.ERROR).
		WithBlockCacheSize(512 << 20)
	if isInMemory {
		opts = opts.WithDir("")
		opts = opts.WithValueDir("")
		opts = opts.WithInMemory(true)
	}

	isDirEmpty, err := isDirectoryEmpty(dbPath)
	if err != nil {
		return nil, err
	}

	storage := &DB{
		badger: nil, stopChan: make(chan struct{}), isRunning: new(atomic.Bool),
		sequence: nil, storedOpts: opts, dbPath: dbPath, isDirEmpty: isDirEmpty,
	}

	return storage, nil
}

func isDirectoryEmpty(dirPath string) (bool, error) {
	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			err := os.Mkdir(dirPath, 0750)
			return true, err
		}
		return false, fmt.Errorf("is dir empty: %w", err)
	}

	return len(dirEntries) == 0, nil
}

func (db *DB) IsFirstRun() bool {
	return db.isDirEmpty
}

func (db *DB) Run(username, password string) (err error) {
	if username == "" || password == "" {
		return errors.New("database: username or password is empty")
	}
	hashSum := security.ConvertToSHA256([]byte(username + "@" + password))
	execOpts := db.storedOpts.WithEncryptionKey(hashSum)

	db.badger, err = badger.Open(execOpts)
	if errors.Is(err, badger.ErrEncryptionKeyMismatch) {
		return ErrWrongPassword
	}
	if err != nil {
		return err
	}
	db.isRunning.Store(true)
	db.sequence, err = db.badger.GetSequence([]byte("SEQUENCE"), 100)
	if err != nil {
		return err
	}

	if !db.storedOpts.InMemory {
		go db.runEventualGC()
	}

	return nil
}

func (db *DB) runEventualGC() {
	log.Infoln("database: garbage collection started")
	gcTicker := time.NewTicker(time.Hour * 8)
	defer gcTicker.Stop()

	dirTicker := time.NewTicker(time.Second)
	defer dirTicker.Stop()

	_ = db.badger.RunValueLogGC(discardRatio)
	for {
		select {
		case <-dirTicker.C:
			isEmpty, err := isDirectoryEmpty(db.dbPath)
			if isEmpty && err == nil {
				log.Errorln("database: folder was emptied")
				os.Exit(1)
			}
		case <-gcTicker.C:
			for {
				err := db.badger.RunValueLogGC(discardRatio)
				if errors.Is(err, badger.ErrNoRewrite) {
					break
				}
				time.Sleep(time.Second)
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

	maxVersion := strconv.FormatInt(int64(db.badger.MaxVersion()), 10)

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

type WarpTxn = badger.Txn

type WarpTxWriter interface {
	Set(key DatabaseKey, value []byte) error
	Get(key DatabaseKey) ([]byte, error)
	SetWithTTL(key DatabaseKey, value []byte, ttl time.Duration) error
	BatchSet(data []ListItem) (err error)
	Delete(key DatabaseKey) error
	Increment(key DatabaseKey) (uint64, error)
	Decrement(key DatabaseKey) (uint64, error)
	Commit() error
	Rollback()
}

type WarpWriteTxn struct {
	txn *badger.Txn
}

func (db *DB) NewWriteTxn() (WarpTxWriter, error) {
	if db == nil {
		return nil, ErrNotRunning
	}
	if !db.isRunning.Load() {
		return nil, ErrNotRunning
	}
	return &WarpWriteTxn{db.badger.NewTransaction(true)}, nil
}

func (t *WarpWriteTxn) Set(key DatabaseKey, value []byte) error {
	err := t.txn.Set(key.Bytes(), value)
	if err != nil {
		return err
	}
	return nil
}

func (t *WarpWriteTxn) Get(key DatabaseKey) ([]byte, error) {
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

func (t *WarpWriteTxn) SetWithTTL(key DatabaseKey, value []byte, ttl time.Duration) error {
	e := badger.NewEntry(key.Bytes(), value)
	e.WithTTL(ttl)

	err := t.txn.SetEntry(e)
	if err != nil {
		return err
	}
	return nil
}

func (t *WarpWriteTxn) BatchSet(data []ListItem) (err error) {
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
		data = nil
		err = t.BatchSet(leftovers)
	}
	return err
}

func (t *WarpWriteTxn) Delete(key DatabaseKey) error {
	if err := t.txn.Delete(key.Bytes()); err != nil {
		return err
	}
	return t.txn.Delete([]byte(key))
}

func (t *WarpWriteTxn) Increment(key DatabaseKey) (uint64, error) {
	return increment(t.txn, key.Bytes(), 1)
}

func (t *WarpWriteTxn) Decrement(key DatabaseKey) (uint64, error) {
	return increment(t.txn, key.Bytes(), -1)
}

func increment(txn *badger.Txn, key []byte, incVal int64) (uint64, error) {
	var newValue int64

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
	newValue = decodeInt64(val) + incVal
	if newValue < 0 {
		newValue = 0
	}

	return uint64(newValue), txn.Set(key, encodeInt64(newValue))
}

func (t *WarpWriteTxn) Commit() error {
	return t.txn.Commit()
}

func (t *WarpWriteTxn) Rollback() {
	t.txn.Discard()
}

// =========== READ ===============================

const endCursor = "end"

type WarpTxReader interface {
	IterateKeys(prefix DatabaseKey, handler IterKeysFunc) error
	Get(key DatabaseKey) ([]byte, error)
	ReverseIterateKeys(prefix DatabaseKey, handler IterKeysFunc) error
	List(prefix DatabaseKey, limit *uint64, cursor *string) ([]ListItem, string, error)
	BatchGet(keys ...DatabaseKey) ([]ListItem, error)
	Commit() error
	Rollback()
}

type WarpReadTxn struct {
	txn *badger.Txn
}

func (db *DB) NewReadTxn() (WarpTxReader, error) {
	if db == nil {
		return nil, ErrNotRunning
	}
	if !db.isRunning.Load() {
		return nil, ErrNotRunning
	}
	return &WarpReadTxn{db.badger.NewTransaction(false)}, nil
}

type IterKeysFunc func(key string) error

func (t *WarpReadTxn) IterateKeys(prefix DatabaseKey, handler IterKeysFunc) error {
	if strings.Contains(prefix.String(), FixedKey) {
		return errors.New("cannot iterate thru fixed key")
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

func (t *WarpReadTxn) ReverseIterateKeys(prefix DatabaseKey, handler IterKeysFunc) error {
	if strings.Contains(prefix.String(), FixedKey) {
		return errors.New("cannot iterate thru fixed key")
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

func (t *WarpReadTxn) List(prefix DatabaseKey, limit *uint64, cursor *string) ([]ListItem, string, error) {
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
	cur, err := iterateKeysValues(
		t.txn, prefix, startCursor, limit,
		func(key string, value []byte) error {
			items = append(items, ListItem{
				Key:   key,
				Value: value,
			})
			return nil
		})
	return items, cur, err
}

type iterKeysValuesFunc func(key string, val []byte) error

func iterateKeysValues(
	txn *badger.Txn,
	prefix DatabaseKey,
	startCursor DatabaseKey,
	limit *uint64,
	handler iterKeysValuesFunc,
) (cursor string, err error) {
	if strings.Contains(prefix.String(), FixedKey) {
		return "", errors.New("cannot iterate thru fixed keys")
	}
	if startCursor.String() == endCursor {
		return endCursor, nil
	}
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = true
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

func (t *WarpReadTxn) Get(key DatabaseKey) ([]byte, error) {
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

func (t *WarpReadTxn) BatchGet(keys ...DatabaseKey) ([]ListItem, error) {
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

func (t *WarpReadTxn) Commit() error {
	return t.txn.Commit()
}

func (t *WarpReadTxn) Rollback() {
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
	if db == nil {
		return ErrNotRunning
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

func encodeInt64(n int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(n))
	return b
}

func decodeInt64(b []byte) int64 {
	if len(b) == 0 {
		return 0
	}
	return int64(binary.BigEndian.Uint64(b))
}

func (db *DB) Close() {
	if db == nil {
		return
	}
	log.Infoln("closing database...")
	close(db.stopChan)
	if db.sequence != nil {
		_ = db.sequence.Release()
	}
	if db.badger == nil {
		return
	}
	if !db.isRunning.Load() {
		return
	}

	_ = db.Sync()
	if err := db.badger.Close(); err != nil {
		log.Infoln("database: close: ", err)
		return
	}
	db.isRunning.Store(false)
}

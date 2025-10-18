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

package local

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	localDB "github.com/Warp-net/warpnet/database/local/db"
	"github.com/dgraph-io/badger/v4"
	crdt "github.com/ipfs/go-ds-crdt"
	format "github.com/ipfs/go-ipld-format"
	log "github.com/sirupsen/logrus"
)

type DBError = localDB.DBError

var ErrKeyNotFound = localDB.ErrKeyNotFound

type DB struct {
	localstore *localDB.DistributedDatastore
	crdtStore  *crdt.Datastore
}

type Options = localDB.Options

func New(dbPath string) (*DB, error) {
	store, err := localDB.NewDistributedDatastore(dbPath, localDB.DefaultOptions())
	return &DB{localstore: store}, err
}

type (
	Syncer      = format.DAGService
	Broadcaster = crdt.Broadcaster
)

func (db *DB) EnableCRDT(ns string, s Syncer, b Broadcaster) (err error) {
	opts := crdt.DefaultOptions()
	opts.Logger = log.StandardLogger()
	opts.RebroadcastInterval = 5 * time.Second
	opts.PutHook = func(k localDB.DatastoreKey, v []byte) {
		fmt.Printf("Added: [%s] -> %s\n", k, string(v))

	}
	opts.DeleteHook = func(k localDB.DatastoreKey) {
		fmt.Printf("Removed: [%s]\n", k)
	}

	db.crdtStore, err = crdt.New(db.localstore, localDB.NewKey(ns), s, b, opts)
	return
}

func (db *DB) IsFirstRun() bool {
	return db.localstore.IsFirstRun()
}

func (db *DB) Run(username, password string) (err error) {
	return db.localstore.Run(username, password)
}

func (db *DB) Stats() map[string]string {
	storeStats := db.localstore.Stats()
	crdtStats := db.crdtStore.InternalStats(context.TODO())

	storeStats["crdt"] = fmt.Sprintf("%#v", crdtStats)
	return storeStats
}

func (db *DB) Set(key DatabaseKey, value []byte) error {
	if db.crdtStore == nil {
		return db.localstore.Put(context.TODO(), localDB.NewKey(key.String()), value)
	}
	return db.crdtStore.Put(context.TODO(), localDB.NewKey(key.String()), value)
}

func (db *DB) SetWithTTL(key DatabaseKey, value []byte, ttl time.Duration) error {
	return db.localstore.PutWithTTL(context.TODO(), localDB.NewKey(key.String()), value, ttl)
}

func (db *DB) Get(key DatabaseKey) ([]byte, error) {
	if db.crdtStore == nil {
		return db.localstore.Get(context.TODO(), localDB.NewKey(key.String()))
	}
	return db.crdtStore.Get(context.TODO(), localDB.NewKey(key.String()))
}

func (db *DB) Delete(key DatabaseKey) error {
	if db.crdtStore == nil {
		return db.localstore.Delete(context.TODO(), localDB.NewKey(key.String()))
	}
	return db.crdtStore.Delete(context.TODO(), localDB.NewKey(key.String()))
}

func (db *DB) Sync() error {
	if db.crdtStore == nil {
		return db.localstore.Sync(context.TODO(), localDB.DatastoreKey{})
	}
	return db.crdtStore.Sync(context.TODO(), localDB.DatastoreKey{})
}

func (db *DB) GetExpiration(key DatabaseKey) (uint64, error) {
	exp, err := db.localstore.GetExpiration(context.TODO(), localDB.NewKey(key.String()))
	if err != nil {
		return 0, err
	}
	return uint64(exp.UnixMilli()), nil
}

func (db *DB) GetSize(key DatabaseKey) (int64, error) {
	if db.crdtStore == nil {
		size, err := db.localstore.GetSize(context.TODO(), localDB.NewKey(key.String()))
		return int64(size), err
	}
	size, err := db.crdtStore.GetSize(context.TODO(), localDB.NewKey(key.String()))
	return int64(size), err
}

func (db *DB) BatchSet(data []ListItem) (err error) {
	ctx := context.TODO()

	var b localDB.Batch

	if db.crdtStore != nil {
		b, err = db.crdtStore.Batch(ctx)
		if err != nil {
			return err
		}
	} else {
		b, err = db.localstore.Batch(ctx)
		if err != nil {
			return err
		}
	}

	for _, item := range data {
		key := item.Key
		value := item.Value
		if err := b.Put(ctx, localDB.NewKey(key), value); err != nil {
			return err
		}
	}
	return b.Commit(ctx)
}

func (db *DB) BatchGet(keys ...DatabaseKey) ([]ListItem, error) {
	tx, err := db.NewTxn()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	result := make([]ListItem, 0, len(keys))

	for _, key := range keys {
		val, err := tx.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		it := ListItem{
			Key:   key.String(),
			Value: val,
		}
		result = append(result, it)
	}

	return result, tx.Commit()
}

func (db *DB) Close() {
	if err := db.localstore.Close(); err != nil {
		log.Error("db close:", err)
	}
	if db.crdtStore != nil {
		if err := db.crdtStore.Close(); err != nil {
			log.Error("crdt close:", err)
		}
	}
}

type WarpTransactioner interface {
	Set(key DatabaseKey, value []byte) error
	Get(key DatabaseKey) ([]byte, error)
	Rollback()
	Commit() error
	Delete(key DatabaseKey) error
	List(prefix DatabaseKey, limit *uint64, cursor *string) ([]ListItem, string, error)
	Increment(key DatabaseKey) (uint64, error)
	Decrement(key DatabaseKey) (uint64, error)
	SetWithTTL(key DatabaseKey, value []byte, ttl time.Duration) error
}

type WarpTxn struct {
	txn   localDB.CustomTransaction
	batch localDB.Batch
}

func (db *DB) NewTxn() (WarpTransactioner, error) {
	ctx := context.TODO()
	tx, err := db.localstore.NewCustomTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	b, err := db.crdtStore.Batch(ctx)
	return &WarpTxn{txn: tx, batch: b}, err
}

func (t *WarpTxn) Increment(key DatabaseKey) (uint64, error) {
	num, err := t.txn.Increment(localDB.NewKey(key.String()))
	if err != nil {
		return 0, err
	}
	err = t.batch.Put(context.TODO(), localDB.NewKey(key.String()), localDB.EncodeInt64(int64(num)))
	if err != nil {
		log.Error("crdt: tx increment", "key", key, "value", num, "msg", err)
	}

	return num, nil
}

func (t *WarpTxn) Decrement(key DatabaseKey) (uint64, error) {
	num, err := t.txn.Decrement(localDB.NewKey(key.String()))
	if err != nil {
		return 0, err
	}
	err = t.batch.Put(context.TODO(), localDB.NewKey(key.String()), localDB.EncodeInt64(int64(num)))
	if err != nil {
		log.Error("crdt: tx decrement", "key", key, "value", num, "msg", err)
	}

	return num, nil
}

func (t *WarpTxn) Set(key DatabaseKey, value []byte) error {
	err := t.txn.Put(context.TODO(), localDB.NewKey(key.String()), value)
	if err != nil {
		return err
	}
	err = t.batch.Put(context.TODO(), localDB.NewKey(key.String()), value)
	if err != nil {
		log.Error("crdt: tx set", "key", key, "msg", err)
	}
	return nil
}

func (t *WarpTxn) Get(key DatabaseKey) ([]byte, error) {
	return t.txn.Get(context.TODO(), localDB.NewKey(key.String()))
}

func (t *WarpTxn) SetWithTTL(key DatabaseKey, value []byte, ttl time.Duration) error {
	return t.txn.PutWithTTL(context.TODO(), localDB.NewKey(key.String()), value, ttl)
}

func (t *WarpTxn) Discard(ctx context.Context) {
	t.txn.Discard(ctx)
	t.batch = nil
}

func (t *WarpTxn) Commit() error {
	err := t.txn.Commit(context.TODO())
	if err != nil {
		return err
	}
	if t.batch != nil {
		if err := t.batch.Commit(context.Background()); err != nil {
			log.Error("crdt: tx commit", "msg", err)
		}
	}
	return nil
}

// Rollback is back compatible with Badger V3
func (t *WarpTxn) Rollback() {
	t.txn.Discard(context.TODO())
	t.batch = nil
}

func (t *WarpTxn) Delete(key DatabaseKey) error {
	err := t.txn.Delete(context.TODO(), localDB.NewKey(key.String()))
	if err != nil {
		return err
	}

	if err := t.batch.Delete(context.TODO(), localDB.NewKey(key.String())); err != nil {
		log.Error("crdt: tx delete", "key", key, "msg", err)
	}
	return nil
}

const endCursor = "end"

type IterKeysFunc func(key string) error

func (t *WarpTxn) IterateKeys(prefix DatabaseKey, handler IterKeysFunc) error {
	if strings.Contains(prefix.String(), FixedKey) {
		return DBError("cannot iterate thru fixed key")
	}
	opts := badger.DefaultIteratorOptions
	it, err := t.txn.NewIterator(opts)
	if err != nil {
		return err
	}
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

func (t *WarpTxn) ReverseIterateKeys(prefix DatabaseKey, handler IterKeysFunc) error {
	if strings.Contains(prefix.String(), FixedKey) {
		return DBError("cannot iterate thru fixed key")
	}
	opts := badger.DefaultIteratorOptions
	opts.Reverse = true
	it, err := t.txn.NewIterator(opts)
	if err != nil {
		return err
	}
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

func (t *WarpTxn) List(prefix DatabaseKey, limit *uint64, cursor *string) ([]ListItem, string, error) {
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
	txn localDB.CustomTransaction,
	prefix DatabaseKey,
	startCursor DatabaseKey,
	limit *uint64,
	handler iterKeysValuesFunc,
) (cursor string, err error) {
	if strings.Contains(prefix.String(), FixedKey) {
		return "", DBError("cannot iterate thru fixed keys")
	}
	if startCursor.String() == endCursor {
		return endCursor, nil
	}
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = true
	opts.PrefetchSize = 20

	it, err := txn.NewIterator(opts)
	if err != nil {
		return "", err
	}
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

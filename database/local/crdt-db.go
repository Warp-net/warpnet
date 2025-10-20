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
	"sync/atomic"
	"time"

	localDB "github.com/Warp-net/warpnet/database/local/db"
	crdt "github.com/ipfs/go-ds-crdt"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/protocol"
	log "github.com/sirupsen/logrus"
)

type DBError = localDB.DBError

const ErrKeyNotFound = DBError("warp db: key not found")

type DB struct {
	ctx        context.Context
	localstore *localDB.LocalDatastore

	isCRDTEnabled *atomic.Bool
	crdtStore     *crdt.Datastore
}

func New(dbPath string) (*DB, error) {
	store, err := localDB.NewLocalDatastore(dbPath, localDB.DefaultOptions())
	return &DB{ctx: context.Background(), localstore: store, isCRDTEnabled: new(atomic.Bool)}, err
}

func (db *DB) Run(username, password string) (err error) {
	return db.localstore.Run(username, password)
}

type Broadcaster = crdt.Broadcaster

type PubSubProvider interface {
	SubscribeUserUpdate(userId string) (err error)
	UnsubscribeUserUpdate(userId string) (err error)
	PublishUpdateToFollowers(ownerId, dest string, bt []byte) (err error)
	Close() error
}

const (
	crdtProtocolV1_0_0 = protocol.ID("/crdt/1.0.0")
)

func (db *DB) EnableCRDT(ns string, h host.Host) (err error) {
	if db.isCRDTEnabled.Load() {
		return nil
	}
	opts := crdt.DefaultOptions()
	opts.Logger = log.StandardLogger()
	opts.RebroadcastInterval = 55 * time.Second
	opts.PutHook = func(k localDB.DatastoreKey, v []byte) {
		fmt.Printf("Added: [%s] -> %s\n", k, string(v))
	}

	dag, err := db.localstore.NewDagStore(db.ctx)
	if err != nil {
		return err
	}

	protos := []protocol.ID{crdtProtocolV1_0_0}
	router := pubsub.DefaultGossipSubRouter(h)
	// TODO implement own router
	ps, err := pubsub.NewGossipSubWithRouter(
		db.ctx,
		h,
		router,
		pubsub.WithGossipSubProtocols(protos, nil),
		pubsub.WithMessageAuthor(h.ID()),
	)
	if err != nil {
		return err
	}

	crdtTopic := fmt.Sprintf("crdt-%s", ns)
	broadcaster, err := crdt.NewPubSubBroadcaster(db.ctx, ps, crdtTopic)
	if err != nil {
		return err
	}

	db.crdtStore, err = crdt.New(db.localstore, localDB.NewKey(ns), dag, broadcaster, opts)
	if err != nil {
		return err
	}

	db.isCRDTEnabled.Store(true)
	return nil
}

func (db *DB) IsFirstRun() bool {
	return db.localstore.IsFirstRun()
}

type LocalStore interface {
	localDB.Datastore
	localDB.Batching
}

func (db *DB) LocalStore() LocalStore {
	return db.localstore
}

func (db *DB) Stats() map[string]string {
	ctx, cancelF := context.WithTimeout(db.ctx, 5*time.Second)
	defer cancelF()

	storeStats := db.localstore.Stats()
	if db.isCRDTEnabled.Load() {
		storeStats["crdt"] = fmt.Sprintf("%#v", db.crdtStore.InternalStats(ctx))
	}
	return storeStats
}

func (db *DB) Set(key DatabaseKey, value []byte) (err error) {
	ctx, cancelF := context.WithTimeout(db.ctx, 5*time.Second)
	defer cancelF()

	localKey := localDB.NewKey(key.String())
	if db.isCRDTEnabled.Load() {
		err = db.crdtStore.Put(ctx, localKey, value)
	} else {
		err = db.localstore.Put(ctx, localKey, value)
	}
	return err
}

func (db *DB) SetWithTTL(key DatabaseKey, value []byte, ttl time.Duration) error {
	ctx, cancelF := context.WithTimeout(db.ctx, 5*time.Second)
	defer cancelF()

	localKey := localDB.NewKey(key.String())
	err := db.localstore.PutWithTTL(ctx, localKey, value, ttl)
	if err != nil {
		return err
	}
	if ttl != localDB.PermanentTTL {
		return nil
	}
	err = db.crdtStore.Put(ctx, localKey, value)
	if err != nil {
		log.Error("crdt: tx set with ttl", "key", key, "msg", err)
	}
	return nil
}

func (db *DB) Get(key DatabaseKey) (data []byte, err error) {
	ctx, cancelF := context.WithTimeout(db.ctx, 5*time.Second)
	defer cancelF()

	localKey := localDB.NewKey(key.String())
	if db.isCRDTEnabled.Load() {
		data, err = db.crdtStore.Get(ctx, localKey)
	} else {
		data, err = db.localstore.Get(ctx, localKey)
	}
	if localDB.IsNotFoundError(err) {
		return nil, ErrKeyNotFound
	}
	return data, err
}

func (db *DB) Delete(key DatabaseKey) (err error) {
	ctx, cancelF := context.WithTimeout(db.ctx, 5*time.Second)
	defer cancelF()

	localKey := localDB.NewKey(key.String())

	if db.isCRDTEnabled.Load() {
		err = db.crdtStore.Delete(ctx, localKey)
	} else {
		err = db.localstore.Delete(ctx, localKey)
	}
	return err
}

func (db *DB) Sync() (err error) {
	ctx, cancelF := context.WithTimeout(db.ctx, 5*time.Second)
	defer cancelF()

	if !db.isCRDTEnabled.Load() {
		return
	}

	blankPrefix := localDB.DatastoreKey{}
	if db.isCRDTEnabled.Load() {
		err = db.crdtStore.Sync(ctx, blankPrefix)
	} else {
		err = db.localstore.Sync(ctx, blankPrefix)
	}
	return err
}

func (db *DB) GetSize(key DatabaseKey) (_ int64, err error) {
	ctx, cancelF := context.WithTimeout(db.ctx, 5*time.Second)
	defer cancelF()

	localKey := localDB.NewKey(key.String())

	var size int
	if db.isCRDTEnabled.Load() {
		size, err = db.crdtStore.GetSize(ctx, localKey)
	} else {
		size, err = db.localstore.GetSize(ctx, localKey)
	}
	if localDB.IsNotFoundError(err) {
		return -1, ErrKeyNotFound
	}
	return int64(size), err
}

func (db *DB) BatchSet(data []ListItem) (err error) {
	ctx, cancelF := context.WithTimeout(db.ctx, 5*time.Second)
	defer cancelF()

	var b localDB.Batch
	if db.isCRDTEnabled.Load() {
		b, err = db.crdtStore.Batch(ctx)
	} else {
		b, err = db.localstore.Batch(ctx)
	}
	if err != nil {
		return err
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
		if localDB.IsNotFoundError(err) {
			continue
		}
		if errors.Is(err, ErrKeyNotFound) {
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
	ctx           context.Context
	txn           localDB.CustomTransaction
	isCRDTEnabled bool
	batch         localDB.Batch
}

func (db *DB) NewTxn() (WarpTransactioner, error) {
	tx, err := db.localstore.NewCustomTransaction(db.ctx, false)
	if err != nil {
		return nil, err
	}

	warpTx := &WarpTxn{ctx: db.ctx, txn: tx, isCRDTEnabled: db.isCRDTEnabled.Load()}

	if db.isCRDTEnabled.Load() {
		b, err := db.crdtStore.Batch(db.ctx)
		if err != nil {
			return nil, err
		}
		warpTx.batch = b
	}

	return warpTx, nil
}

func (t *WarpTxn) Increment(key DatabaseKey) (uint64, error) {
	ctx, cancelF := context.WithTimeout(t.ctx, 5*time.Second)
	defer cancelF()

	localKey := localDB.NewKey(key.String())
	num, err := t.txn.Increment(localKey)
	if err != nil {
		return 0, err
	}
	if !t.isCRDTEnabled {
		return num, nil
	}

	err = t.batch.Put(ctx, localKey, localDB.EncodeInt64(int64(num)))
	if err != nil {
		log.Error("crdt: tx increment", "key", key, "value", num, "msg", err)
	}

	return num, nil
}

func (t *WarpTxn) Decrement(key DatabaseKey) (uint64, error) {
	ctx, cancelF := context.WithTimeout(t.ctx, 5*time.Second)
	defer cancelF()

	localKey := localDB.NewKey(key.String())
	num, err := t.txn.Decrement(localKey)
	if err != nil {
		return 0, err
	}
	if !t.isCRDTEnabled {
		return num, nil
	}
	err = t.batch.Put(ctx, localKey, localDB.EncodeInt64(int64(num)))
	if err != nil {
		log.Error("crdt: tx decrement", "key", key, "value", num, "msg", err)
	}

	return num, nil
}

func (t *WarpTxn) Set(key DatabaseKey, value []byte) error {
	ctx, cancelF := context.WithTimeout(t.ctx, 5*time.Second)
	defer cancelF()

	localKey := localDB.NewKey(key.String())
	err := t.txn.Put(ctx, localKey, value)
	if err != nil {
		return err
	}
	if !t.isCRDTEnabled {
		return nil
	}
	err = t.batch.Put(ctx, localKey, value)
	if err != nil {
		log.Error("crdt: tx put", "key", key, "msg", err)
	}
	return nil
}

func (t *WarpTxn) Get(key DatabaseKey) ([]byte, error) {
	ctx, cancelF := context.WithTimeout(t.ctx, 5*time.Second)
	defer cancelF()

	data, err := t.txn.Get(ctx, localDB.NewKey(key.String()))
	if localDB.IsNotFoundError(err) {
		return nil, ErrKeyNotFound
	}
	return data, err
}

func (t *WarpTxn) SetWithTTL(key DatabaseKey, value []byte, ttl time.Duration) error {
	ctx, cancelF := context.WithTimeout(t.ctx, 5*time.Second)
	defer cancelF()

	localKey := localDB.NewKey(key.String())
	err := t.txn.PutWithTTL(ctx, localKey, value, ttl)
	if err != nil {
		return err
	}
	if !t.isCRDTEnabled {
		return nil
	}
	if ttl != localDB.PermanentTTL {
		return nil
	}
	err = t.batch.Put(ctx, localKey, value)
	if err != nil {
		log.Error("crdt: tx set with ttl", "key", key, "msg", err)
	}
	return nil

}

func (t *WarpTxn) Commit() error {
	ctx, cancelF := context.WithTimeout(t.ctx, 5*time.Second)
	defer cancelF()

	err := t.txn.Commit(ctx)
	if err != nil {
		return err
	}
	if !t.isCRDTEnabled {
		return nil
	}
	if err := t.batch.Commit(ctx); err != nil {
		log.Error("crdt: tx commit", "msg", err)
	}

	return nil
}

func (t *WarpTxn) Delete(key DatabaseKey) error {
	ctx, cancelF := context.WithTimeout(t.ctx, 5*time.Second)
	defer cancelF()

	localKey := localDB.NewKey(key.String())
	err := t.txn.Delete(ctx, localKey)
	if err != nil {
		return err
	}
	if !t.isCRDTEnabled {
		return nil
	}
	if err := t.batch.Delete(ctx, localKey); err != nil {
		log.Error("crdt: tx delete", "key", key, "msg", err)
	}
	return nil
}

// Rollback is back compatible with Badger V3
func (t *WarpTxn) Rollback() {
	ctx, cancelF := context.WithTimeout(t.ctx, 5*time.Second)
	defer cancelF()

	t.txn.Discard(ctx)
	t.batch = nil
}

const endCursor = "end"

type IterKeysFunc func(key string) error

func (t *WarpTxn) IterateKeys(prefix DatabaseKey, handler IterKeysFunc) error {
	if strings.Contains(prefix.String(), FixedKey) {
		return DBError("cannot iterate thru fixed key")
	}
	opts := localDB.DefaultIteratorOptions
	it, err := t.txn.NewIterator(opts)
	if err != nil {
		return err
	}
	defer it.Close()

	p := []byte(prefix)

	for it.Seek(p); it.ValidForPrefix(p); it.Next() {
		if t.ctx.Err() != nil {
			return t.ctx.Err()
		}
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
	opts := localDB.DefaultIteratorOptions
	opts.Reverse = true
	it, err := t.txn.NewIterator(opts)
	if err != nil {
		return err
	}
	defer it.Close()

	p := []byte(prefix)

	for it.Seek(p); it.ValidForPrefix(p); it.Next() {
		if t.ctx.Err() != nil {
			return t.ctx.Err()
		}
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

	items := make([]ListItem, 0, *limit)
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
	opts := localDB.DefaultIteratorOptions
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

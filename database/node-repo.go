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

package database

import (
	"context"
	"errors"
	"fmt"
	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database/local"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/dgraph-io/badger/v3"
	log "github.com/sirupsen/logrus"
	"math"
	"runtime"
	"strings"
	"sync"
	"time"

	ds "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
)

// slash is required because of: invalid datastore key: NODES:/peers/keys/AASAQAISEAXNRKHMX2O3AA26JM7NGIWUPOGIITJ2UHHXGX4OWIEKPNAW6YCSK/priv
const (
	NodesNamespace        = "/NODES"
	ProvidersSubNamespace = "PROVIDERS"
	BlocklistSubNamespace = "BLOCKLIST"
	SelfHashSubNamespace  = "SELFHASH"
	InfoSubNamespace      = "INFO"
)

var (
	_              ds.Batching = (*NodeRepo)(nil)
	ErrNilNodeRepo             = local.DBError("node repo is nil")
)

type NodeStorer interface {
	NewTxn() (local.WarpTransactioner, error)
	Get(key local.DatabaseKey) ([]byte, error)
	GetExpiration(key local.DatabaseKey) (uint64, error)
	GetSize(key local.DatabaseKey) (int64, error)
	Sync() error
	IsClosed() bool
	InnerDB() *local.WarpDB
	SetWithTTL(key local.DatabaseKey, value []byte, ttl time.Duration) error
	Set(key local.DatabaseKey, value []byte) error
	Delete(key local.DatabaseKey) error
}

type NodeRepo struct {
	db       NodeStorer
	stopChan chan struct{}

	BootstrapSelfHashHex string
}

// Implements the datastore.Batch interface, enabling batching support for
// the badger Datastore.
type batch struct {
	ds         *NodeRepo
	writeBatch *badger.WriteBatch
}

func NewNodeRepo(db NodeStorer, version *semver.Version) (*NodeRepo, error) {
	nr := &NodeRepo{
		db:       db,
		stopChan: make(chan struct{}),
	}

	return nr, nr.PruneOldSelfHashes(version)
}

func (d *NodeRepo) Put(ctx context.Context, key ds.Key, value []byte) error {
	if d == nil {
		return ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	rootKey := buildRootKey(key)

	prefix := local.NewPrefixBuilder(NodesNamespace).
		AddRootID(rootKey).
		Build()
	return d.db.Set(prefix, value)
}

func (d *NodeRepo) Sync(ctx context.Context, _ ds.Key) error {
	if d == nil {
		return ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return d.db.Sync()
}

func (d *NodeRepo) PutWithTTL(ctx context.Context, key ds.Key, value []byte, ttl time.Duration) error {
	if d == nil {
		return ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	rootKey := buildRootKey(key)

	prefix := local.NewPrefixBuilder(NodesNamespace).
		AddRootID(rootKey).
		Build()

	return d.db.SetWithTTL(prefix, value, ttl)

}

func (d *NodeRepo) SetTTL(ctx context.Context, key ds.Key, ttl time.Duration) error {
	if d == nil {
		return ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if d.db.IsClosed() {
		return local.ErrNotRunning
	}

	item, err := d.Get(ctx, key)
	if err != nil {
		return err
	}
	return d.PutWithTTL(ctx, key, item, ttl)
}

func (d *NodeRepo) GetExpiration(ctx context.Context, key ds.Key) (t time.Time, err error) {
	if d == nil {
		return t, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return t, ctx.Err()
	}

	if d.db.IsClosed() {
		return t, local.ErrNotRunning
	}

	expiration := time.Time{}

	rootKey := buildRootKey(key)

	prefix := local.NewPrefixBuilder(NodesNamespace).
		AddRootID(rootKey).
		Build()

	expiresAt, err := d.db.GetExpiration(prefix)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return t, ds.ErrNotFound
	} else if err != nil {
		return t, err
	}

	if expiresAt > math.MaxInt64 {
		expiresAt = math.MaxInt64
	}
	expiration = time.Unix(int64(expiresAt), 0) //#nosec

	return expiration, err
}

func (d *NodeRepo) Get(ctx context.Context, key ds.Key) (value []byte, err error) {
	if d == nil {
		return nil, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if d.db.IsClosed() {
		return nil, local.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	prefix := local.NewPrefixBuilder(NodesNamespace).
		AddRootID(rootKey).
		Build()

	value, err = d.db.Get(prefix)
	if errors.Is(err, badger.ErrKeyNotFound) {
		err = ds.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return value, err
}

func (d *NodeRepo) Has(ctx context.Context, key ds.Key) (_ bool, err error) {
	if d == nil {
		return false, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	if d.db.IsClosed() {
		return false, local.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	prefix := local.NewPrefixBuilder(NodesNamespace).
		AddRootID(rootKey).
		Build()

	_, err = d.db.Get(prefix)
	switch {
	case errors.Is(err, badger.ErrKeyNotFound) || errors.Is(err, ds.ErrNotFound):
		return false, nil
	case err == nil:
		return true, nil
	default:
		return false, fmt.Errorf("has: %w", err)
	}
}

func (d *NodeRepo) GetSize(ctx context.Context, key ds.Key) (_ int, err error) {
	if d == nil {
		return -1, ErrNilNodeRepo
	}
	size := -1

	if ctx.Err() != nil {
		return size, ctx.Err()
	}

	if d.db.IsClosed() {
		return size, local.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	prefix := local.NewPrefixBuilder(NodesNamespace).
		AddRootID(rootKey).
		Build()

	itemSize, err := d.db.GetSize(prefix)
	switch {
	case err == nil:
		return int(itemSize), nil
	case errors.Is(err, badger.ErrKeyNotFound):
		return 0, ds.ErrNotFound
	default:
		return 0, fmt.Errorf("size: %w", err)
	}
}

func (d *NodeRepo) Delete(ctx context.Context, key ds.Key) error {
	if d == nil {
		return ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if d.db.IsClosed() {
		return local.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	prefix := local.NewPrefixBuilder(NodesNamespace).
		AddRootID(rootKey).
		Build()

	return d.db.Delete(prefix)
}

// DiskUsage implements the PersistentDatastore interface.
// It returns the sum of lsm and value log files sizes in bytes.
func (d *NodeRepo) DiskUsage(ctx context.Context) (uint64, error) {
	if d == nil {
		return 0, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	if d.db.IsClosed() {
		return 0, local.ErrNotRunning
	}
	lsm, vlog := d.db.InnerDB().Size()
	if (lsm + vlog) < 0 {
		return 0, local.DBError("disk usage: malformed value")
	}
	return uint64(lsm + vlog), nil //#nosec
}

type myResults struct {
	query  dsq.Query
	output chan dsq.Result
	done   chan struct{}
	once   sync.Once
}

func (r *myResults) Query() dsq.Query {
	return r.query
}

func (r *myResults) Next() <-chan dsq.Result {
	return r.output
}

func (r *myResults) Done() <-chan struct{} {
	return r.done
}

func (r *myResults) Close() error {
	r.once.Do(func() {
		close(r.done)
	})
	return nil
}

func (r *myResults) NextSync() (dsq.Result, bool) {
	select {
	case res, ok := <-r.output:
		return res, ok
	case <-r.done:
		return dsq.Result{}, false
	}
}

func (r *myResults) Rest() ([]dsq.Entry, error) {
	var entries []dsq.Entry
	for {
		select {
		case res, ok := <-r.output:
			if !ok {
				return entries, nil
			}
			if res.Error != nil {
				return nil, res.Error
			}
			entries = append(entries, res.Entry)
		case <-r.done:
			return entries, nil
		}
	}
}

func (d *NodeRepo) Query(ctx context.Context, q dsq.Query) (res dsq.Results, err error) {
	if d == nil {
		return nil, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if d.db.IsClosed() {
		return nil, local.ErrNotRunning
	}

	// We cannot defer txn.Discard() here, as the txn must remain active while the iterator is open.
	// https://github.com/dgraph-io/badger/commit/b1ad1e93e483bbfef123793ceedc9a7e34b09f79
	// The closing logic in the query goprocess takes care of discarding the implicit transaction.
	tx := d.db.InnerDB().NewTransaction(true)
	return d.query(tx, q, true)
}

func (d *NodeRepo) query(tx *local.Txn, q dsq.Query, implicit bool) (dsq.Results, error) {
	opt := badger.DefaultIteratorOptions
	opt.PrefetchValues = !q.KeysOnly

	key := strings.TrimPrefix(ds.NewKey(q.Prefix).String(), "/")
	prefix := local.NewPrefixBuilder(NodesNamespace).AddRootID(key).Build().Bytes()
	opt.Prefix = prefix

	if len(q.Orders) > 0 {
		switch q.Orders[0].(type) {
		case dsq.OrderByKey, *dsq.OrderByKey:
		case dsq.OrderByKeyDescending, *dsq.OrderByKeyDescending:
			opt.Reverse = true
		default:
			base := q
			base.Limit = 0
			base.Offset = 0
			base.Orders = nil

			baseResults, err := d.query(tx, base, implicit)
			if err != nil {
				return nil, err
			}
			return dsq.NaiveQueryApply(q, baseResults), nil
		}
	}

	it := tx.NewIterator(opt)
	output := make(chan dsq.Result, 128)
	done := make(chan struct{})

	go func() {
		defer func() {
			if r := recover(); r != nil {
				output <- dsq.Result{Error: local.DBError(fmt.Sprintf("query failed: %v", r))}
			}
		}()
		if implicit {
			defer tx.Discard()
		}
		defer it.Close()
		defer close(output)

		if d.db.IsClosed() {
			output <- dsq.Result{Error: local.DBError("core repo closed")}
			return
		}

		it.Rewind()
		skipped := 0
		sent := 0
		for it.Valid() {
			item := it.Item()

			var e dsq.Entry
			e.Key = string(item.Key())
			e.Size = int(item.ValueSize())

			if !q.KeysOnly {
				val, err := item.ValueCopy(nil)
				if err != nil {
					output <- dsq.Result{Error: err}
					it.Next()
					continue
				}
				e.Value = val
			}

			if q.ReturnExpirations {
				e.Expiration = expires(item)
			}

			if len(q.Filters) > 0 && !filter(q.Filters, e) {
				it.Next()
				continue
			}

			if skipped < q.Offset {
				skipped++
				it.Next()
				continue
			}

			select {
			case output <- dsq.Result{Entry: e}:
				sent++
			case <-done:
				return
			}

			it.Next()

			if q.Limit > 0 && sent >= q.Limit {
				break
			}
		}
	}()

	return &myResults{
		query:  q,
		output: output,
		done:   done,
	}, nil
}

// filter returns _true_ if we should filter (skip) the entry
func filter(filters []dsq.Filter, entry dsq.Entry) bool {
	for _, f := range filters {
		if !f.Filter(entry) {
			return true
		}
	}
	return false
}

func expires(item *badger.Item) time.Time {
	expiresAt := item.ExpiresAt()
	if expiresAt > math.MaxInt64 {
		expiresAt--
	}
	return time.Unix(int64(expiresAt), 0) //#nosec
}

func (d *NodeRepo) Close() (err error) {
	if d == nil {
		return nil
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("close recovered: %v", r)
		}
	}()
	close(d.stopChan)

	log.Infoln("node repo: query interrupted")
	return nil
}

// Batch creates a new Batch object. This provides a way to do many writes, when
// there may be too many to fit into a single transaction.
func (d *NodeRepo) Batch(ctx context.Context) (ds.Batch, error) {
	if d == nil {
		return nil, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if d.db.IsClosed() {
		return nil, local.ErrNotRunning
	}

	b := &batch{d, d.db.InnerDB().NewWriteBatch()}
	// Ensure that incomplete transaction resources are cleaned up in case
	// batch is abandoned.
	runtime.SetFinalizer(b, func(b *batch) { _ = b.Cancel() })

	return b, nil
}

var _ ds.Batch = (*batch)(nil)

func (b *batch) Put(ctx context.Context, key ds.Key, value []byte) error {
	if b == nil {
		return ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if b.ds.db.IsClosed() {
		return local.ErrNotRunning
	}

	return b.put(key, value)
}

func (b *batch) put(key ds.Key, value []byte) error {
	if b == nil {
		return ErrNilNodeRepo
	}

	rootKey := buildRootKey(key)

	batchKey := local.NewPrefixBuilder(NodesNamespace).
		AddRootID(rootKey).
		Build()
	return b.writeBatch.Set(batchKey.Bytes(), value)
}

func (b *batch) putWithTTL(key ds.Key, value []byte, ttl time.Duration) error {
	if b == nil {
		return ErrNilNodeRepo
	}

	rootKey := buildRootKey(key)

	batchKey := local.NewPrefixBuilder(NodesNamespace).
		AddRootID(rootKey).
		Build()
	return b.writeBatch.SetEntry(badger.NewEntry(batchKey.Bytes(), value).WithTTL(ttl))
}

func (b *batch) Delete(ctx context.Context, key ds.Key) error {
	if b == nil {
		return ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if b.ds.db.IsClosed() {
		return local.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	batchKey := local.NewPrefixBuilder(NodesNamespace).
		AddRootID(rootKey).
		Build()
	return b.writeBatch.Delete(batchKey.Bytes())
}

func (b *batch) Commit(_ context.Context) error {
	if b == nil {
		return ErrNilNodeRepo
	}
	if b.ds.db.IsClosed() {
		return local.ErrNotRunning
	}

	err := b.writeBatch.Flush()
	if err != nil {
		// Discard incomplete transaction held by b.writeBatch
		_ = b.Cancel()
		return err
	}
	runtime.SetFinalizer(b, nil)
	return nil
}

func (b *batch) Cancel() error {
	if b == nil {
		return ErrNilNodeRepo
	}
	if b.ds.db.IsClosed() {
		return local.ErrNotRunning
	}

	b.writeBatch.Cancel()
	runtime.SetFinalizer(b, nil)
	return nil
}

const (
	ForeverBlockDuration time.Duration = 0
	MaxBlockDuration                   = 90 * 24 * time.Hour
)

type BlocklistedItem struct {
	PeerID   warpnet.WarpPeerID
	Duration *time.Duration
}

func (d *NodeRepo) BlocklistExponential(peerId warpnet.WarpPeerID) error {
	if d == nil {
		return ErrNilNodeRepo
	}
	if peerId == "" {
		return local.DBError("empty peer ID")
	}
	blocklistKey := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(BlocklistSubNamespace).
		AddRootID(peerId.String()).
		Build()

	txn, err := d.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	bt, err := txn.Get(blocklistKey)
	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return err
	}

	var item BlocklistedItem
	if len(bt) != 0 {
		if err := json.JSON.Unmarshal(bt, &item); err != nil {
			return err
		}
	}

	if item.Duration == nil {
		item.Duration = func(d time.Duration) *time.Duration { return &d }(time.Hour)
	}
	if *item.Duration == ForeverBlockDuration {
		return nil
	}

	newDuration := *item.Duration * 2
	item.Duration = &newDuration
	item.PeerID = peerId

	if *item.Duration > MaxBlockDuration {
		*item.Duration = ForeverBlockDuration
	}

	bt, err = json.JSON.Marshal(item)
	if err != nil {
		return err
	}

	if err := txn.SetWithTTL(blocklistKey, bt, *item.Duration); err != nil {
		return err
	}

	return txn.Commit()
}

func (d *NodeRepo) IsBlocklisted(peerId warpnet.WarpPeerID) (bool, error) {
	if d == nil {
		return false, ErrNilNodeRepo
	}
	if peerId == "" {
		return false, nil
	}
	blocklistKey := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(BlocklistSubNamespace).
		AddRootID(peerId.String()).
		Build()
	_, err := d.db.Get(blocklistKey)

	if errors.Is(err, local.ErrKeyNotFound) || errors.Is(err, ds.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (d *NodeRepo) BlocklistRemove(peerId warpnet.WarpPeerID) (err error) {
	if d == nil {
		return ErrNilNodeRepo
	}
	if peerId == "" {
		return local.DBError("empty peer ID")
	}
	blocklistKey := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(BlocklistSubNamespace).
		AddRootID(peerId.String()).
		Build()

	err = d.db.Delete(blocklistKey)
	if errors.Is(err, local.ErrKeyNotFound) || errors.Is(err, ds.ErrNotFound) {
		return nil
	}
	return err
}

func (d *NodeRepo) AddSelfHash(selfHashHex, version string) error {
	if d == nil {
		return ErrNilNodeRepo
	}
	if len(selfHashHex) == 0 {
		return local.DBError("empty codebase hash")
	}
	selfHashKey := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(SelfHashSubNamespace).
		AddRootID(version).
		Build()

	txn, err := d.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	bt, err := txn.Get(selfHashKey)
	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return err
	}

	var item = make(map[string]struct{})
	if len(bt) != 0 {
		if err := json.JSON.Unmarshal(bt, &item); err != nil {
			return err
		}
	}

	item[selfHashHex] = struct{}{}

	bt, err = json.JSON.Marshal(item)
	if err != nil {
		return err
	}

	if err := txn.Set(selfHashKey, bt); err != nil {
		return err
	}

	return txn.Commit()
}

func (d *NodeRepo) PruneOldSelfHashes(currentVersion *semver.Version) error {
	if d == nil {
		return ErrNilNodeRepo
	}
	if currentVersion == nil {
		return local.DBError("empty current version value")
	}

	selfHashPrefix := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(SelfHashSubNamespace).Build()

	txn, err := d.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	var limit uint64 = 100
	items, _, err := txn.List(selfHashPrefix, &limit, nil)
	if err != nil {
		return err
	}

	for _, item := range items {
		key := item.Key

		versionSuffix := strings.TrimPrefix(key, selfHashPrefix.String()+"/")
		itemVersion, err := semver.NewVersion(versionSuffix)
		if err != nil {
			return fmt.Errorf("node repo: semver: %s, %v", versionSuffix, err)
		}

		if itemVersion.Major() < currentVersion.Major() {
			if err := txn.Delete(local.DatabaseKey(key)); err != nil {
				return err
			}
		}
	}
	return txn.Commit()
}

var ErrNotInRecords = local.DBError("self hash is not in the consensus records")

const SelfHashConsensusKey = "selfhash"

func (d *NodeRepo) ValidateSelfHash(ev event.ValidationEvent) error {
	if d == nil {
		return ErrNilNodeRepo
	}

	if len(ev.SelfHashHex) == 0 {
		return local.DBError("empty codebase hash")
	}

	if d.db == nil {
		if d.BootstrapSelfHashHex != ev.SelfHashHex {
			return ErrNotInRecords
		}
		return nil
	}

	selfHashPrefix := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(SelfHashSubNamespace).Build()

	txn, err := d.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	var limit uint64 = 100
	items, _, err := txn.List(selfHashPrefix, &limit, nil)
	if err != nil {
		return err
	}

	itemsHashes := make(map[string]struct{})
	for _, item := range items {
		if err := json.JSON.Unmarshal(item.Value, &itemsHashes); err != nil {
			return err
		}
	}

	for h := range itemsHashes {
		if h == ev.SelfHashHex {
			return txn.Discard()
		}
	}

	return ErrNotInRecords
}

func (d *NodeRepo) GetSelfHashes() (map[string]struct{}, error) {
	if d == nil {
		return nil, ErrNilNodeRepo
	}

	selfHashPrefix := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(SelfHashSubNamespace).Build()

	txn, err := d.db.NewTxn()
	if err != nil {
		return nil, err
	}
	defer txn.Rollback()

	var limit uint64 = 100
	items, _, err := txn.List(selfHashPrefix, &limit, nil)
	if err != nil {
		return nil, err
	}

	allVersionsHashes := make(map[string]struct{})
	for _, item := range items {
		if err := json.JSON.Unmarshal(item.Value, &allVersionsHashes); err != nil {
			return nil, err
		}
	}
	return allVersionsHashes, nil
}

func buildRootKey(key ds.Key) string {
	rootKey := strings.TrimPrefix(key.String(), "/")
	if len(rootKey) == 0 {
		rootKey = key.String()
	}
	return rootKey
}

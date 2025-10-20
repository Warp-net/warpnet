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
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database/local"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

// slash is required because of: invalid datastore key: NODES:/peers/keys/AASAQAISEAXNRKHMX2O3AA26JM7NGIWUPOGIITJ2UHHXGX4OWIEKPNAW6YCSK/priv
const (
	NodesNamespace        = "/NODES"
	BlocklistSubNamespace = "BLOCKLIST"

	ErrNilNodeRepo = local.DBError("node repo is nil")
)

var _ local.Batching = (*NodeRepo)(nil)

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

func NewNodeRepo(db NodeStorer) *NodeRepo {
	nr := &NodeRepo{
		db:       db,
		stopChan: make(chan struct{}),
	}

	return nr
}

func (d *NodeRepo) Put(ctx context.Context, key local.Key, value []byte) error {
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

func (d *NodeRepo) Sync(ctx context.Context, _ local.Key) error {
	if d == nil {
		return ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return d.db.Sync()
}

func (d *NodeRepo) PutWithTTL(ctx context.Context, key local.Key, value []byte, ttl time.Duration) error {
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

func (d *NodeRepo) SetTTL(ctx context.Context, key local.Key, ttl time.Duration) error {
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

func (d *NodeRepo) GetExpiration(ctx context.Context, key local.Key) (t time.Time, err error) {
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
	if local.IsNotFoundError(err) {
		return t, *local.ErrDatastoreKeyNotFound
	}
	if err != nil {
		return t, err
	}

	if expiresAt > math.MaxInt64 {
		expiresAt = math.MaxInt64
	}
	expiration = time.Unix(int64(expiresAt), 0) //#nosec

	return expiration, err
}

func (d *NodeRepo) Get(ctx context.Context, key local.Key) (value []byte, err error) {
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
	if local.IsNotFoundError(err) {
		return nil, *local.ErrDatastoreKeyNotFound
	}
	if err != nil {
		return nil, err
	}

	return value, err
}

func (d *NodeRepo) Has(ctx context.Context, key local.Key) (_ bool, err error) {
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
	case local.IsNotFoundError(err):
		return false, nil
	case err == nil:
		return true, nil
	default:
		return false, fmt.Errorf("has: %w", err)
	}
}

func (d *NodeRepo) GetSize(ctx context.Context, key local.Key) (_ int, err error) {
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
	case local.IsNotFoundError(err):
		return 0, *local.ErrDatastoreKeyNotFound
	default:
		return 0, fmt.Errorf("size: %w", err)
	}
}

func (d *NodeRepo) Delete(ctx context.Context, key local.Key) error {
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

func (d *NodeRepo) Query(ctx context.Context, q local.Query) (local.Results, error) {
	if d == nil {
		return nil, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if d.db.IsClosed() {
		return nil, local.ErrNotRunning
	}

	tx := d.db.InnerDB().NewTransaction(true)
	return d.query(tx, q)
}

func (d *NodeRepo) query(tx *local.Txn, q local.Query) (local.Results, error) {
	opt := local.DefaultIteratorOptions
	opt.PrefetchValues = !q.KeysOnly

	prefix := local.NewKey(q.Prefix).String()
	if prefix != "/" {
		opt.Prefix = []byte(prefix + "/")
	}

	// Handle ordering
	if len(q.Orders) > 0 {
		switch q.Orders[0].(type) {
		case local.OrderByKey, *local.OrderByKey:
		// We order by key by default.
		case local.OrderByKeyDescending, *local.OrderByKeyDescending:
			// Reverse order by key
			opt.Reverse = true
		default:
			// Ok, we have a weird order we can't handle. Let's
			// perform the _base_ query (prefix, filter, etc.), then
			// handle sort/offset/limit later.

			// Skip the stuff we can't apply.
			baseQuery := q
			baseQuery.Limit = 0
			baseQuery.Offset = 0
			baseQuery.Orders = nil

			// perform the base query.
			res, err := d.query(tx, baseQuery)
			if err != nil {
				return nil, err
			}

			res = local.ResultsReplaceQuery(res, q)

			naiveQuery := q
			naiveQuery.Prefix = ""
			naiveQuery.Filters = nil

			return local.NaiveQueryApply(naiveQuery, res), nil
		}
	}

	it := tx.NewIterator(opt)
	results := local.ResultsWithContext(q, func(ctx context.Context, output chan<- local.Result) {
		closedEarly := false
		defer func() {
			if closedEarly {
				select {
				case output <- local.Result{
					Error: local.ErrNotRunning,
				}:
				case <-ctx.Done():
				}
			}
		}()
		if !d.db.IsClosed() {
			closedEarly = true
			return
		}

		defer tx.Discard()
		defer it.Close()

		it.Rewind()

		for skipped := 0; skipped < q.Offset && it.Valid(); it.Next() {
			if len(q.Filters) == 0 {
				skipped++
				continue
			}
			item := it.Item()

			matches := true
			check := func(value []byte) error {
				e := local.DsEntry{
					Key:   string(item.Key()),
					Value: value,
					Size:  int(item.ValueSize()),
				}

				if q.ReturnExpirations {
					e.Expiration = expires(item)
				}
				matches = filter(q.Filters, e)
				return nil
			}

			var err error
			if q.KeysOnly {
				err = check(nil)
			} else {
				err = item.Value(check)
			}

			if err != nil {
				select {
				case output <- local.Result{Error: err}:
				case <-d.stopChan:
					closedEarly = true
					return
				case <-ctx.Done():
					return
				}
			}
			if !matches {
				skipped++
			}
		}

		for sent := 0; (q.Limit <= 0 || sent < q.Limit) && it.Valid(); it.Next() {
			item := it.Item()
			e := local.DsEntry{Key: string(item.Key())}

			var result local.Result
			if !q.KeysOnly {
				b, err := item.ValueCopy(nil)
				if err != nil {
					result = local.Result{Error: err}
				} else {
					e.Value = b
					e.Size = len(b)
					result = local.Result{Entry: e}
				}
			} else {
				e.Size = int(item.ValueSize())
				result = local.Result{Entry: e}
			}

			if q.ReturnExpirations {
				result.Expiration = expires(item)
			}

			if result.Error == nil && filter(q.Filters, e) {
				continue
			}

			select {
			case output <- result:
				sent++
			case <-d.stopChan:
				closedEarly = true
				return
			case <-ctx.Done():
				return
			}
		}
	})

	return results, nil
}

func filter(filters []local.Filter, entry local.DsEntry) bool {
	for _, f := range filters {
		if !f.Filter(entry) {
			return true
		}
	}
	return false
}

func expires(item *local.Item) time.Time {
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

// Implements the datastore.Batch interface, enabling batching support for
// the badger Datastore.
type batch struct {
	ds         *NodeRepo
	writeBatch *local.WriteBatch
}

// Batch creates a new Batch object. This provides a way to do many writes, when
// there may be too many to fit into a single transaction.
func (d *NodeRepo) Batch(ctx context.Context) (local.Batch, error) {
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

var _ local.Batch = (*batch)(nil)

func (b *batch) Put(ctx context.Context, key local.Key, value []byte) error {
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

func (b *batch) put(key local.Key, value []byte) error {
	if b == nil {
		return ErrNilNodeRepo
	}

	rootKey := buildRootKey(key)

	batchKey := local.NewPrefixBuilder(NodesNamespace).
		AddRootID(rootKey).
		Build()
	return b.writeBatch.Set(batchKey.Bytes(), value)
}

func (b *batch) putWithTTL(key local.Key, value []byte, ttl time.Duration) error {
	if b == nil {
		return ErrNilNodeRepo
	}

	rootKey := buildRootKey(key)

	batchKey := local.NewPrefixBuilder(NodesNamespace).
		AddRootID(rootKey).
		Build()
	return b.writeBatch.SetEntry(&local.Entry{
		Key:       batchKey.Bytes(),
		Value:     value,
		ExpiresAt: uint64(ttl),
	})
}

func (b *batch) Delete(ctx context.Context, key local.Key) error {
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
	if err != nil && !local.IsNotFoundError(err) {
		return err
	}

	var item BlocklistedItem
	if len(bt) != 0 {
		if err := json.Unmarshal(bt, &item); err != nil {
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

	bt, err = json.Marshal(item)
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

	if local.IsNotFoundError(err) {
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
	if local.IsNotFoundError(err) {
		return nil
	}
	return err
}

func buildRootKey(key local.Key) string {
	rootKey := strings.TrimPrefix(key.String(), "/")
	if len(rootKey) == 0 {
		rootKey = key.String()
	}
	return rootKey
}

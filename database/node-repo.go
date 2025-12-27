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

	"github.com/Warp-net/warpnet/database/datastore"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

// slash is required because of: invalid datastore key: NODES:/peers/keys/AASAQAISEAXNRKHMX2O3AA26JM7NGIWUPOGIITJ2UHHXGX4OWIEKPNAW6YCSK/priv
const (
	requiredPrefixSlash = "/"

	ErrNilNodeRepo = local_store.DBError("node repo is nil")
)

var _ datastore.Batching = (*NodeRepo)(nil)

type NodeStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
	Get(key local_store.DatabaseKey) ([]byte, error)
	GetExpiration(key local_store.DatabaseKey) (uint64, error)
	GetSize(key local_store.DatabaseKey) (int64, error)
	Sync() error
	IsClosed() bool
	InnerDB() *local_store.WarpDB
	SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error
	Set(key local_store.DatabaseKey, value []byte) error
	Delete(key local_store.DatabaseKey) error
}

type NodeRepo struct {
	db       NodeStorer
	prefix   string
	stopChan chan struct{}

	BootstrapSelfHashHex string
}

func NewNodeRepo(db NodeStorer, prefix string) *NodeRepo {
	if !strings.HasPrefix(prefix, requiredPrefixSlash) {
		prefix = requiredPrefixSlash + prefix
	}
	nr := &NodeRepo{
		db:       db,
		prefix:   prefix,
		stopChan: make(chan struct{}),
	}
	return nr
}

func (d *NodeRepo) Prefix() string {
	return d.prefix
}

func (d *NodeRepo) Put(ctx context.Context, key datastore.Key, value []byte) error {
	if d == nil || d.db == nil {
		return ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if d.db.IsClosed() {
		return local_store.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	prefix := local_store.NewPrefixBuilder(d.prefix).
		AddRootID(rootKey).
		Build()
	return d.db.Set(prefix, value)
}

func (d *NodeRepo) Sync(ctx context.Context, _ datastore.Key) error {
	if d == nil || d.db == nil {
		return ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if d.db.IsClosed() {
		return local_store.ErrNotRunning
	}

	return d.db.Sync()
}

func (d *NodeRepo) PutWithTTL(ctx context.Context, key datastore.Key, value []byte, ttl time.Duration) error {
	if d == nil || d.db == nil {
		return ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if d.db.IsClosed() {
		return local_store.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	prefix := local_store.NewPrefixBuilder(d.prefix).
		AddRootID(rootKey).
		Build()

	return d.db.SetWithTTL(prefix, value, ttl)
}

func (d *NodeRepo) SetTTL(ctx context.Context, key datastore.Key, ttl time.Duration) error {
	if d == nil || d.db == nil {
		return ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if d.db.IsClosed() {
		return local_store.ErrNotRunning
	}

	item, err := d.Get(ctx, key)
	if err != nil {
		return err
	}
	return d.PutWithTTL(ctx, key, item, ttl)
}

func (d *NodeRepo) GetExpiration(ctx context.Context, key datastore.Key) (t time.Time, err error) {
	if d == nil || d.db == nil {
		return t, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return t, ctx.Err()
	}
	if d.db.IsClosed() {
		return t, local_store.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	prefix := local_store.NewPrefixBuilder(d.prefix).
		AddRootID(rootKey).
		Build()

	expiresAt, err := d.db.GetExpiration(prefix)
	if local_store.IsNotFoundError(err) {
		return t, local_store.ToDatatoreErrNotFound(err)
	}
	if err != nil {
		return t, err
	}

	if expiresAt > math.MaxInt64 {
		expiresAt = math.MaxInt64
	}
	expiration := time.Unix(int64(expiresAt), 0) //#nosec

	return expiration, err
}

func (d *NodeRepo) Get(ctx context.Context, key datastore.Key) (value []byte, err error) {
	if d == nil || d.db == nil {
		return nil, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if d.db.IsClosed() {
		return nil, local_store.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	prefix := local_store.NewPrefixBuilder(d.prefix).
		AddRootID(rootKey).
		Build()

	value, err = d.db.Get(prefix)
	if local_store.IsNotFoundError(err) {
		return nil, local_store.ToDatatoreErrNotFound(err)
	}
	if err != nil {
		return nil, err
	}

	return value, err
}

func (d *NodeRepo) Has(ctx context.Context, key datastore.Key) (_ bool, err error) {
	if d == nil || d.db == nil {
		return false, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if d.db.IsClosed() {
		return false, local_store.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	prefix := local_store.NewPrefixBuilder(d.prefix).
		AddRootID(rootKey).
		Build()

	_, err = d.db.Get(prefix)
	switch {
	case local_store.IsNotFoundError(err):
		return false, nil
	case err == nil:
		return true, nil
	default:
		return false, fmt.Errorf("has: %w", err)
	}
}

func (d *NodeRepo) GetSize(ctx context.Context, key datastore.Key) (_ int, err error) {
	size := -1
	if d == nil || d.db == nil {
		return size, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return size, ctx.Err()
	}
	if d.db.IsClosed() {
		return size, local_store.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	prefix := local_store.NewPrefixBuilder(d.prefix).
		AddRootID(rootKey).
		Build()

	itemSize, err := d.db.GetSize(prefix)
	switch {
	case err == nil:
		return int(itemSize), nil
	case local_store.IsNotFoundError(err):
		return 0, local_store.ToDatatoreErrNotFound(err)
	default:
		return 0, fmt.Errorf("size: %w", err)
	}
}

func (d *NodeRepo) Delete(ctx context.Context, key datastore.Key) error {
	if d == nil || d.db == nil {
		return ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if d.db.IsClosed() {
		return local_store.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	prefix := local_store.NewPrefixBuilder(d.prefix).
		AddRootID(rootKey).
		Build()

	return d.db.Delete(prefix)
}

// DiskUsage implements the PersistentDatastore interface.
// It returns the sum of lsm and value log files sizes in bytes.
func (d *NodeRepo) DiskUsage(ctx context.Context) (uint64, error) {
	if d == nil || d.db == nil {
		return 0, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if d.db.IsClosed() {
		return 0, local_store.ErrNotRunning
	}

	lsm, vlog := d.db.InnerDB().Size()
	if (lsm + vlog) < 0 {
		return 0, local_store.DBError("disk usage: malformed value")
	}
	return uint64(lsm + vlog), nil //#nosec
}

func (d *NodeRepo) Query(ctx context.Context, q datastore.Query) (datastore.Results, error) {
	if d == nil || d.db == nil {
		return nil, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if d.db.IsClosed() {
		return nil, local_store.ErrNotRunning
	}

	tx := d.db.InnerDB().NewTransaction(true)
	return d.query(tx, q)
}

func (d *NodeRepo) query(tx *local_store.Txn, q datastore.Query) (_ datastore.Results, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = local_store.DBError("node repo: query recovered")
			err = fmt.Errorf("%w: %v", err, r)
		}
	}()

	if d.db.IsClosed() {
		return nil, local_store.ErrNotRunning
	}
	opt := local_store.DefaultIteratorOptions
	opt.PrefetchValues = !q.KeysOnly

	key := strings.TrimPrefix(q.Prefix, "/")
	prefix := local_store.NewPrefixBuilder(d.prefix).AddRootID(key).Build().Bytes()
	opt.Prefix = prefix

	// Handle ordering
	if len(q.Orders) > 0 {
		switch q.Orders[0].(type) {
		case datastore.OrderByKey, *datastore.OrderByKey:
		// We order by key by default.
		case datastore.OrderByKeyDescending, *datastore.OrderByKeyDescending:
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

			res = datastore.ResultsReplaceQuery(res, q)

			naiveQuery := q
			naiveQuery.Prefix = ""
			naiveQuery.Filters = nil

			return datastore.NaiveQueryApply(naiveQuery, res), nil
		}
	}

	it := tx.NewIterator(opt)
	results := datastore.ResultsWithContext(q, func(ctx context.Context, output chan<- datastore.Result) {
		defer tx.Discard()
		defer it.Close()

		it.Rewind()

		for skipped := 0; skipped < q.Offset && it.Valid(); it.Next() {
			if d.db.IsClosed() {
				return
			}

			if len(q.Filters) == 0 {
				skipped++
				continue
			}
			item := it.Item()

			matches := true
			check := func(value []byte) error {
				e := datastore.DsEntry{
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
				case output <- datastore.Result{Error: err}:
				case <-d.stopChan:
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
			if d.db.IsClosed() {
				return
			}
			item := it.Item()
			e := datastore.DsEntry{Key: string(item.Key())}

			var result datastore.Result
			if !q.KeysOnly {
				b, err := item.ValueCopy(nil)
				if err != nil {
					result = datastore.Result{Error: err}
				} else {
					e.Value = b
					e.Size = len(b)
					result = datastore.Result{Entry: e}
				}
			} else {
				e.Size = int(item.ValueSize())
				result = datastore.Result{Entry: e}
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
				return
			case <-ctx.Done():
				return
			}
		}
	})

	return results, nil
}

func filter(filters []datastore.Filter, entry datastore.DsEntry) bool {
	for _, f := range filters {
		if !f.Filter(entry) {
			return true
		}
	}
	return false
}

func expires(item *local_store.Item) time.Time {
	expiresAt := item.ExpiresAt()
	if expiresAt > math.MaxInt64 {
		expiresAt--
	}
	return time.Unix(int64(expiresAt), 0) //#nosec
}

func (d *NodeRepo) Close() (err error) {
	if d == nil || d.db == nil {
		return nil
	}
	if d.db.IsClosed() {
		return nil
	}

	close(d.stopChan)
	log.Infoln("node repo: closed")
	return nil
}

type batch struct {
	db         NodeStorer
	prefix     string
	writeBatch *local_store.WriteBatch
}

var _ datastore.Batch = (*batch)(nil)

func (d *NodeRepo) Batch(ctx context.Context) (datastore.Batch, error) {
	if d == nil || d.db == nil {
		return nil, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if d.db.IsClosed() {
		return nil, local_store.ErrNotRunning
	}
	b := &batch{d.db, d.prefix, d.db.InnerDB().NewWriteBatch()}
	runtime.SetFinalizer(b, func(b *batch) { _ = b.Cancel() })

	return b, nil
}

func (b *batch) Put(ctx context.Context, key datastore.Key, value []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return b.put(key, value)
}

func (b *batch) put(key datastore.Key, value []byte) error {
	if b == nil || b.db == nil || b.writeBatch == nil {
		return ErrNilNodeRepo
	}
	if b.db.IsClosed() {
		return local_store.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	batchKey := local_store.NewPrefixBuilder(b.prefix).
		AddRootID(rootKey).
		Build()
	return b.writeBatch.Set(batchKey.Bytes(), value)
}

func (b *batch) putWithTTL(key datastore.Key, value []byte, ttl time.Duration) error {
	if b == nil || b.db == nil || b.writeBatch == nil {
		return ErrNilNodeRepo
	}
	if b.db.IsClosed() {
		return local_store.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	batchKey := local_store.NewPrefixBuilder(b.prefix).
		AddRootID(rootKey).
		Build()
	return b.writeBatch.SetEntry(&local_store.Entry{
		Key:       batchKey.Bytes(),
		Value:     value,
		ExpiresAt: uint64(time.Now().Add(ttl).Unix()), //#nosec
	})
}

func (b *batch) Delete(ctx context.Context, key datastore.Key) error {
	if b == nil || b.db == nil || b.writeBatch == nil {
		return ErrNilNodeRepo
	}
	if b.db.IsClosed() {
		return local_store.ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	rootKey := buildRootKey(key)

	batchKey := local_store.NewPrefixBuilder(b.prefix).
		AddRootID(rootKey).
		Build()
	return b.writeBatch.Delete(batchKey.Bytes())
}

func (b *batch) Commit(ctx context.Context) error {
	if b == nil || b.db == nil || b.writeBatch == nil {
		return ErrNilNodeRepo
	}
	if b.db.IsClosed() {
		_ = b.Cancel()
		return nil
	}
	if ctx.Err() != nil {
		_ = b.Cancel()
		return ctx.Err()
	}

	err := b.writeBatch.Flush()
	_ = b.Cancel()
	return err
}

func (b *batch) Cancel() error {
	if b == nil || b.db == nil || b.writeBatch == nil {
		return nil
	}

	b.writeBatch.Cancel()
	b.writeBatch = nil
	runtime.SetFinalizer(b, nil)
	return nil
}

func buildRootKey(key datastore.Key) string {
	rootKey := strings.TrimPrefix(key.String(), "/")
	if len(rootKey) == 0 {
		rootKey = key.String()
	}
	return rootKey
}

const (
	BlocklistSubNamespace     = "BLOCKLIST"
	BlocklistUserSubNamespace = "USER"
	BlocklistTermSubNamespace = "TERM"
)

type BlockLevel int

func (b BlockLevel) Next() BlockLevel {
	if b >= PermanentBlock {
		return PermanentBlock
	}
	return b + 1
}

const (
	InitialBlock BlockLevel = iota + 1
	MediumBlock
	AdvancedBlock
	PermanentBlock
)

const (
	noExpiryBlockDuration time.Duration = 0
	advancedBlockDuration               = 7 * 24 * time.Hour
	mediumBlockDuration                 = 24 * time.Hour
	initialBlockDuration                = time.Hour
)

var blockDurationMapping = map[BlockLevel]time.Duration{
	InitialBlock:   initialBlockDuration,
	MediumBlock:    mediumBlockDuration,
	AdvancedBlock:  advancedBlockDuration,
	PermanentBlock: noExpiryBlockDuration,
}

type BlocklistTerm struct {
	PeerID string
	Level  BlockLevel
}

func (d *NodeRepo) Blocklist(peerId string) error {
	if d == nil {
		return ErrNilNodeRepo
	}
	if peerId == "" {
		return local_store.DBError("empty peer ID")
	}

	txn, err := d.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	blocklistTermKey := local_store.NewPrefixBuilder(d.prefix).
		AddSubPrefix(BlocklistSubNamespace).
		AddSubPrefix(BlocklistTermSubNamespace).
		AddRootID(peerId).
		Build()

	bt, err := txn.Get(blocklistTermKey)
	if err != nil && !local_store.IsNotFoundError(err) {
		return err
	}

	var term BlocklistTerm
	if bt != nil {
		if err := json.Unmarshal(bt, &term); err != nil {
			return err
		}
	}

	if term.Level == 0 {
		term.Level = InitialBlock
	} else {
		term.Level = term.Level.Next()
	}

	dur := blockDurationMapping[term.Level]

	blocklistUserKey := local_store.NewPrefixBuilder(d.prefix).
		AddSubPrefix(BlocklistSubNamespace).
		AddSubPrefix(BlocklistUserSubNamespace).
		AddRootID(peerId).
		Build()

	if err := txn.SetWithTTL(blocklistUserKey, []byte{}, dur); err != nil {
		return err
	}

	bt, err = json.Marshal(term)
	if err != nil {
		return err
	}

	if err := txn.Set(blocklistTermKey, bt); err != nil {
		return err
	}
	return txn.Commit()
}

func (d *NodeRepo) IsBlocklisted(peerId string) bool {
	if d == nil {
		return false
	}
	if peerId == "" {
		return false
	}

	blocklistUserKey := local_store.NewPrefixBuilder(d.prefix).
		AddSubPrefix(BlocklistSubNamespace).
		AddSubPrefix(BlocklistUserSubNamespace).
		AddRootID(peerId).
		Build()
	_, err := d.db.Get(blocklistUserKey)
	return err == nil
}

func (d *NodeRepo) BlocklistTerm(peerId string) (*BlocklistTerm, error) {
	if d == nil {
		return nil, ErrNilNodeRepo
	}
	if peerId == "" {
		return nil, local_store.DBError("empty peer ID")
	}
	var term BlocklistTerm

	blocklistTermKey := local_store.NewPrefixBuilder(d.prefix).
		AddSubPrefix(BlocklistSubNamespace).
		AddSubPrefix(BlocklistTermSubNamespace).
		AddRootID(peerId).
		Build()
	bt, err := d.db.Get(blocklistTermKey)
	if local_store.IsNotFoundError(err) {
		return &term, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(bt, &term); err != nil {
		return nil, err
	}
	return &term, nil
}

func (d *NodeRepo) BlocklistRemove(peerId string) error {
	if d == nil {
		return nil
	}
	if peerId == "" {
		return nil
	}
	txn, err := d.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	blocklistUserKey := local_store.NewPrefixBuilder(d.prefix).
		AddSubPrefix(BlocklistSubNamespace).
		AddSubPrefix(BlocklistUserSubNamespace).
		AddRootID(peerId).
		Build()
	_ = txn.Delete(blocklistUserKey)

	blocklistTermKey := local_store.NewPrefixBuilder(d.prefix).
		AddSubPrefix(BlocklistSubNamespace).
		AddSubPrefix(BlocklistTermSubNamespace).
		AddRootID(peerId).
		Build()
	term := BlocklistTerm{PeerID: peerId, Level: 0}
	bt, err := json.Marshal(term)
	if err != nil {
		return err
	}
	if err := txn.Set(blocklistTermKey, bt); err != nil {
		return err
	}
	return txn.Commit()
}

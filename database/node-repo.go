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
	NodesNamespace = "/NODES"

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
	if d == nil || d.db == nil {
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
	return d.db.Set(prefix, value)
}

func (d *NodeRepo) Sync(ctx context.Context, _ local.Key) error {
	if d == nil || d.db == nil {
		return ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if d.db.IsClosed() {
		return local.ErrNotRunning
	}

	return d.db.Sync()
}

func (d *NodeRepo) PutWithTTL(ctx context.Context, key local.Key, value []byte, ttl time.Duration) error {
	if d == nil || d.db == nil {
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

	return d.db.SetWithTTL(prefix, value, ttl)

}

func (d *NodeRepo) SetTTL(ctx context.Context, key local.Key, ttl time.Duration) error {
	if d == nil || d.db == nil {
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
	if d == nil || d.db == nil {
		return t, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return t, ctx.Err()
	}
	if d.db.IsClosed() {
		return t, local.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	prefix := local.NewPrefixBuilder(NodesNamespace).
		AddRootID(rootKey).
		Build()

	expiresAt, err := d.db.GetExpiration(prefix)
	if local.IsNotFoundError(err) {
		return t, local.ToDatatoreErrNotFound(err)
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

func (d *NodeRepo) Get(ctx context.Context, key local.Key) (value []byte, err error) {
	if d == nil || d.db == nil {
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
		return nil, local.ToDatatoreErrNotFound(err)
	}
	if err != nil {
		return nil, err
	}

	return value, err
}

func (d *NodeRepo) Has(ctx context.Context, key local.Key) (_ bool, err error) {
	if d == nil || d.db == nil {
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
	size := -1
	if d == nil || d.db == nil {
		return size, ErrNilNodeRepo
	}
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
		return 0, local.ToDatatoreErrNotFound(err)
	default:
		return 0, fmt.Errorf("size: %w", err)
	}
}

func (d *NodeRepo) Delete(ctx context.Context, key local.Key) error {
	if d == nil || d.db == nil {
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
	if d == nil || d.db == nil {
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
	if d == nil || d.db == nil {
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

func (d *NodeRepo) query(tx *local.Txn, q local.Query) (_ local.Results, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("node repo: query recover: %v", r)
		}
	}()

	if d.db.IsClosed() {
		return nil, local.ErrNotRunning
	}
	opt := local.DefaultIteratorOptions
	opt.PrefetchValues = !q.KeysOnly

	key := strings.TrimPrefix(q.Prefix, "/")
	prefix := local.NewPrefixBuilder(NodesNamespace).AddRootID(key).Build().Bytes()
	opt.Prefix = prefix

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
	writeBatch *local.WriteBatch
}

var _ local.Batch = (*batch)(nil)

func (d *NodeRepo) Batch(ctx context.Context) (local.Batch, error) {
	if d == nil || d.db == nil {
		return nil, ErrNilNodeRepo
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if d.db.IsClosed() {
		return nil, local.ErrNotRunning
	}
	b := &batch{d.db, d.db.InnerDB().NewWriteBatch()}
	runtime.SetFinalizer(b, func(b *batch) { _ = b.Cancel() })

	return b, nil
}

func (b *batch) Put(ctx context.Context, key local.Key, value []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return b.put(key, value)
}

func (b *batch) put(key local.Key, value []byte) error {
	if b == nil || b.db == nil || b.writeBatch == nil {
		return ErrNilNodeRepo
	}
	if b.db.IsClosed() {
		return local.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	batchKey := local.NewPrefixBuilder(NodesNamespace).
		AddRootID(rootKey).
		Build()
	return b.writeBatch.Set(batchKey.Bytes(), value)
}

func (b *batch) putWithTTL(key local.Key, value []byte, ttl time.Duration) error {
	if b == nil || b.db == nil || b.writeBatch == nil {
		return ErrNilNodeRepo
	}
	if b.db.IsClosed() {
		return local.ErrNotRunning
	}

	rootKey := buildRootKey(key)

	batchKey := local.NewPrefixBuilder(NodesNamespace).
		AddRootID(rootKey).
		Build()
	return b.writeBatch.SetEntry(&local.Entry{
		Key:       batchKey.Bytes(),
		Value:     value,
		ExpiresAt: uint64(time.Now().Add(ttl).Unix()),
	})
}

func (b *batch) Delete(ctx context.Context, key local.Key) error {
	if b == nil || b.db == nil || b.writeBatch == nil {
		return ErrNilNodeRepo
	}
	if b.db.IsClosed() {
		return local.ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	rootKey := buildRootKey(key)

	batchKey := local.NewPrefixBuilder(NodesNamespace).
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

func buildRootKey(key local.Key) string {
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
	PeerID warpnet.WarpPeerID
	Level  BlockLevel
}

func (d *NodeRepo) Blocklist(peerId warpnet.WarpPeerID) error {
	if d == nil {
		return ErrNilNodeRepo
	}
	if peerId == "" {
		return local.DBError("empty peer ID")
	}

	txn, err := d.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	blocklistTermKey := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(BlocklistSubNamespace).
		AddSubPrefix(BlocklistTermSubNamespace).
		AddRootID(peerId.String()).
		Build()

	bt, err := txn.Get(blocklistTermKey)
	if err != nil && !local.IsNotFoundError(err) {
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

	blocklistUserKey := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(BlocklistSubNamespace).
		AddSubPrefix(BlocklistUserSubNamespace).
		AddRootID(peerId.String()).
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

func (d *NodeRepo) IsBlocklisted(peerId warpnet.WarpPeerID) bool {
	if d == nil {
		return false
	}
	if peerId == "" {
		return false
	}

	blocklistUserKey := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(BlocklistSubNamespace).
		AddSubPrefix(BlocklistUserSubNamespace).
		AddRootID(peerId.String()).
		Build()
	_, err := d.db.Get(blocklistUserKey)
	return err == nil
}

func (d *NodeRepo) BlocklistTerm(peerId warpnet.WarpPeerID) (*BlocklistTerm, error) {
	if d == nil {
		return nil, ErrNilNodeRepo
	}
	if peerId == "" {
		return nil, local.DBError("empty peer ID")
	}
	var term BlocklistTerm

	blocklistTermKey := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(BlocklistSubNamespace).
		AddSubPrefix(BlocklistTermSubNamespace).
		AddRootID(peerId.String()).
		Build()
	bt, err := d.db.Get(blocklistTermKey)
	if local.IsNotFoundError(err) {
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

func (d *NodeRepo) BlocklistRemove(peerId warpnet.WarpPeerID) error {
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

	blocklistUserKey := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(BlocklistSubNamespace).
		AddSubPrefix(BlocklistUserSubNamespace).
		AddRootID(peerId.String()).
		Build()
	_ = txn.Delete(blocklistUserKey)

	blocklistTermKey := local.NewPrefixBuilder(NodesNamespace).
		AddSubPrefix(BlocklistSubNamespace).
		AddSubPrefix(BlocklistTermSubNamespace).
		AddRootID(peerId.String()).
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

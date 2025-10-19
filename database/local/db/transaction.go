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
	"encoding/binary"
	"errors"
	"time"

	"github.com/dgraph-io/badger/v4"
	ds "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
)

type CustomTransaction interface {
	ds.Txn
	NewIterator(opt IteratorOptions) (*Iterator, error)
	PutWithTTL(ctx context.Context, key DatastoreKey, value []byte, ttl time.Duration) error
	Increment(key DatastoreKey) (uint64, error)
	Decrement(key DatastoreKey) (uint64, error)
}

type txn struct {
	ds       *LocalDatastore
	txn      *badger.Txn
	implicit bool
}

func (t *txn) Put(ctx context.Context, key DatastoreKey, value []byte) error {
	if !t.ds.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return t.txn.Set(key.Bytes(), value)
}

func (t *txn) Sync(_ context.Context, _ DatastoreKey) error {
	return nil
}

func (t *txn) PutWithTTL(ctx context.Context, key DatastoreKey, value []byte, ttl time.Duration) error {
	if !t.ds.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return t.txn.SetEntry(badger.NewEntry(key.Bytes(), value).WithTTL(ttl))
}

func (t *txn) GetExpiration(ctx context.Context, key DatastoreKey) (time.Time, error) {
	if !t.ds.isRunning.Load() {
		return time.Time{}, ErrNotRunning
	}
	if ctx.Err() != nil {
		return time.Time{}, ctx.Err()
	}

	item, err := t.txn.Get(key.Bytes())
	if errors.Is(err, badger.ErrKeyNotFound) || errors.Is(err, ds.ErrNotFound) {
		return time.Time{}, ds.ErrNotFound
	} else if err != nil {
		return time.Time{}, err
	}
	return time.Unix(int64(item.ExpiresAt()), 0), nil
}

func (t *txn) SetTTL(ctx context.Context, key DatastoreKey, ttl time.Duration) error {
	if !t.ds.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	item, err := t.txn.Get(key.Bytes())
	if err != nil {
		return err
	}
	return item.Value(func(data []byte) error {
		return t.txn.SetEntry(badger.NewEntry(key.Bytes(), data).WithTTL(ttl))
	})
}

func (t *txn) Get(ctx context.Context, key DatastoreKey) ([]byte, error) {
	if !t.ds.isRunning.Load() {
		return nil, ErrNotRunning
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	item, err := t.txn.Get(key.Bytes())
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, ds.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return item.ValueCopy(nil)
}

func (t *txn) Has(ctx context.Context, key DatastoreKey) (bool, error) {
	if !t.ds.isRunning.Load() {
		return false, ErrNotRunning
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	_, err := t.txn.Get(key.Bytes())
	switch {
	case errors.Is(err, badger.ErrKeyNotFound) || errors.Is(err, ds.ErrNotFound):
		return false, nil
	case err == nil:
		return true, nil
	default:
		return false, err
	}
}

func (t *txn) GetSize(ctx context.Context, key DatastoreKey) (int, error) {
	if !t.ds.isRunning.Load() {
		return -1, ErrNotRunning
	}
	if ctx.Err() != nil {
		return -1, ctx.Err()
	}

	item, err := t.txn.Get(key.Bytes())
	switch {
	case err == nil:
		return int(item.ValueSize()), nil
	case errors.Is(err, badger.ErrKeyNotFound):
		return -1, ds.ErrNotFound
	case errors.Is(err, ds.ErrNotFound):
		return -1, ds.ErrNotFound
	default:
		return -1, err
	}
}

func (t *txn) Delete(ctx context.Context, key DatastoreKey) error {
	if !t.ds.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return t.txn.Delete(key.Bytes())
}

func (t *txn) Increment(key DatastoreKey) (uint64, error) {
	return increment(t.txn, key.Bytes(), 1)
}

func (t *txn) Decrement(key DatastoreKey) (uint64, error) {
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
	newValue = DecodeInt64(val) + incVal
	if newValue < 0 {
		newValue = 0
	}

	return uint64(newValue), txn.Set(key, EncodeInt64(newValue))
}

func EncodeInt64(n int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(n))
	return b
}

func DecodeInt64(b []byte) int64 {
	if len(b) == 0 {
		return 0
	}
	return int64(binary.BigEndian.Uint64(b))
}

type (
	Iterator        = badger.Iterator
	IteratorOptions = badger.IteratorOptions
)

func (t *txn) NewIterator(opts IteratorOptions) (*Iterator, error) {
	if !t.ds.isRunning.Load() {
		return nil, ErrNotRunning
	}

	return t.txn.NewIterator(opts), nil
}

func (t *txn) Query(ctx context.Context, q dsq.Query) (dsq.Results, error) {
	if !t.ds.isRunning.Load() {
		return nil, ErrNotRunning
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return t.query(q)
}

func (t *txn) query(q dsq.Query) (dsq.Results, error) {
	opt := badger.DefaultIteratorOptions
	opt.PrefetchValues = !q.KeysOnly

	prefix := ds.NewKey(q.Prefix).String()
	if prefix != "/" {
		opt.Prefix = []byte(prefix + "/")
	}

	// Handle ordering
	if len(q.Orders) > 0 {
		switch q.Orders[0].(type) {
		case dsq.OrderByKey, *dsq.OrderByKey:
		// We order by key by default.
		case dsq.OrderByKeyDescending, *dsq.OrderByKeyDescending:
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
			res, err := t.query(baseQuery)
			if err != nil {
				return nil, err
			}

			// fix the query
			res = dsq.ResultsReplaceQuery(res, q)

			// Remove the parts we've already applied.
			naiveQuery := q
			naiveQuery.Prefix = ""
			naiveQuery.Filters = nil

			// Apply the rest of the query
			return dsq.NaiveQueryApply(naiveQuery, res), nil
		}
	}

	it := t.txn.NewIterator(opt)
	results := dsq.ResultsWithContext(q, func(ctx context.Context, output chan<- dsq.Result) {
		closedEarly := false
		defer func() {
			if closedEarly {
				select {
				case output <- dsq.Result{
					Error: ErrNotRunning,
				}:
				case <-ctx.Done():
				}
			}

		}()
		if !t.ds.isRunning.Load() {
			closedEarly = true
			return
		}

		// this iterator is part of an implicit transaction, so when
		// we're done we must discard the transaction. It's safe to
		// discard the txn it because it contains the iterator only.
		if t.implicit {
			defer t.txn.Discard()
		}

		defer it.Close()

		// All iterators must be started by rewinding.
		it.Rewind()

		// skip to the offset
		for skipped := 0; skipped < q.Offset && it.Valid(); it.Next() {
			// On the happy path, we have no filters and we can go
			// on our way.
			if len(q.Filters) == 0 {
				skipped++
				continue
			}

			// On the sad path, we need to apply filters before
			// counting the item as "skipped" as the offset comes
			// _after_ the filter.
			item := it.Item()

			matches := true
			check := func(value []byte) error {
				e := dsq.Entry{
					Key:   string(item.Key()),
					Value: value,
					Size:  int(item.ValueSize()), // this function is basically free
				}

				// Only calculate expirations if we need them.
				if q.ReturnExpirations {
					e.Expiration = expires(item)
				}
				matches = filter(q.Filters, e)
				return nil
			}

			// Maybe check with the value, only if we need it.
			var err error
			if q.KeysOnly {
				err = check(nil)
			} else {
				err = item.Value(check)
			}

			if err != nil {
				select {
				case output <- dsq.Result{Error: err}:
				case <-t.ds.stopChan: // datastore closing.
					closedEarly = true
					return
				case <-ctx.Done(): // client told us to close early
					return
				}
			}
			if !matches {
				skipped++
			}
		}

		for sent := 0; (q.Limit <= 0 || sent < q.Limit) && it.Valid(); it.Next() {
			item := it.Item()
			e := dsq.Entry{Key: string(item.Key())}

			// Maybe get the value
			var result dsq.Result
			if !q.KeysOnly {
				b, err := item.ValueCopy(nil)
				if err != nil {
					result = dsq.Result{Error: err}
				} else {
					e.Value = b
					e.Size = len(b)
					result = dsq.Result{Entry: e}
				}
			} else {
				e.Size = int(item.ValueSize())
				result = dsq.Result{Entry: e}
			}

			if q.ReturnExpirations {
				result.Expiration = expires(item)
			}

			// Finally, filter it (unless we're dealing with an error).
			if result.Error == nil && filter(q.Filters, e) {
				continue
			}

			select {
			case output <- result:
				sent++
			case <-t.ds.stopChan: // datastore closing.
				closedEarly = true
				return
			case <-ctx.Done(): // client told us to close early
				return
			}
		}
	})

	return results, nil
}

func (t *txn) Commit(ctx context.Context) error {
	if !t.ds.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return t.txn.Commit()
}

func (t *txn) Discard(ctx context.Context) {
	if !t.ds.isRunning.Load() {
		return
	}
	if ctx.Err() != nil {
		return
	}

	t.txn.Discard()
}

// Close is just a stub
func (t *txn) Close() error {
	return t.txn.Commit()
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
	return time.Unix(int64(item.ExpiresAt()), 0)
}

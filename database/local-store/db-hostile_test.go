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

//nolint:all
package local_store

import (
	"errors"
	"fmt"
	"testing"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	ds "github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func liveDB(t *testing.T) *DB {
	t.Helper()
	db, err := New("", DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	require.NoError(t, db.Run("user", "password"))
	t.Cleanup(db.Close)
	return db
}

// deadDB is a database that was created but never started.
func deadDB(t *testing.T) *DB {
	t.Helper()
	db, err := New("", DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	require.True(t, db.IsClosed())
	return db
}

// ---------------------------------------------------------------------------
// Lifecycle.
// ---------------------------------------------------------------------------

func TestDB_RunRequiresCredentials(t *testing.T) {
	db, err := New("", DefaultOptions().WithInMemory(true))
	require.NoError(t, err)

	assert.Error(t, db.Run("", "password"), "an unnamed database must not open")
	assert.Error(t, db.Run("user", ""), "an unencrypted database must not open")
	assert.True(t, db.IsClosed())
}

// Re-running an open database must not drop the data written so far.
func TestDB_RunIsIdempotent(t *testing.T) {
	db := liveDB(t)

	require.NoError(t, db.Set(DatabaseKey("/A/key"), []byte("value")))
	require.NoError(t, db.Run("user", "password"))

	got, err := db.Get(DatabaseKey("/A/key"))
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), got, "a redundant Run must not wipe the store")
}

func TestDB_IsFirstRunForAFreshInMemoryStore(t *testing.T) {
	db, err := New("", DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	assert.True(t, db.IsFirstRun(), "a store with no lock file has never been run")
}

func TestDB_CloseIsIdempotentAndNilSafe(t *testing.T) {
	db, err := New("", DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	require.NoError(t, db.Run("user", "password"))

	db.Close()
	assert.True(t, db.IsClosed())
	assert.NotPanics(t, db.Close, "a second close must not double-close the stop channel")

	var nilDB *DB
	assert.NotPanics(t, nilDB.Close)
	assert.True(t, nilDB.IsClosed(), "a nil database is definitionally closed")
}

// Every entry point must degrade to ErrNotRunning rather than dereferencing a
// nil badger handle when the store is down.
func TestDB_DeadStoreRefusesEveryOperation(t *testing.T) {
	db := deadDB(t)
	key := DatabaseKey("/A/key")

	assert.ErrorIs(t, db.Set(key, []byte("v")), ErrNotRunning)
	assert.ErrorIs(t, db.SetWithTTL(key, []byte("v"), time.Minute), ErrNotRunning)
	assert.ErrorIs(t, db.Delete(key), ErrNotRunning)
	assert.ErrorIs(t, db.Sync(), ErrNotRunning)

	_, err := db.Get(key)
	assert.ErrorIs(t, err, ErrNotRunning)
	_, err = db.GetExpiration(key)
	assert.ErrorIs(t, err, ErrNotRunning)
	_, err = db.GetSize(key)
	assert.ErrorIs(t, err, ErrNotRunning)
	_, err = db.NewTxn()
	assert.ErrorIs(t, err, ErrNotRunning)
	_, err = db.NewReadTxn()
	assert.ErrorIs(t, err, ErrNotRunning)
	_, err = db.NextSequence()
	assert.ErrorIs(t, err, ErrNotRunning)

	assert.NotPanics(t, db.GC)
	assert.Nil(t, db.InnerDB())
	assert.NotNil(t, db.Stats())
}

func TestDB_NilReceiverRefusesEveryOperation(t *testing.T) {
	var db *DB
	key := DatabaseKey("/A/key")

	assert.ErrorIs(t, db.Set(key, []byte("v")), ErrNotRunning)
	assert.ErrorIs(t, db.SetWithTTL(key, []byte("v"), time.Minute), ErrNotRunning)
	assert.ErrorIs(t, db.Delete(key), ErrNotRunning)

	_, err := db.Get(key)
	assert.ErrorIs(t, err, ErrNotRunning)
	_, err = db.NewTxn()
	assert.ErrorIs(t, err, ErrNotRunning)
	_, err = db.NewReadTxn()
	assert.ErrorIs(t, err, ErrNotRunning)
}

func TestDB_PathAndInnerHandleOnLiveStore(t *testing.T) {
	db := liveDB(t)

	assert.NotNil(t, db.InnerDB())
	assert.NotEmpty(t, db.Path())
	assert.NotPanics(t, db.GC)
	assert.NoError(t, db.Sync())
	assert.NotEmpty(t, db.Stats())
}

// ---------------------------------------------------------------------------
// Read-only transactions.
// ---------------------------------------------------------------------------

// A read transaction must see committed data but refuse to write — that is the
// whole reason it skips conflict tracking.
func TestNewReadTxn_ReadsButCannotWrite(t *testing.T) {
	db := liveDB(t)
	key := DatabaseKey("/A/readonly")

	require.NoError(t, db.Set(key, []byte("committed")))

	txn, err := db.NewReadTxn()
	require.NoError(t, err)
	defer txn.Rollback()

	got, err := txn.Get(key)
	require.NoError(t, err)
	assert.Equal(t, []byte("committed"), got)

	assert.Error(t, txn.Set(DatabaseKey("/A/nope"), []byte("x")),
		"a read-only transaction must refuse writes")
}

// ---------------------------------------------------------------------------
// Batch writes.
// ---------------------------------------------------------------------------

func TestBatchSet_WritesEveryEntry(t *testing.T) {
	db := liveDB(t)

	txn, err := db.NewTxn()
	require.NoError(t, err)

	items := make([]ListItem, 0, 100)
	for i := 0; i < 100; i++ {
		items = append(items, ListItem{
			Key:   fmt.Sprintf("/BATCH/key-%03d", i),
			Value: []byte(fmt.Sprintf("value-%03d", i)),
		})
	}

	require.NoError(t, txn.BatchSet(items))
	require.NoError(t, txn.Commit())

	for _, item := range items {
		got, err := db.Get(DatabaseKey(item.Key))
		require.NoErrorf(t, err, "key %s", item.Key)
		assert.Equal(t, item.Value, got)
	}
}

func TestBatchSet_EmptyInputIsANoOp(t *testing.T) {
	db := liveDB(t)

	txn, err := db.NewTxn()
	require.NoError(t, err)
	defer txn.Rollback()

	assert.NoError(t, txn.BatchSet(nil))
	assert.NoError(t, txn.BatchSet([]ListItem{}))
}

// A batch larger than one badger transaction must still land in full — an
// archive import writes far more than a single transaction can hold.
func TestBatchSet_SplitsOversizedTransactions(t *testing.T) {
	db := liveDB(t)

	txn, err := db.NewTxn()
	require.NoError(t, err)

	payload := make([]byte, 4*1024)
	const n = 700

	items := make([]ListItem, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, ListItem{
			Key:   fmt.Sprintf("/BIGBATCH/key-%05d", i),
			Value: payload,
		})
	}

	require.NoError(t, txn.BatchSet(items), "an oversized batch must be split, not rejected")
	_ = txn.Commit()

	// Spot-check both ends of the range to prove nothing was silently dropped.
	for _, idx := range []int{0, n / 2, n - 1} {
		_, err := db.Get(DatabaseKey(fmt.Sprintf("/BIGBATCH/key-%05d", idx)))
		assert.NoErrorf(t, err, "entry %d must have been written", idx)
	}
}

// ---------------------------------------------------------------------------
// Sequences.
// ---------------------------------------------------------------------------

func TestNextSequence_IsStrictlyIncreasing(t *testing.T) {
	db := liveDB(t)

	seen := make(map[uint64]struct{}, 50)
	var last uint64
	for i := 0; i < 50; i++ {
		n, err := db.NextSequence()
		require.NoError(t, err)

		_, dup := seen[n]
		assert.Falsef(t, dup, "sequence %d handed out twice", n)
		seen[n] = struct{}{}

		if i > 0 {
			assert.Greaterf(t, n, last, "sequence must increase (step %d)", i)
		}
		last = n
	}
}

// ---------------------------------------------------------------------------
// Error mapping.
// ---------------------------------------------------------------------------

func TestNotFoundErrorMapping(t *testing.T) {
	assert.False(t, IsNotFoundError(nil))
	assert.True(t, IsNotFoundError(badger.ErrKeyNotFound))
	assert.True(t, IsNotFoundError(ds.ErrNotFound))
	assert.True(t, IsNotFoundError(fmt.Errorf("wrapped: %w", badger.ErrKeyNotFound)))
	assert.False(t, IsNotFoundError(errors.New("disk on fire")))

	assert.NoError(t, ToDatastoreErrNotFound(nil))
	assert.ErrorIs(t, ToDatastoreErrNotFound(badger.ErrKeyNotFound), ds.ErrNotFound)
	assert.ErrorIs(t, ToDatastoreErrNotFound(ds.ErrNotFound), ds.ErrNotFound)
	assert.ErrorIs(t, ToDatastoreErrNotFound(fmt.Errorf("wrapped: %w", badger.ErrKeyNotFound)), ds.ErrNotFound)

	// A genuine failure must NOT be laundered into "not found" — that would
	// make a broken disk look like an empty timeline.
	real := errors.New("disk on fire")
	assert.ErrorIs(t, ToDatastoreErrNotFound(real), real)
}

func TestDBError_Sentinels(t *testing.T) {
	const e DBError = "boom"
	assert.Equal(t, "boom", e.Error())
	assert.True(t, errors.Is(e, e))
	assert.True(t, errors.Is(fmt.Errorf("wrapped: %w", ErrNotRunning), ErrNotRunning))
}

// ---------------------------------------------------------------------------
// Key building.
// ---------------------------------------------------------------------------

// The writer/reader segments are currently inert; pinning that keeps a future
// re-enable from silently changing every stored key's shape.
func TestPrefixBuilder_WriterAndReaderSegmentsAreInert(t *testing.T) {
	pb := &PrefixBuilder{}

	assert.Same(t, pb, pb.AddWriterId("writer"))
	assert.Same(t, pb, pb.AddReaderId("reader"))
}

func TestDatabaseKey_DatastoreKeyRoundTrip(t *testing.T) {
	key := NewPrefixBuilder("/TEST").AddRootID("root").AddParentId("child").Build()

	dsKey := key.DatastoreKey()
	assert.Contains(t, dsKey.String(), "TEST")
	assert.NotEmpty(t, key.Bytes())
	assert.Equal(t, key.String(), string(key.Bytes()))
}

// ---------------------------------------------------------------------------
// Expiry and size on a live store.
// ---------------------------------------------------------------------------

func TestGetExpirationAndSize(t *testing.T) {
	db := liveDB(t)

	plain := DatabaseKey("/A/plain")
	require.NoError(t, db.Set(plain, []byte("12345")))

	size, err := db.GetSize(plain)
	require.NoError(t, err)
	assert.Equal(t, int64(5), size)

	exp, err := db.GetExpiration(plain)
	require.NoError(t, err)
	assert.Zero(t, exp, "a key without a TTL must not claim an expiry")

	expiring := DatabaseKey("/A/expiring")
	require.NoError(t, db.SetWithTTL(expiring, []byte("v"), time.Hour))
	exp, err = db.GetExpiration(expiring)
	require.NoError(t, err)
	assert.Positive(t, exp)

	missing := DatabaseKey("/A/missing")
	_, err = db.GetSize(missing)
	assert.True(t, IsNotFoundError(err))
	_, err = db.GetExpiration(missing)
	assert.True(t, IsNotFoundError(err))
	_, err = db.Get(missing)
	assert.True(t, IsNotFoundError(err))
}

// A key written with a TTL already in the past must not be readable.
func TestSetWithTTL_AlreadyExpiredIsInvisible(t *testing.T) {
	db := liveDB(t)
	key := DatabaseKey("/A/stale")

	require.NoError(t, db.SetWithTTL(key, []byte("v"), -time.Hour))

	_, err := db.Get(key)
	assert.True(t, IsNotFoundError(err), "an already-expired write must not be served")
}

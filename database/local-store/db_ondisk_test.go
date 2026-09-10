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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNewOnDiskLifecycle drives the on-disk branch of New/Run/Close that the
// in-memory suite never reaches: the first-run lock file, the background GC
// loop, a real Sync, and reopening the same directory.
func TestNewOnDiskLifecycle(t *testing.T) {
	dir := t.TempDir()

	db, err := New(dir, DefaultOptions().
		WithIntervalGC(10*time.Millisecond).
		WithSleepGC(time.Millisecond).
		WithDiscardRatioGC(0.5))
	require.NoError(t, err)
	require.True(t, db.IsFirstRun())
	require.NotEmpty(t, db.Path())

	require.NoError(t, db.Run("user", "pass"))
	require.False(t, db.IsClosed())
	require.False(t, db.IsFirstRun(), "the lock file marks the store as already initialised")

	key := NewPrefixBuilder("/DISK").AddRootID("k").Build()
	require.NoError(t, db.Set(key, []byte("v")))
	require.NoError(t, db.Sync())

	got, err := db.Get(key)
	require.NoError(t, err)
	require.Equal(t, []byte("v"), got)

	// let the GC ticker fire at least once
	time.Sleep(40 * time.Millisecond)
	db.GC()

	require.NotNil(t, db.InnerDB())
	require.NotEmpty(t, db.Stats())

	db.Close()
	require.True(t, db.IsClosed())

	// a second store over the same directory sees the first-run flag
	reopened, err := New(dir, DefaultOptions())
	require.NoError(t, err)
	require.False(t, reopened.IsFirstRun())

	require.NoError(t, reopened.Run("user", "pass"))
	defer reopened.Close()

	got, err = reopened.Get(key)
	require.NoError(t, err)
	require.Equal(t, []byte("v"), got)
}

func TestRunRejectsWrongPassword(t *testing.T) {
	dir := t.TempDir()

	db, err := New(dir, DefaultOptions())
	require.NoError(t, err)
	require.NoError(t, db.Run("user", "pass"))
	db.Close()

	wrong, err := New(dir, DefaultOptions())
	require.NoError(t, err)
	require.ErrorIs(t, wrong.Run("user", "not-the-password"), ErrWrongPassword)
}

func TestNewFillsInZeroGCOptions(t *testing.T) {
	db, err := New("", &Options{isInMemory: true})
	require.NoError(t, err)
	require.Equal(t, defaultIntervalGC, db.intervalGC)
	require.Equal(t, defaultDiscardRatioGC, db.discardRatioGC)
	require.Equal(t, defaultSleepGC, db.sleepGC)
}

func TestGCOnDeadStore(t *testing.T) {
	db, err := New("", DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	// not running yet: GC returns rather than touching a nil badger handle
	db.GC()

	var nilDB *DB
	require.Panics(t, func() { nilDB.GC() })
	require.Panics(t, func() { nilDB.InnerDB() })
	require.Empty(t, nilDB.Path())
	require.True(t, nilDB.IsClosed())
}

func TestPageLimitClamps(t *testing.T) {
	require.Equal(t, defaultLimit, *pageLimit(nil))

	zero := uint64(0)
	require.Equal(t, defaultLimit, *pageLimit(&zero))

	over := MaxPageLimit + 1
	require.Equal(t, MaxPageLimit, *pageLimit(&over))

	exact := uint64(7)
	require.Equal(t, exact, *pageLimit(&exact))
}

func TestTxnBatchSet(t *testing.T) {
	db, err := New("", DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	require.NoError(t, db.Run("user", "pass"))
	defer db.Close()

	txn, err := db.NewTxn()
	require.NoError(t, err)

	items := make([]ListItem, 0, 3)
	for _, id := range []string{"a", "b", "c"} {
		items = append(items, ListItem{
			Key:   NewPrefixBuilder("/BATCH").AddRootID(id).Build().String(),
			Value: []byte(id),
		})
	}
	require.NoError(t, txn.BatchSet(items))
	require.NoError(t, txn.Commit())

	read, err := db.NewReadTxn()
	require.NoError(t, err)
	defer read.Rollback()

	got, err := read.BatchGet(
		DatabaseKey(items[0].Key),
		DatabaseKey(items[1].Key),
		NewPrefixBuilder("/BATCH").AddRootID("absent").Build(),
	)
	require.NoError(t, err)
	require.Len(t, got, 2, "missing keys are skipped, not reported")

	require.NoError(t, txn.BatchSet(nil))
}

func TestTxnMissingKeyErrors(t *testing.T) {
	db, err := New("", DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	require.NoError(t, db.Run("user", "pass"))
	defer db.Close()

	txn, err := db.NewTxn()
	require.NoError(t, err)
	defer txn.Rollback()

	absent := NewPrefixBuilder("/MISS").AddRootID("nope").Build()

	_, err = txn.Get(absent)
	require.True(t, IsNotFoundError(err))

	_, err = txn.GetExpiration(absent)
	require.True(t, IsNotFoundError(err))

	_, err = db.GetExpiration(absent)
	require.True(t, IsNotFoundError(err))

	_, err = db.GetSize(absent)
	require.True(t, IsNotFoundError(err))

	// deleting a key that was never written is not an error
	require.NoError(t, txn.Delete(absent))
	require.NoError(t, db.Delete(absent))
}

func TestNewPrefixBuilderRejectsBadNamespace(t *testing.T) {
	require.Panics(t, func() { NewPrefixBuilder("") })
	require.Panics(t, func() { NewPrefixBuilder("NO-LEADING-SLASH") })
	require.NotPanics(t, func() { NewPrefixBuilder("/OK") })
}

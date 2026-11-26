//nolint:all
package local_store

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func runningDB(t *testing.T) *DB {
	t.Helper()
	db, err := New("", DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	require.NoError(t, db.Run("user", "pass"))
	t.Cleanup(db.Close)
	return db
}

func TestRunRefusesADirectoryHeldByAnotherStore(t *testing.T) {
	dir := t.TempDir()

	held, err := New(dir, DefaultOptions())
	require.NoError(t, err)
	require.NoError(t, held.Run("user", "pass"))
	t.Cleanup(held.Close)

	second, err := New(dir, DefaultOptions())
	require.NoError(t, err)

	err = second.Run("user", "pass")
	require.Error(t, err, "badger holds a directory lock; a second store must not steal it")
	require.NotErrorIs(t, err, ErrWrongPassword)
}

func TestNilReceiverGuards(t *testing.T) {
	var db *DB

	_, err := db.GetExpiration("key")
	require.ErrorIs(t, err, ErrNotRunning)

	_, err = db.GetSize("key")
	require.ErrorIs(t, err, ErrNotRunning)

	_, err = db.NextSequence()
	require.ErrorIs(t, err, ErrNotRunning)

	require.NotPanics(t, db.Close)
}

func TestCloseOnAnUnstartedStore(t *testing.T) {
	db, err := New("", DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	// never Run: Close must return before touching badger
	require.NotPanics(t, db.Close)
}

func TestIterateKeysPropagatesHandlerFailures(t *testing.T) {
	db := runningDB(t)

	prefix := NewPrefixBuilder("/ITER").AddRootID("root").Build()
	txn, err := db.NewTxn()
	require.NoError(t, err)
	require.NoError(t, txn.Set(NewPrefixBuilder("/ITER").AddRootID("root").AddParentId("a").Build(), []byte("a")))
	require.NoError(t, txn.Commit())

	handlerErr := errors.New("handler gave up")

	read, err := db.NewReadTxn()
	require.NoError(t, err)
	defer read.Rollback()

	require.ErrorIs(t, read.IterateKeys(prefix, func(string) error { return handlerErr }), handlerErr)
	require.ErrorIs(t, read.ReverseIterateKeys(prefix, func(string) error { return handlerErr }), handlerErr)
}

func TestListingAtTheEndCursor(t *testing.T) {
	db := runningDB(t)
	prefix := NewPrefixBuilder("/PAGED").AddRootID("root").Build()

	txn, err := db.NewTxn()
	require.NoError(t, err)
	defer txn.Rollback()

	end := EndCursor

	keys, cur, err := txn.ListKeys(prefix, nil, &end)
	require.NoError(t, err)
	require.Empty(t, keys)
	require.Equal(t, EndCursor, cur)

	items, cur, err := txn.List(prefix, nil, &end)
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, EndCursor, cur)
}

func TestListRejectsAFixedKeyPrefix(t *testing.T) {
	db := runningDB(t)

	fixed := NewPrefixBuilder("/FIXED").
		AddRootID("root").
		AddRange(FixedRangeKey).
		Build()

	txn, err := db.NewTxn()
	require.NoError(t, err)
	defer txn.Rollback()

	_, _, err = txn.List(fixed, nil, nil)
	require.Error(t, err)

	_, _, err = txn.ListKeys(fixed, nil, nil)
	require.Error(t, err)
}

func TestListSkipsFixedKeysUnderAScannedPrefix(t *testing.T) {
	db := runningDB(t)
	root := NewPrefixBuilder("/MIXED").AddRootID("root").Build()

	txn, err := db.NewTxn()
	require.NoError(t, err)
	require.NoError(t, txn.Set(
		NewPrefixBuilder("/MIXED").AddRootID("root").AddRange(FixedRangeKey).AddParentId("a").Build(),
		[]byte("pointer"),
	))
	require.NoError(t, txn.Set(
		NewPrefixBuilder("/MIXED").AddRootID("root").AddParentId("b").Build(),
		[]byte("value"),
	))
	require.NoError(t, txn.Commit())

	read, err := db.NewReadTxn()
	require.NoError(t, err)
	defer read.Rollback()

	items, _, err := read.List(root, nil, nil)
	require.NoError(t, err)
	for _, item := range items {
		require.NotContains(t, item.Key, FixedKey, "fixed pointer rows are skipped by a scan")
	}
	require.Len(t, items, 1)
}

func TestSimultaneousClosesCloseOnce(t *testing.T) {
	db, err := New("", DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	require.NoError(t, db.Run("user", "password"))

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.NotPanics(t, db.Close)
		}()
	}
	wg.Wait()
	require.True(t, db.IsClosed())
}

func TestMaintenanceCallsAfterClose(t *testing.T) {
	db, err := New(t.TempDir(), DefaultOptions())
	require.NoError(t, err)
	require.NoError(t, db.Run("user", "password"))
	db.Close()

	require.ErrorIs(t, db.Sync(), ErrNotRunning)
	_, err = db.NextSequence()
	require.ErrorIs(t, err, ErrNotRunning)
	require.Empty(t, db.Stats())
	require.NotPanics(t, db.GC)
}

func TestLoginAfterLogoutWhileCollecting(t *testing.T) {
	db, err := New(t.TempDir(), DefaultOptions().
		WithIntervalGC(time.Millisecond).
		WithSleepGC(time.Hour).
		WithDiscardRatioGC(0.5))
	require.NoError(t, err)

	for range 20 {
		require.NoError(t, db.Run("user", "password"))
		require.NoError(t, db.Set(DatabaseKey("/GC/key"), make([]byte, 2048)))
		time.Sleep(5 * time.Millisecond)

		start := time.Now()
		db.Close()
		require.Less(t, time.Since(start), time.Minute, "the close must not sit out the collector's pause")
	}
}

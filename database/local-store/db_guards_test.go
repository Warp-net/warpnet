//nolint:all
package local_store

import (
	"errors"
	"fmt"
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

// A logout closes the store while paired devices keep sending requests: every
// operation must finish, refused or not, instead of parking inside badger.
func TestCloseUnderLoadLeavesNoOperationHanging(t *testing.T) {
	const (
		rounds  = 5
		workers = 256
	)
	for round := range rounds {
		db, err := New("", DefaultOptions().WithInMemory(true))
		require.NoError(t, err)
		require.NoError(t, db.Run("user", "password"))

		var wg sync.WaitGroup
		for worker := range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { _ = recover() }() // badger panics on a txn caught by the close
				key := DatabaseKey(fmt.Sprintf("/LOAD/%d", worker))
				for {
					if err := db.Set(key, []byte("v")); errors.Is(err, ErrNotRunning) {
						return
					}
					txn, err := db.NewTxn()
					if err != nil {
						return
					}
					_, _ = txn.Get(key)
					_ = txn.Set(key, []byte("w"))
					_ = txn.Commit()
					txn.Rollback()
				}
			}()
		}

		time.Sleep(20 * time.Millisecond)
		db.Close()

		finished := make(chan struct{})
		go func() {
			wg.Wait()
			close(finished)
		}()
		select {
		case <-finished:
		case <-time.After(30 * time.Second):
			t.Fatalf("round %d: operations hang after the store closed", round)
		}
	}
}

// More open transactions than badger's watermark channel holds: finishing them
// on an already closed badger would block forever.
func TestCloseLetsOpenTransactionsFinish(t *testing.T) {
	db, err := New("", DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	require.NoError(t, db.Run("user", "password"))

	txns := make([]WarpTransactioner, 0, 256)
	for range cap(txns) {
		txn, err := db.NewTxn()
		require.NoError(t, err)
		txns = append(txns, txn)
	}

	closed := make(chan struct{})
	go func() {
		db.Close()
		close(closed)
	}()
	require.Eventually(t, db.IsClosed, 5*time.Second, time.Millisecond)

	_, err = db.NewTxn()
	require.ErrorIs(t, err, ErrNotRunning, "a closing store takes no new work")
	_, err = db.Get(DatabaseKey("/LOAD/key"))
	require.ErrorIs(t, err, ErrNotRunning, "and refuses reads made inside an open transaction")

	finished := make(chan struct{})
	go func() {
		for _, txn := range txns {
			txn.Rollback()
		}
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(closeDrainTimeout + 10*time.Second):
		t.Fatal("open transactions hang on the closing store")
	}
	<-closed
}

func TestCloseDoesNotWaitForeverForALeakedTransaction(t *testing.T) {
	db, err := New("", DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	require.NoError(t, db.Run("user", "password"))

	leaked, err := db.NewTxn()
	require.NoError(t, err)

	closed := make(chan struct{})
	go func() {
		db.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(closeDrainTimeout + 10*time.Second):
		t.Fatal("a leaked transaction blocks the close")
	}
	leaked.Rollback()
}

func TestCloseWithoutOpenTransactionsIsImmediate(t *testing.T) {
	db := runningDB(t)
	txn, err := db.NewTxn()
	require.NoError(t, err)
	require.NoError(t, txn.Set(DatabaseKey("/CLOSE/key"), []byte("v")))
	require.NoError(t, txn.Commit())

	start := time.Now()
	db.Close()
	require.Less(t, time.Since(start), closeDrainTimeout/2)
}

func TestFinishingATransactionTwiceCountsOnce(t *testing.T) {
	db := runningDB(t)

	committed, err := db.NewTxn()
	require.NoError(t, err)
	require.NoError(t, committed.Commit())
	committed.Rollback()
	require.NoError(t, committed.Discard())

	read, err := db.NewReadTxn()
	require.NoError(t, err)
	read.Rollback()
	read.Rollback()

	require.Zero(t, db.openTxns.Load())

	open, err := db.NewTxn()
	require.NoError(t, err)
	require.EqualValues(t, 1, db.openTxns.Load(), "an open transaction must stay visible to Close")
	open.Rollback()
}

func TestCommitDuringCloseIsDurable(t *testing.T) {
	db, err := New(t.TempDir(), DefaultOptions())
	require.NoError(t, err)
	require.NoError(t, db.Run("user", "password"))
	key := DatabaseKey("/CLOSE/late")

	txn, err := db.NewTxn()
	require.NoError(t, err)
	require.NoError(t, txn.Set(key, []byte("kept")))

	closed := make(chan struct{})
	go func() {
		db.Close()
		close(closed)
	}()
	require.Eventually(t, db.IsClosed, 5*time.Second, time.Millisecond)

	require.NoError(t, txn.Commit())
	<-closed

	require.NoError(t, db.Run("user", "password"))
	t.Cleanup(db.Close)
	got, err := db.Get(key)
	require.NoError(t, err)
	require.Equal(t, []byte("kept"), got)
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

func TestStoreServesAgainAfterLogoutAndLogin(t *testing.T) {
	db, err := New(t.TempDir(), DefaultOptions())
	require.NoError(t, err)
	require.NoError(t, db.Run("user", "password"))
	t.Cleanup(db.Close)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for worker := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { _ = recover() }() // badger panics on a txn caught by the close
			key := DatabaseKey(fmt.Sprintf("/RELOGIN/%d", worker))
			for {
				select {
				case <-stop:
					return
				default:
				}
				txn, err := db.NewTxn()
				if err != nil {
					continue
				}
				_ = txn.Set(key, []byte("v"))
				_ = txn.Commit()
				txn.Rollback()
			}
		}()
	}

	for range 3 {
		time.Sleep(10 * time.Millisecond)
		db.Close()
		require.NoError(t, db.Run("user", "password"))
	}
	close(stop)

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(30 * time.Second):
		t.Fatal("operations hang across a logout and login")
	}

	key := DatabaseKey("/RELOGIN/after")
	require.NoError(t, db.Set(key, []byte("v")))
	got, err := db.Get(key)
	require.NoError(t, err)
	require.Equal(t, []byte("v"), got)
}

func TestMaintenanceCallsSurviveAClose(t *testing.T) {
	db, err := New(t.TempDir(), DefaultOptions())
	require.NoError(t, err)
	require.NoError(t, db.Run("user", "password"))

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !db.IsClosed() {
				_ = db.Sync()
				_, _ = db.NextSequence()
				_ = db.InnerDB()
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	db.Close()
	wg.Wait()

	require.ErrorIs(t, db.Sync(), ErrNotRunning)
	require.Empty(t, db.Stats())
	require.NotPanics(t, db.GC)
}

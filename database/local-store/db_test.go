package local_store

import (
	"errors"
	"fmt"
	badger "github.com/dgraph-io/badger/v4"
	ds "github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
	"testing"
	"time"
)

type DBTestSuite struct {
	suite.Suite

	db *DB
}

func (s *DBTestSuite) SetupSuite() {
	var err error
	s.db, err = New("", DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	err = s.db.Run("test", "test")
	s.Require().NoError(err)
}

func (s *DBTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *DBTestSuite) TestDefaultOptions() {
	opts := DefaultOptions()
	assert.NotNil(s.T(), opts)
	assert.Equal(s.T(), defaultDiscardRatioGC, opts.discardRatioGC)
	assert.Equal(s.T(), defaultIntervalGC, opts.intervalGC)
	assert.Equal(s.T(), defaultSleepGC, opts.sleepGC)
	assert.False(s.T(), opts.isInMemory)
}

func (s *DBTestSuite) TestOptionsChaining() {
	opts := DefaultOptions().
		WithDiscardRatioGC(0.7).
		WithIntervalGC(2 * time.Hour).
		WithSleepGC(2 * time.Second).
		WithInMemory(true)
	assert.Equal(s.T(), 0.7, opts.discardRatioGC)
	assert.Equal(s.T(), 2*time.Hour, opts.intervalGC)
	assert.Equal(s.T(), 2*time.Second, opts.sleepGC)
	assert.True(s.T(), opts.isInMemory)
}

func (s *DBTestSuite) TestDBError() {
	err := DBError("test error")
	assert.Equal(s.T(), "test error", err.Error())
}

func (s *DBTestSuite) TestSetAndGet() {
	key := DatabaseKey("/test/key1")
	value := []byte("value1")

	err := s.db.Set(key, value)
	assert.NoError(s.T(), err)

	result, err := s.db.Get(key)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), value, result)
}

func (s *DBTestSuite) TestGet_NotFound() {
	_, err := s.db.Get(DatabaseKey("/nonexistent"))
	assert.Error(s.T(), err)
	assert.True(s.T(), IsNotFoundError(err))
}

func (s *DBTestSuite) TestSetWithTTL() {
	key := DatabaseKey("/test/ttl-key")
	value := []byte("ttl-value")

	err := s.db.SetWithTTL(key, value, time.Hour)
	assert.NoError(s.T(), err)

	result, err := s.db.Get(key)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), value, result)
}

func (s *DBTestSuite) TestDelete() {
	key := DatabaseKey("/test/delete-key")
	err := s.db.Set(key, []byte("to-delete"))
	assert.NoError(s.T(), err)

	err = s.db.Delete(key)
	assert.NoError(s.T(), err)

	_, err = s.db.Get(key)
	assert.True(s.T(), IsNotFoundError(err))
}

func (s *DBTestSuite) TestGetExpiration() {
	key := DatabaseKey("/test/expiration-key")
	err := s.db.SetWithTTL(key, []byte("data"), time.Hour)
	assert.NoError(s.T(), err)

	exp, err := s.db.GetExpiration(key)
	assert.NoError(s.T(), err)
	assert.True(s.T(), exp > 0)
}

func (s *DBTestSuite) TestGetSize() {
	key := DatabaseKey("/test/size-key")
	err := s.db.Set(key, []byte("hello"))
	assert.NoError(s.T(), err)

	size, err := s.db.GetSize(key)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(5), size)
}

func (s *DBTestSuite) TestIsClosed() {
	assert.False(s.T(), s.db.IsClosed())
}

func (s *DBTestSuite) TestSync() {
	err := s.db.Sync()
	assert.NoError(s.T(), err) // in-memory is a no-op
}

func (s *DBTestSuite) TestNewTxn() {
	txn, err := s.db.NewTxn()
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), txn)
	txn.Rollback()
}

func (s *DBTestSuite) TestTxn_SetAndGet() {
	txn, err := s.db.NewTxn()
	assert.NoError(s.T(), err)
	defer txn.Rollback()

	key := DatabaseKey("/txn/key1")
	err = txn.Set(key, []byte("txn-value"))
	assert.NoError(s.T(), err)

	val, err := txn.Get(key)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), []byte("txn-value"), val)

	err = txn.Commit()
	assert.NoError(s.T(), err)
}

func (s *DBTestSuite) TestTxn_Delete() {
	txn1, err := s.db.NewTxn()
	assert.NoError(s.T(), err)
	key := DatabaseKey("/txn/delete-key")
	err = txn1.Set(key, []byte("val"))
	assert.NoError(s.T(), err)
	err = txn1.Commit()
	assert.NoError(s.T(), err)

	txn2, err := s.db.NewTxn()
	assert.NoError(s.T(), err)
	defer txn2.Rollback()
	err = txn2.Delete(key)
	assert.NoError(s.T(), err)
	err = txn2.Commit()
	assert.NoError(s.T(), err)
}

func (s *DBTestSuite) TestTxn_Increment() {
	txn, _ := s.db.NewTxn()
	defer txn.Rollback()

	key := DatabaseKey("/txn/counter")
	val, err := txn.Increment(key)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), uint64(1), val)

	val, err = txn.Increment(key)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), uint64(2), val)

	_ = txn.Commit()
}

func (s *DBTestSuite) TestTxn_Decrement() {
	txn1, _ := s.db.NewTxn()
	key := DatabaseKey("/txn/decr-counter")
	_, _ = txn1.Increment(key)
	_, _ = txn1.Increment(key)
	_ = txn1.Commit()

	txn2, _ := s.db.NewTxn()
	defer txn2.Rollback()
	val, err := txn2.Decrement(key)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), uint64(1), val)
	_ = txn2.Commit()
}

func (s *DBTestSuite) TestTxn_Decrement_AtZero() {
	txn, _ := s.db.NewTxn()
	defer txn.Rollback()

	key := DatabaseKey("/txn/decr-zero")
	val, err := txn.Decrement(key)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), uint64(0), val)
	_ = txn.Commit()
}

func (s *DBTestSuite) TestTxn_SetWithTTL() {
	txn, _ := s.db.NewTxn()
	defer txn.Rollback()

	key := DatabaseKey("/txn/ttl")
	err := txn.SetWithTTL(key, []byte("ttl-val"), time.Hour)
	assert.NoError(s.T(), err)

	exp, err := txn.GetExpiration(key)
	assert.NoError(s.T(), err)
	assert.True(s.T(), exp > 0)

	_ = txn.Commit()
}

func (s *DBTestSuite) TestTxn_List() {
	txn, _ := s.db.NewTxn()
	_ = txn.Set(DatabaseKey("/list/item1"), []byte("a"))
	_ = txn.Set(DatabaseKey("/list/item2"), []byte("b"))
	_ = txn.Commit()

	txn2, _ := s.db.NewTxn()
	defer txn2.Rollback()
	items, cursor, err := txn2.List(DatabaseKey("/list/"), nil, nil)
	assert.NoError(s.T(), err)
	assert.True(s.T(), len(items) >= 2)
	assert.NotEmpty(s.T(), cursor)
	_ = txn2.Commit()
}

func (s *DBTestSuite) TestTxn_ReverseList() {
	txn, _ := s.db.NewTxn()
	_ = txn.Set(DatabaseKey("/revlist/item1"), []byte("a"))
	_ = txn.Set(DatabaseKey("/revlist/item2"), []byte("b"))
	_ = txn.Set(DatabaseKey("/revlist/item3"), []byte("c"))
	_ = txn.Commit()

	txn2, _ := s.db.NewTxn()
	defer txn2.Rollback()
	fwd, _, err := txn2.List(DatabaseKey("/revlist/"), nil, nil)
	assert.NoError(s.T(), err)
	rev, cursor, err := txn2.ReverseList(DatabaseKey("/revlist/"), nil, nil)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), rev, 3)
	assert.Equal(s.T(), endCursor, cursor)
	assert.Equal(s.T(), fwd[0].Key, rev[len(rev)-1].Key)
	assert.Equal(s.T(), fwd[len(fwd)-1].Key, rev[0].Key)
	_ = txn2.Commit()
}

func (s *DBTestSuite) TestTxn_ListKeys() {
	txn, _ := s.db.NewTxn()
	_ = txn.Set(DatabaseKey("/listkeys/item1"), []byte("a"))
	_ = txn.Set(DatabaseKey("/listkeys/item2"), []byte("b"))
	_ = txn.Commit()

	txn2, _ := s.db.NewTxn()
	defer txn2.Rollback()
	keys, cursor, err := txn2.ListKeys(DatabaseKey("/listkeys/"), nil, nil)
	assert.NoError(s.T(), err)
	assert.True(s.T(), len(keys) >= 2)
	assert.NotEmpty(s.T(), cursor)
	_ = txn2.Commit()
}

func (s *DBTestSuite) TestTxn_BatchGet() {
	txn, _ := s.db.NewTxn()
	_ = txn.Set(DatabaseKey("/batchget/k1"), []byte("v1"))
	_ = txn.Set(DatabaseKey("/batchget/k2"), []byte("v2"))
	_ = txn.Commit()

	txn2, _ := s.db.NewTxn()
	defer txn2.Rollback()
	items, err := txn2.BatchGet(DatabaseKey("/batchget/k1"), DatabaseKey("/batchget/k2"), DatabaseKey("/batchget/missing"))
	assert.NoError(s.T(), err)
	assert.Len(s.T(), items, 2)
}

func (s *DBTestSuite) TestTxn_Discard() {
	txn, _ := s.db.NewTxn()
	err := txn.Discard()
	assert.NoError(s.T(), err)
}

func (s *DBTestSuite) TestNextSequence() {
	seq, err := s.db.NextSequence()
	assert.NoError(s.T(), err)
	assert.True(s.T(), seq > 0)
}

func (s *DBTestSuite) TestStats() {
	stats := s.db.Stats()
	assert.NotNil(s.T(), stats)
	assert.Contains(s.T(), stats, "size")
	assert.Contains(s.T(), stats, "cache_hit_miss")
	assert.Contains(s.T(), stats, "max_version")
}

func (s *DBTestSuite) TestIsNotFoundError() {
	assert.False(s.T(), IsNotFoundError(nil))
	assert.False(s.T(), IsNotFoundError(DBError("other")))
}

func (s *DBTestSuite) TestNilDB_Operations() {
	var db *DB
	assert.True(s.T(), db.IsClosed())
	assert.Equal(s.T(), "", db.Path())

	err := db.Set(DatabaseKey("/k"), []byte("v"))
	assert.Error(s.T(), err)

	_, err = db.Get(DatabaseKey("/k"))
	assert.Error(s.T(), err)

	err = db.Delete(DatabaseKey("/k"))
	assert.Error(s.T(), err)

	_, err = db.NewTxn()
	assert.Error(s.T(), err)

	_, err = db.NextSequence()
	assert.Error(s.T(), err)
}

func (s *DBTestSuite) TestRun_EmptyCredentials() {
	db, err := New("", DefaultOptions().WithInMemory(true))
	assert.NoError(s.T(), err)
	defer db.Close()

	err = db.Run("", "")
	assert.Error(s.T(), err)
}

func TestDBTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(DBTestSuite))
}

// TestReopenAfterClose covers the long-lived-node lifecycle the remote node
// relies on: a logout closes the database and a later login reopens it on the
// same handle, so data must survive the cycle and a second Close (and a
// redundant one) must not double-close the lifecycle channel.
func TestReopenAfterClose(t *testing.T) {
	db, err := New(t.TempDir(), DefaultOptions())
	assert.NoError(t, err)

	const user, pass = "user", "pass"
	key := DatabaseKey("/TEST/key")

	assert.NoError(t, db.Run(user, pass))
	assert.NoError(t, db.Set(key, []byte("value")))
	db.Close()
	assert.True(t, db.IsClosed())

	// reopen on the same handle — data persists and the node's repos recover
	assert.NoError(t, db.Run(user, pass))
	got, err := db.Get(key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("value"), got)

	// second Close targets the channel recreated by the second Run, not the
	// already-closed first one
	assert.NotPanics(t, db.Close)
	// a redundant close (process-exit defer after a logout) is a no-op
	assert.NotPanics(t, db.Close)
}
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

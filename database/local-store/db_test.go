package local_store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
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
	txn1, _ := s.db.NewTxn()
	key := DatabaseKey("/txn/delete-key")
	_ = txn1.Set(key, []byte("val"))
	_ = txn1.Commit()

	txn2, _ := s.db.NewTxn()
	defer txn2.Rollback()
	err := txn2.Delete(key)
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

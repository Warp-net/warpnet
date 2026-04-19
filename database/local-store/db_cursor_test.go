package local_store

import (
	"fmt"
	"reflect"
	"testing"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := New("", DefaultOptions().WithInMemory(true))
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	if err := db.Run("user", "pass"); err != nil {
		t.Fatalf("run db: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

func seedKeys(t *testing.T, db *DB, keys []string) {
	t.Helper()
	txn, err := db.NewTxn()
	if err != nil {
		t.Fatalf("new txn: %v", err)
	}
	for i, k := range keys {
		if err := txn.Set(DatabaseKey(k), fmt.Appendf(nil, "val-%d", i)); err != nil {
			txn.Rollback()
			t.Fatalf("set %s: %v", k, err)
		}
	}
	if err := txn.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestListCursorPagination(t *testing.T) {
	db := newTestDB(t)
	seedKeys(t, db, []string{
		"/feed/none/001",
		"/feed/none/002",
		"/feed/none/003",
		"/feed/none/004",
		"/feed-other/none/001",
	})

	limit := uint64(2)
	txn, _ := db.NewTxn()
	items, cursor, err := txn.List(DatabaseKey("/feed/none"), &limit, nil)
	if err != nil {
		t.Fatalf("list page1: %v", err)
	}
	if got := []string{items[0].Key, items[1].Key}; !reflect.DeepEqual(got, []string{"/feed/none/001", "/feed/none/002"}) {
		t.Fatalf("page1 keys mismatch: %v", got)
	}
	if cursor != "/feed/none/003" {
		t.Fatalf("page1 cursor mismatch: %q", cursor)
	}

	items, cursor, err = txn.List(DatabaseKey("/feed/none"), &limit, &cursor)
	if err != nil {
		t.Fatalf("list page2: %v", err)
	}
	if got := []string{items[0].Key, items[1].Key}; !reflect.DeepEqual(got, []string{"/feed/none/003", "/feed/none/004"}) {
		t.Fatalf("page2 keys mismatch: %v", got)
	}
	if cursor != endCursor {
		t.Fatalf("page2 cursor mismatch: %q", cursor)
	}

	items, cursor, err = txn.List(DatabaseKey("/feed/none"), &limit, &cursor)
	if err != nil {
		t.Fatalf("list page3: %v", err)
	}
	if len(items) != 0 || cursor != endCursor {
		t.Fatalf("page3 expected empty+end, got len=%d cursor=%q", len(items), cursor)
	}
}

func TestListKeysCursorPagination(t *testing.T) {
	db := newTestDB(t)
	seedKeys(t, db, []string{
		"/room/none/a",
		"/room/none/b",
		"/room/none/c",
		"/room/none/d",
	})

	limit := uint64(3)
	txn, _ := db.NewTxn()
	keys, cursor, err := txn.ListKeys(DatabaseKey("/room/none"), &limit, nil)
	if err != nil {
		t.Fatalf("listkeys page1: %v", err)
	}
	if !reflect.DeepEqual(keys, []string{"/room/none/a", "/room/none/b", "/room/none/c"}) {
		t.Fatalf("page1 keys mismatch: %v", keys)
	}
	if cursor != "/room/none/d" {
		t.Fatalf("page1 cursor mismatch: %q", cursor)
	}

	keys, cursor, err = txn.ListKeys(DatabaseKey("/room/none"), &limit, &cursor)
	if err != nil {
		t.Fatalf("listkeys page2: %v", err)
	}
	if !reflect.DeepEqual(keys, []string{"/room/none/d"}) {
		t.Fatalf("page2 keys mismatch: %v", keys)
	}
	if cursor != endCursor {
		t.Fatalf("page2 cursor mismatch: %q", cursor)
	}
}

func TestIterateKeysAndReverseIterateKeys(t *testing.T) {
	db := newTestDB(t)
	seedKeys(t, db, []string{
		"/iter/none/1",
		"/iter/none/2",
		"/iter/fixed/skip",
		"/iter/none/3",
	})

	txn, _ := db.NewTxn()
	var direct []string
	if err := txn.IterateKeys(DatabaseKey("/iter"), func(key string) error {
		direct = append(direct, key)
		return nil
	}); err != nil {
		t.Fatalf("iterate: %v", err)
	}
	if !reflect.DeepEqual(direct, []string{"/iter/none/1", "/iter/none/2", "/iter/none/3"}) {
		t.Fatalf("iterate keys mismatch: %v", direct)
	}

	var reverse []string
	if err := txn.ReverseIterateKeys(DatabaseKey("/iter"), func(key string) error {
		reverse = append(reverse, key)
		return nil
	}); err != nil {
		t.Fatalf("reverse iterate: %v", err)
	}
	if !reflect.DeepEqual(reverse, []string{"/iter/none/3", "/iter/none/2", "/iter/none/1"}) {
		t.Fatalf("reverse keys mismatch: %v", reverse)
	}
}

func TestIterateRejectsFixedPrefix(t *testing.T) {
	db := newTestDB(t)
	txn, _ := db.NewTxn()
	if err := txn.IterateKeys(DatabaseKey("/x/fixed"), func(string) error { return nil }); err == nil {
		t.Fatal("expected iterate fixed prefix error")
	}
	if err := txn.ReverseIterateKeys(DatabaseKey("/x/fixed"), func(string) error { return nil }); err == nil {
		t.Fatal("expected reverse iterate fixed prefix error")
	}
}

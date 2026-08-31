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
package database

import (
	"sync"
	"testing"
	"time"

	local_store "github.com/Warp-net/warpnet/database/local-store"
)

// errFault is the error every injected failure returns. It is deliberately a
// plain DBError so local_store.IsNotFoundError reports false for it — repos
// swallow not-found errors, and a fault must not be mistaken for one.
const errFault = local_store.DBError("injected fault")

// faultStore wraps a real in-memory DB and fails a chosen operation on its Nth
// call, so a repo's error branches run against otherwise-genuine storage.
type faultStore struct {
	db     *local_store.DB
	newErr error
	failOn map[string]int
	counts map[string]int
	mx     sync.Mutex
}

func newFaultStore(t *testing.T) *faultStore {
	t.Helper()
	db, err := local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	if err := db.Run("test", "test"); err != nil {
		t.Fatalf("run db: %v", err)
	}
	t.Cleanup(db.Close)
	return &faultStore{db: db, failOn: map[string]int{}, counts: map[string]int{}}
}

// failNewTxn makes NewTxn / NewReadTxn return err instead of a transaction.
func (s *faultStore) failNewTxn(err error) *faultStore {
	s.newErr = err
	return s
}

// arm schedules the nth (1-based) call of the named method to fail and forgets
// any calls made so far, so a test can seed state first and still count from 1.
func (s *faultStore) arm(method string, nth int) *faultStore {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.counts = map[string]int{}
	s.failOn = map[string]int{method: nth}
	return s
}

func (s *faultStore) shouldFail(method string) error {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.counts[method]++
	if nth, ok := s.failOn[method]; ok && nth == s.counts[method] {
		return errFault
	}
	return nil
}

func (s *faultStore) NewTxn() (local_store.WarpTransactioner, error) {
	if s.newErr != nil {
		return nil, s.newErr
	}
	txn, err := s.db.NewTxn()
	if err != nil {
		return nil, err
	}
	return &faultTxn{WarpTransactioner: txn, store: s}, nil
}

func (s *faultStore) NewReadTxn() (local_store.WarpTransactioner, error) {
	if s.newErr != nil {
		return nil, s.newErr
	}
	txn, err := s.db.NewReadTxn()
	if err != nil {
		return nil, err
	}
	return &faultTxn{WarpTransactioner: txn, store: s}, nil
}

// The remaining methods satisfy the wider *Storer interfaces (user, node,
// follower, timeline, aliases, media) and honour the same injection table.

func (s *faultStore) Set(key local_store.DatabaseKey, value []byte) error {
	if err := s.shouldFail("db.Set"); err != nil {
		return err
	}
	return s.db.Set(key, value)
}

func (s *faultStore) SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error {
	if err := s.shouldFail("db.SetWithTTL"); err != nil {
		return err
	}
	return s.db.SetWithTTL(key, value, ttl)
}

func (s *faultStore) Get(key local_store.DatabaseKey) ([]byte, error) {
	if err := s.shouldFail("db.Get"); err != nil {
		return nil, err
	}
	return s.db.Get(key)
}

func (s *faultStore) Delete(key local_store.DatabaseKey) error {
	if err := s.shouldFail("db.Delete"); err != nil {
		return err
	}
	return s.db.Delete(key)
}

func (s *faultStore) GetExpiration(key local_store.DatabaseKey) (uint64, error) {
	if err := s.shouldFail("db.GetExpiration"); err != nil {
		return 0, err
	}
	return s.db.GetExpiration(key)
}

func (s *faultStore) GetSize(key local_store.DatabaseKey) (int64, error) {
	if err := s.shouldFail("db.GetSize"); err != nil {
		return 0, err
	}
	return s.db.GetSize(key)
}

func (s *faultStore) Sync() error {
	if err := s.shouldFail("db.Sync"); err != nil {
		return err
	}
	return s.db.Sync()
}

func (s *faultStore) IsClosed() bool                      { return s.db.IsClosed() }
func (s *faultStore) InnerDB() *local_store.WarpDB        { return s.db.InnerDB() }
func (s *faultStore) Close()                              { s.db.Close() }
func (s *faultStore) Run(username, password string) error { return s.db.Run(username, password) }

// faultTxn delegates to a real transaction unless the store has scheduled the
// current call to fail.
type faultTxn struct {
	local_store.WarpTransactioner
	store *faultStore
}

func (t *faultTxn) Set(key local_store.DatabaseKey, value []byte) error {
	if err := t.store.shouldFail("Set"); err != nil {
		return err
	}
	return t.WarpTransactioner.Set(key, value)
}

func (t *faultTxn) SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error {
	if err := t.store.shouldFail("SetWithTTL"); err != nil {
		return err
	}
	return t.WarpTransactioner.SetWithTTL(key, value, ttl)
}

func (t *faultTxn) Get(key local_store.DatabaseKey) ([]byte, error) {
	if err := t.store.shouldFail("Get"); err != nil {
		return nil, err
	}
	return t.WarpTransactioner.Get(key)
}

func (t *faultTxn) GetExpiration(key local_store.DatabaseKey) (uint64, error) {
	if err := t.store.shouldFail("GetExpiration"); err != nil {
		return 0, err
	}
	return t.WarpTransactioner.GetExpiration(key)
}

func (t *faultTxn) Delete(key local_store.DatabaseKey) error {
	if err := t.store.shouldFail("Delete"); err != nil {
		return err
	}
	return t.WarpTransactioner.Delete(key)
}

func (t *faultTxn) BatchSet(data []local_store.ListItem) error {
	if err := t.store.shouldFail("BatchSet"); err != nil {
		return err
	}
	return t.WarpTransactioner.BatchSet(data)
}

func (t *faultTxn) BatchGet(keys ...local_store.DatabaseKey) ([]local_store.ListItem, error) {
	if err := t.store.shouldFail("BatchGet"); err != nil {
		return nil, err
	}
	return t.WarpTransactioner.BatchGet(keys...)
}

func (t *faultTxn) Increment(key local_store.DatabaseKey) (uint64, error) {
	if err := t.store.shouldFail("Increment"); err != nil {
		return 0, err
	}
	return t.WarpTransactioner.Increment(key)
}

func (t *faultTxn) Decrement(key local_store.DatabaseKey) (uint64, error) {
	if err := t.store.shouldFail("Decrement"); err != nil {
		return 0, err
	}
	return t.WarpTransactioner.Decrement(key)
}

func (t *faultTxn) Commit() error {
	if err := t.store.shouldFail("Commit"); err != nil {
		return err
	}
	return t.WarpTransactioner.Commit()
}

func (t *faultTxn) List(prefix local_store.DatabaseKey, limit *uint64, cursor *string) ([]local_store.ListItem, string, error) {
	if err := t.store.shouldFail("List"); err != nil {
		return nil, "", err
	}
	return t.WarpTransactioner.List(prefix, limit, cursor)
}

func (t *faultTxn) ReverseList(prefix local_store.DatabaseKey, limit *uint64, cursor *string) ([]local_store.ListItem, string, error) {
	if err := t.store.shouldFail("ReverseList"); err != nil {
		return nil, "", err
	}
	return t.WarpTransactioner.ReverseList(prefix, limit, cursor)
}

func (t *faultTxn) ListKeys(prefix local_store.DatabaseKey, limit *uint64, cursor *string) ([]string, string, error) {
	if err := t.store.shouldFail("ListKeys"); err != nil {
		return nil, "", err
	}
	return t.WarpTransactioner.ListKeys(prefix, limit, cursor)
}

func (t *faultTxn) IterateKeys(prefix local_store.DatabaseKey, handler local_store.IterKeysFunc) error {
	if err := t.store.shouldFail("IterateKeys"); err != nil {
		return err
	}
	return t.WarpTransactioner.IterateKeys(prefix, handler)
}

func (t *faultTxn) ReverseIterateKeys(prefix local_store.DatabaseKey, handler local_store.IterKeysFunc) error {
	if err := t.store.shouldFail("ReverseIterateKeys"); err != nil {
		return err
	}
	return t.WarpTransactioner.ReverseIterateKeys(prefix, handler)
}

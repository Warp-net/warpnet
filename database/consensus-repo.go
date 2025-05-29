/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU General Public License for more details.

 You should have received a copy of the GNU General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

package database

import (
	"encoding/binary"
	"errors"
	"github.com/Warp-net/warpnet/database/storage"
	"github.com/dgraph-io/badger/v3"
	"os"
	"path/filepath"
)

const ConsensusConfigNamespace = "/CONFIGS/"

var (
	ErrConsensusKeyNotFound        = errors.New("consensus key not found")
	ErrConsensusRepoNotInitialized = errors.New("consensus repo not initialized")
)

type ConsensusStorer interface {
	Set(key storage.DatabaseKey, value []byte) error
	Get(key storage.DatabaseKey) ([]byte, error)
	Sync() error
	Path() string
	NewTxn() (storage.WarpTransactioner, error)
	Delete(key storage.DatabaseKey) error
}

type ConsensusRepo struct {
	db ConsensusStorer
}

func NewConsensusRepo(db ConsensusStorer) *ConsensusRepo {
	return &ConsensusRepo{db: db}
}

func (cr *ConsensusRepo) Sync() error {
	if cr == nil || cr.db == nil {
		return nil
	}
	return cr.db.Sync()
}

func (cr *ConsensusRepo) SnapshotsPath() (path string) {
	if cr == nil || cr.db == nil {
		return "/tmp/snapshots/member"
	}
	return filepath.Join(cr.db.Path(), "snapshots")
}

func (cr *ConsensusRepo) Reset() error {
	if cr == nil || cr.db == nil {
		return nil
	}
	txn, err := cr.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	err = txn.IterateKeys(ConsensusConfigNamespace, func(key string) error {
		return cr.db.Delete(storage.DatabaseKey(key))
	})
	if err != nil {
		return err
	}
	if err := txn.Commit(); err != nil {
		return err
	}
	return os.RemoveAll(cr.SnapshotsPath()) // NOTE this is only snapshots dir
}

// Set is used to set a key/value set outside of the raft log.
func (cr *ConsensusRepo) Set(key []byte, val []byte) error {
	if cr == nil || cr.db == nil {
		return ErrConsensusRepoNotInitialized
	}
	return cr.db.Set(storage.DatabaseKey(append([]byte(ConsensusConfigNamespace), key...)), val)
}

// Get is used to retrieve a value from the k/v store by key
func (cr *ConsensusRepo) Get(key []byte) ([]byte, error) {
	if cr == nil || cr.db == nil {
		return nil, ErrConsensusRepoNotInitialized
	}
	prefix := storage.DatabaseKey(append([]byte(ConsensusConfigNamespace), key...))
	val, err := cr.db.Get(prefix)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, ErrConsensusKeyNotFound
	}
	return val, err
}

// SetUint64 is like Set, but handles uint64 values
func (cr *ConsensusRepo) SetUint64(key []byte, val uint64) error {
	if cr == nil || cr.db == nil {
		return ErrConsensusRepoNotInitialized
	}
	fullKey := storage.DatabaseKey(append([]byte(ConsensusConfigNamespace), key...))
	return cr.db.Set(fullKey, uint64ToBytes(val))
}

func (cr *ConsensusRepo) GetUint64(key []byte) (uint64, error) {
	if cr == nil || cr.db == nil {
		return 0, ErrConsensusRepoNotInitialized
	}
	fullKey := storage.DatabaseKey(append([]byte(ConsensusConfigNamespace), key...))
	val, err := cr.db.Get(fullKey)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return 0, nil // intentionally!
	}

	return bytesToUint64(val), err
}

func (cr *ConsensusRepo) Close() error {
	return nil
}

func bytesToUint64(b []byte) uint64 {
	if len(b) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}

// Converts a uint64 to a byte slice
func uint64ToBytes(u uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, u)
	return buf
}

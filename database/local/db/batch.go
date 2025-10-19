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
	"runtime"

	"github.com/dgraph-io/badger/v4"
	ds "github.com/ipfs/go-datastore"
)

type Batch = ds.Batch

type batch struct {
	ds         *LocalDatastore
	writeBatch *badger.WriteBatch
}

var _ Batch = (*batch)(nil)

func (b *batch) Put(ctx context.Context, key DatastoreKey, value []byte) error {
	if !b.ds.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return b.writeBatch.Set(key.Bytes(), value)
}

func (b *batch) Delete(ctx context.Context, key DatastoreKey) error {
	if !b.ds.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return b.writeBatch.Delete(key.Bytes())
}

func (b *batch) Commit(ctx context.Context) error {
	if !b.ds.isRunning.Load() {
		return ErrNotRunning
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	err := b.writeBatch.Flush()
	if err != nil {
		// Discard incomplete transaction held by b.writeBatch
		_ = b.Cancel()
		return err
	}
	runtime.SetFinalizer(b, nil)
	return nil
}

func (b *batch) Cancel() error {
	if !b.ds.isRunning.Load() {
		return ErrNotRunning
	}

	b.writeBatch.Cancel()
	runtime.SetFinalizer(b, nil)
	return nil
}

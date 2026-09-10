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
package datastore

import (
	"context"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	ds "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"
)

func TestNewKey(t *testing.T) {
	require.Equal(t, ds.NewKey("/a/b"), NewKey("/a/b"))
}

func TestResultsHelpers(t *testing.T) {
	ctx := context.Background()

	base := dsq.ResultsWithEntries(dsq.Query{}, []dsq.Entry{
		{Key: "/b", Value: []byte("b")},
		{Key: "/a", Value: []byte("a")},
	})

	t.Run("ResultsReplaceQuery", func(t *testing.T) {
		q := Query{Prefix: "/", Orders: []dsq.Order{dsq.OrderByKey{}}}
		replaced := ResultsReplaceQuery(base, q)
		require.Equal(t, q, replaced.Query())
		require.NoError(t, replaced.Close())
	})

	t.Run("NaiveQueryApply", func(t *testing.T) {
		src := dsq.ResultsWithEntries(dsq.Query{}, []dsq.Entry{
			{Key: "/b", Value: []byte("b")},
			{Key: "/a", Value: []byte("a")},
		})
		out := NaiveQueryApply(Query{Orders: []dsq.Order{dsq.OrderByKey{}}}, src)
		entries, err := out.Rest()
		require.NoError(t, err)
		require.Equal(t, []string{"/a", "/b"}, []string{entries[0].Key, entries[1].Key})
	})

	t.Run("ResultsWithContext", func(t *testing.T) {
		res := ResultsWithContext(Query{}, func(_ context.Context, out chan<- Result) {
			out <- Result{Entry: DsEntry{Key: "/x", Value: []byte("x")}}
		})
		entries, err := res.Rest()
		require.NoError(t, err)
		require.Len(t, entries, 1)
		require.Equal(t, "/x", entries[0].Key)
	})

	_ = ctx
}

func TestMutexWrap(t *testing.T) {
	ctx := context.Background()

	wrapped := MutexWrap(ds.NewMapDatastore())
	require.IsType(t, &dssync.MutexDatastore{}, wrapped)

	require.NoError(t, wrapped.Put(ctx, NewKey("/k"), []byte("v")))
	got, err := wrapped.Get(ctx, NewKey("/k"))
	require.NoError(t, err)
	require.Equal(t, []byte("v"), got)

	_, err = wrapped.Get(ctx, NewKey("/absent"))
	require.ErrorIs(t, err, ErrNotFound)
}

func TestBlockstoreHelpers(t *testing.T) {
	ctx := context.Background()

	backing := MutexWrap(ds.NewMapDatastore())
	bs := NewBlockstore(backing, WriteThrough(true))
	require.NotNil(t, bs)

	block := blocks.NewBlock([]byte("hello warpnet"))
	require.NoError(t, bs.Put(ctx, block))

	got, err := bs.Get(ctx, block.Cid())
	require.NoError(t, err)
	require.Equal(t, block.RawData(), got.RawData())

	idStore := NewIdStore(bs)
	require.NotNil(t, idStore)

	got, err = idStore.Get(ctx, block.Cid())
	require.NoError(t, err)
	require.Equal(t, block.RawData(), got.RawData())
}

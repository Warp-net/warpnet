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
	"context"
	"testing"
	"time"

	local_store "github.com/Warp-net/warpnet/database/local-store"
	datastore "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	"github.com/stretchr/testify/require"
)

func TestNodeRepoNilGuards(t *testing.T) {
	ctx := context.Background()
	key := datastore.NewKey("/a")

	for _, repo := range []*NodeRepo{nil, NewNodeRepo(nil)} {
		require.ErrorIs(t, repo.Put(ctx, key, nil), ErrNilNodeRepo)
		require.ErrorIs(t, repo.Sync(ctx, key), ErrNilNodeRepo)
		require.ErrorIs(t, repo.PutWithTTL(ctx, key, nil, time.Minute), ErrNilNodeRepo)
		require.ErrorIs(t, repo.SetTTL(ctx, key, time.Minute), ErrNilNodeRepo)
		require.ErrorIs(t, repo.Delete(ctx, key), ErrNilNodeRepo)

		_, err := repo.GetExpiration(ctx, key)
		require.ErrorIs(t, err, ErrNilNodeRepo)
		_, err = repo.Get(ctx, key)
		require.ErrorIs(t, err, ErrNilNodeRepo)
		_, err = repo.Has(ctx, key)
		require.ErrorIs(t, err, ErrNilNodeRepo)
		_, err = repo.GetSize(ctx, key)
		require.ErrorIs(t, err, ErrNilNodeRepo)
		_, err = repo.DiskUsage(ctx)
		require.ErrorIs(t, err, ErrNilNodeRepo)
		_, err = repo.Query(ctx, dsq.Query{})
		require.ErrorIs(t, err, ErrNilNodeRepo)
		_, err = repo.Batch(ctx)
		require.ErrorIs(t, err, ErrNilNodeRepo)

		require.NoError(t, repo.Close())
	}

	var nilRepo *NodeRepo
	require.ErrorIs(t, nilRepo.BlocklistExponential("peer"), ErrNilNodeRepo)
	require.ErrorIs(t, nilRepo.BlocklistPermanent("peer"), ErrNilNodeRepo)
	require.False(t, nilRepo.IsBlocklisted("peer"))
	_, err := nilRepo.BlocklistTerm("peer")
	require.ErrorIs(t, err, ErrNilNodeRepo)
	require.NoError(t, nilRepo.BlocklistRemove("peer"))
}

func TestNodeRepoCancelledContext(t *testing.T) {
	repo := NewNodeRepo(newFaultStore(t))
	key := datastore.NewKey("/a")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.ErrorIs(t, repo.Put(ctx, key, nil), context.Canceled)
	require.ErrorIs(t, repo.Sync(ctx, key), context.Canceled)
	require.ErrorIs(t, repo.PutWithTTL(ctx, key, nil, time.Minute), context.Canceled)
	require.ErrorIs(t, repo.SetTTL(ctx, key, time.Minute), context.Canceled)
	require.ErrorIs(t, repo.Delete(ctx, key), context.Canceled)

	_, err := repo.GetExpiration(ctx, key)
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.Get(ctx, key)
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.Has(ctx, key)
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.GetSize(ctx, key)
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.DiskUsage(ctx)
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.Query(ctx, dsq.Query{})
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.Batch(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestNodeRepoClosedStore(t *testing.T) {
	ctx := context.Background()
	key := datastore.NewKey("/a")

	s := newFaultStore(t)
	repo := NewNodeRepo(s)

	b, err := repo.Batch(ctx)
	require.NoError(t, err)

	s.Close()

	require.ErrorIs(t, repo.Put(ctx, key, nil), local_store.ErrNotRunning)
	require.ErrorIs(t, repo.Sync(ctx, key), local_store.ErrNotRunning)
	require.ErrorIs(t, repo.PutWithTTL(ctx, key, nil, time.Minute), local_store.ErrNotRunning)
	require.ErrorIs(t, repo.SetTTL(ctx, key, time.Minute), local_store.ErrNotRunning)
	require.ErrorIs(t, repo.Delete(ctx, key), local_store.ErrNotRunning)

	_, err = repo.GetExpiration(ctx, key)
	require.ErrorIs(t, err, local_store.ErrNotRunning)
	_, err = repo.Get(ctx, key)
	require.ErrorIs(t, err, local_store.ErrNotRunning)
	_, err = repo.Has(ctx, key)
	require.ErrorIs(t, err, local_store.ErrNotRunning)
	_, err = repo.GetSize(ctx, key)
	require.ErrorIs(t, err, local_store.ErrNotRunning)
	_, err = repo.DiskUsage(ctx)
	require.ErrorIs(t, err, local_store.ErrNotRunning)
	_, err = repo.Query(ctx, dsq.Query{})
	require.ErrorIs(t, err, local_store.ErrNotRunning)
	_, err = repo.Batch(ctx)
	require.ErrorIs(t, err, local_store.ErrNotRunning)

	// a batch opened before the close refuses every write and commits as a no-op
	require.ErrorIs(t, b.Put(ctx, key, nil), local_store.ErrNotRunning)
	require.ErrorIs(t, b.Delete(ctx, key), local_store.ErrNotRunning)
	require.NoError(t, b.Commit(ctx))

	// closing an already-closed repo is a no-op
	require.NoError(t, repo.Close())
}

func TestNodeRepoBatch(t *testing.T) {
	ctx := context.Background()
	s := newFaultStore(t)
	repo := NewNodeRepo(s)

	b, err := repo.Batch(ctx)
	require.NoError(t, err)

	require.NoError(t, b.Put(ctx, datastore.NewKey("/batch/a"), []byte("a")))
	require.NoError(t, b.Put(ctx, datastore.NewKey("/batch/b"), []byte("b")))
	require.NoError(t, b.Delete(ctx, datastore.NewKey("/batch/b")))
	require.NoError(t, b.Commit(ctx))

	got, err := repo.Get(ctx, datastore.NewKey("/batch/a"))
	require.NoError(t, err)
	require.Equal(t, []byte("a"), got)

	_, err = repo.Get(ctx, datastore.NewKey("/batch/b"))
	require.ErrorIs(t, err, datastore.ErrNotFound)

	t.Run("PutWithTTL", func(t *testing.T) {
		raw, err := repo.Batch(ctx)
		require.NoError(t, err)
		ttlBatch, ok := raw.(*batch)
		require.True(t, ok)

		require.NoError(t, ttlBatch.putWithTTL(datastore.NewKey("/batch/ttl"), []byte("v"), time.Hour))
		require.NoError(t, ttlBatch.Commit(ctx))

		exp, err := repo.GetExpiration(ctx, datastore.NewKey("/batch/ttl"))
		require.NoError(t, err)
		require.False(t, exp.IsZero())
	})

	t.Run("CancelledContext", func(t *testing.T) {
		raw, err := repo.Batch(ctx)
		require.NoError(t, err)

		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		require.ErrorIs(t, raw.Put(cancelledCtx, datastore.NewKey("/batch/x"), []byte("x")), context.Canceled)
		require.ErrorIs(t, raw.Delete(cancelledCtx, datastore.NewKey("/batch/x")), context.Canceled)
		require.ErrorIs(t, raw.Commit(cancelledCtx), context.Canceled)
	})

	t.Run("NilBatch", func(t *testing.T) {
		var b *batch
		require.ErrorIs(t, b.put(datastore.NewKey("/a"), nil), ErrNilNodeRepo)
		require.ErrorIs(t, b.putWithTTL(datastore.NewKey("/a"), nil, time.Minute), ErrNilNodeRepo)
		require.ErrorIs(t, b.Delete(ctx, datastore.NewKey("/a")), ErrNilNodeRepo)
		require.ErrorIs(t, b.Commit(ctx), ErrNilNodeRepo)
		require.NoError(t, b.Cancel())
	})

	t.Run("ClosedStore", func(t *testing.T) {
		closed := newFaultStore(t)
		raw, err := NewNodeRepo(closed).Batch(ctx)
		require.NoError(t, err)
		ttlBatch, ok := raw.(*batch)
		require.True(t, ok)
		closed.Close()

		require.ErrorIs(t, ttlBatch.putWithTTL(datastore.NewKey("/a"), nil, time.Minute), local_store.ErrNotRunning)
	})
}

func TestNodeRepoQueryOptions(t *testing.T) {
	ctx := context.Background()
	repo := NewNodeRepo(newFaultStore(t))

	for _, k := range []string{"/q/a", "/q/b", "/q/c", "/q/d"} {
		require.NoError(t, repo.Put(ctx, datastore.NewKey(k), []byte(k)))
	}

	collect := func(t *testing.T, q dsq.Query) []dsq.Entry {
		t.Helper()
		res, err := repo.Query(ctx, q)
		require.NoError(t, err)
		defer res.Close()
		entries, err := res.Rest()
		require.NoError(t, err)
		return entries
	}

	t.Run("Descending", func(t *testing.T) {
		entries := collect(t, dsq.Query{Orders: []dsq.Order{dsq.OrderByKeyDescending{}}})
		require.NotEmpty(t, entries)
		for i := 1; i < len(entries); i++ {
			require.Greater(t, entries[i-1].Key, entries[i].Key)
		}
	})

	t.Run("Ascending", func(t *testing.T) {
		entries := collect(t, dsq.Query{Orders: []dsq.Order{dsq.OrderByKey{}}})
		require.NotEmpty(t, entries)
		for i := 1; i < len(entries); i++ {
			require.Less(t, entries[i-1].Key, entries[i].Key)
		}
	})

	t.Run("UnsupportedOrderFallsBackToNaive", func(t *testing.T) {
		entries := collect(t, dsq.Query{Orders: []dsq.Order{dsq.OrderByValue{}}})
		require.NotEmpty(t, entries)
	})

	t.Run("OffsetAndLimit", func(t *testing.T) {
		all := collect(t, dsq.Query{})
		require.GreaterOrEqual(t, len(all), 4)

		page := collect(t, dsq.Query{Offset: 1, Limit: 2})
		require.Len(t, page, 2)
	})

	t.Run("KeysOnly", func(t *testing.T) {
		entries := collect(t, dsq.Query{KeysOnly: true})
		require.NotEmpty(t, entries)
		for _, e := range entries {
			require.Empty(t, e.Value)
		}
	})

	t.Run("ReturnExpirations", func(t *testing.T) {
		require.NoError(t, repo.PutWithTTL(ctx, datastore.NewKey("/q/ttl"), []byte("v"), time.Hour))
		entries := collect(t, dsq.Query{Prefix: "/q", ReturnExpirations: true})
		require.NotEmpty(t, entries)
	})

	t.Run("Filters", func(t *testing.T) {
		entries := collect(t, dsq.Query{
			Filters: []dsq.Filter{dsq.FilterKeyCompare{Op: dsq.Equal, Key: "/q/a"}},
		})
		require.Len(t, entries, 1)
		require.Equal(t, "/q/a", entries[0].Key)
	})

	t.Run("FiltersWithOffset", func(t *testing.T) {
		entries := collect(t, dsq.Query{
			Filters: []dsq.Filter{dsq.FilterKeyCompare{Op: dsq.GreaterThan, Key: "/q/a"}},
			Offset:  1,
		})
		require.NotEmpty(t, entries)
	})

	t.Run("FiltersKeysOnly", func(t *testing.T) {
		entries := collect(t, dsq.Query{
			KeysOnly: true,
			Filters:  []dsq.Filter{dsq.FilterKeyCompare{Op: dsq.Equal, Key: "/q/a"}},
		})
		require.Len(t, entries, 1)
	})

	t.Run("PrefixMisses", func(t *testing.T) {
		entries := collect(t, dsq.Query{Prefix: "/nothing-here"})
		require.Empty(t, entries)
	})
}

func TestNodeRepoBlocklistErrorPaths(t *testing.T) {
	const peer = "12D3KooWBlockedPeer"

	t.Run("EmptyPeerId", func(t *testing.T) {
		repo := NewNodeRepo(newFaultStore(t))
		require.Error(t, repo.BlocklistExponential(""))
		require.Error(t, repo.BlocklistPermanent(""))
		require.False(t, repo.IsBlocklisted(""))
		_, err := repo.BlocklistTerm("")
		require.Error(t, err)
		require.NoError(t, repo.BlocklistRemove(""))
	})

	t.Run("ExponentialFaults", func(t *testing.T) {
		for _, o := range []faultOp{op("Get"), op("SetWithTTL"), op("Set"), op("Commit")} {
			t.Run(o.String(), func(t *testing.T) {
				s := newFaultStore(t).arm(o.method, o.nth)
				require.ErrorIs(t, NewNodeRepo(s).BlocklistExponential(peer), errFault)
			})
		}
		t.Run("NewTxn", func(t *testing.T) {
			s := newFaultStore(t).failNewTxn(errFault)
			require.ErrorIs(t, NewNodeRepo(s).BlocklistExponential(peer), errFault)
		})
	})

	t.Run("PermanentFaults", func(t *testing.T) {
		for _, o := range []faultOp{op("SetWithTTL"), op("Set"), op("Commit")} {
			t.Run(o.String(), func(t *testing.T) {
				s := newFaultStore(t).arm(o.method, o.nth)
				require.ErrorIs(t, NewNodeRepo(s).BlocklistPermanent(peer), errFault)
			})
		}
		t.Run("NewTxn", func(t *testing.T) {
			s := newFaultStore(t).failNewTxn(errFault)
			require.ErrorIs(t, NewNodeRepo(s).BlocklistPermanent(peer), errFault)
		})
	})

	t.Run("RemoveFaults", func(t *testing.T) {
		for _, o := range []faultOp{op("Set"), op("Commit")} {
			t.Run(o.String(), func(t *testing.T) {
				s := newFaultStore(t)
				require.NoError(t, NewNodeRepo(s).BlocklistPermanent(peer))
				s.arm(o.method, o.nth)
				require.ErrorIs(t, NewNodeRepo(s).BlocklistRemove(peer), errFault)
			})
		}
		t.Run("NewTxn", func(t *testing.T) {
			s := newFaultStore(t).failNewTxn(errFault)
			require.ErrorIs(t, NewNodeRepo(s).BlocklistRemove(peer), errFault)
		})
	})

	t.Run("TermStoreFails", func(t *testing.T) {
		s := newFaultStore(t)
		require.NoError(t, NewNodeRepo(s).BlocklistPermanent(peer))

		s.arm("db.Get", 1)
		_, err := NewNodeRepo(s).BlocklistTerm(peer)
		require.ErrorIs(t, err, errFault)
	})

	t.Run("CorruptTermPayload", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewNodeRepo(s)
		require.NoError(t, repo.BlocklistPermanent(peer))

		termKey := local_store.NewPrefixBuilder(repo.prefix).
			AddSubPrefix(BlocklistSubNamespace).
			AddSubPrefix(BlocklistTermSubNamespace).
			AddRootID(peer).
			Build()
		require.NoError(t, s.db.Set(termKey, []byte("{not-json")))

		_, err := repo.BlocklistTerm(peer)
		require.Error(t, err)
		require.Error(t, repo.BlocklistExponential(peer))
	})

	t.Run("TermOfUnknownPeerIsZero", func(t *testing.T) {
		term, err := NewNodeRepo(newFaultStore(t)).BlocklistTerm("never-blocked")
		require.NoError(t, err)
		require.Equal(t, BlockLevel(0), term.Level)
	})
}

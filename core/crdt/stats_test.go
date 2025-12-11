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

 WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
 Use at your own risk. The maintainers shall not be liable for any damages or data loss
 resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package crdt

import (
	"context"
	"testing"
	"time"

	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/stretchr/testify/require"
)

func TestCRDTStatsStore_IncrementStat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a test libp2p host
	host, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer host.Close()

	// Create pubsub
	ps, err := pubsub.NewGossipSub(ctx, host)
	require.NoError(t, err)

	// Create in-memory datastore
	baseStore := dssync.MutexWrap(ds.NewMapDatastore())

	// Create CRDT stats store
	store, err := NewCRDTStatsStore(ctx, baseStore, ps, host.ID().String(), "testnet")
	require.NoError(t, err)
	defer store.Close()

	// Test incrementing likes
	tweetID := "tweet123"
	count, err := store.IncrementStat(tweetID, StatTypeLikes)
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)

	// Increment again
	count, err = store.IncrementStat(tweetID, StatTypeLikes)
	require.NoError(t, err)
	require.Equal(t, uint64(2), count)

	// Get aggregated stat
	total, err := store.GetAggregatedStat(tweetID, StatTypeLikes)
	require.NoError(t, err)
	require.Equal(t, uint64(2), total)
}

func TestCRDTStatsStore_DecrementStat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a test libp2p host
	host, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer host.Close()

	// Create pubsub
	ps, err := pubsub.NewGossipSub(ctx, host)
	require.NoError(t, err)

	// Create in-memory datastore
	baseStore := dssync.MutexWrap(ds.NewMapDatastore())

	// Create CRDT stats store
	store, err := NewCRDTStatsStore(ctx, baseStore, ps, host.ID().String(), "testnet")
	require.NoError(t, err)
	defer store.Close()

	// Test incrementing and decrementing
	tweetID := "tweet456"
	
	// Increment to 3
	_, err = store.IncrementStat(tweetID, StatTypeRetweets)
	require.NoError(t, err)
	_, err = store.IncrementStat(tweetID, StatTypeRetweets)
	require.NoError(t, err)
	_, err = store.IncrementStat(tweetID, StatTypeRetweets)
	require.NoError(t, err)

	// Decrement once
	count, err := store.DecrementStat(tweetID, StatTypeRetweets)
	require.NoError(t, err)
	require.Equal(t, uint64(2), count)
}

func TestCRDTStatsStore_GetTweetStats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a test libp2p host
	host, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer host.Close()

	// Create pubsub
	ps, err := pubsub.NewGossipSub(ctx, host)
	require.NoError(t, err)

	// Create in-memory datastore
	baseStore := dssync.MutexWrap(ds.NewMapDatastore())

	// Create CRDT stats store
	store, err := NewCRDTStatsStore(ctx, baseStore, ps, host.ID().String(), "testnet")
	require.NoError(t, err)
	defer store.Close()

	tweetID := "tweet789"

	// Add various stats
	_, err = store.IncrementStat(tweetID, StatTypeLikes)
	require.NoError(t, err)
	_, err = store.IncrementStat(tweetID, StatTypeLikes)
	require.NoError(t, err)

	_, err = store.IncrementStat(tweetID, StatTypeRetweets)
	require.NoError(t, err)

	_, err = store.IncrementStat(tweetID, StatTypeReplies)
	require.NoError(t, err)
	_, err = store.IncrementStat(tweetID, StatTypeReplies)
	require.NoError(t, err)
	_, err = store.IncrementStat(tweetID, StatTypeReplies)
	require.NoError(t, err)

	_, err = store.IncrementStat(tweetID, StatTypeViews)
	require.NoError(t, err)

	// Get all stats
	stats, err := store.GetTweetStats(tweetID)
	require.NoError(t, err)
	require.Equal(t, uint64(2), stats.LikesCount)
	require.Equal(t, uint64(1), stats.RetweetsCount)
	require.Equal(t, uint64(3), stats.RepliesCount)
	require.Equal(t, uint64(1), stats.ViewsCount)
}

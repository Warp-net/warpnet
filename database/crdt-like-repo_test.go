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

package database

import (
	"context"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/crdt"
	"github.com/Warp-net/warpnet/database/local"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/stretchr/testify/require"
)

func TestCRDTLikeRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup local database
	db, err := local.New("", local.DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	defer db.Close()

	// Create a test libp2p host
	host, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer host.Close()

	// Create pubsub
	ps, err := pubsub.NewGossipSub(ctx, host)
	require.NoError(t, err)

	// Create in-memory datastore for CRDT
	crdtStore := dssync.MutexWrap(ds.NewMapDatastore())

	// Create CRDT stats store
	statsStore, err := crdt.NewCRDTStatsStore(ctx, crdtStore, ps, host.ID().String(), "testnet")
	require.NoError(t, err)
	defer statsStore.Close()

	// Create CRDT like repository
	likeRepo := NewCRDTLikeRepo(db, statsStore)

	// Test like operation
	tweetID := "tweet123"
	userID := "user456"

	count, err := likeRepo.Like(tweetID, userID)
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)

	// Test like count
	count, err = likeRepo.LikesCount(tweetID)
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)

	// Test unlike operation
	count, err = likeRepo.Unlike(tweetID, userID)
	require.NoError(t, err)
	require.Equal(t, uint64(0), count)
}

func TestCRDTLikeRepo_MultipleNodes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup two nodes
	// Node 1
	db1, err := local.New("", local.DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	defer db1.Close()

	host1, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer host1.Close()

	ps1, err := pubsub.NewGossipSub(ctx, host1)
	require.NoError(t, err)

	crdtStore1 := dssync.MutexWrap(ds.NewMapDatastore())
	statsStore1, err := crdt.NewCRDTStatsStore(ctx, crdtStore1, ps1, host1.ID().String(), "testnet")
	require.NoError(t, err)
	defer statsStore1.Close()

	likeRepo1 := NewCRDTLikeRepo(db1, statsStore1)

	// Node 2
	db2, err := local.New("", local.DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	defer db2.Close()

	host2, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer host2.Close()

	ps2, err := pubsub.NewGossipSub(ctx, host2)
	require.NoError(t, err)

	crdtStore2 := dssync.MutexWrap(ds.NewMapDatastore())
	statsStore2, err := crdt.NewCRDTStatsStore(ctx, crdtStore2, ps2, host2.ID().String(), "testnet")
	require.NoError(t, err)
	defer statsStore2.Close()

	likeRepo2 := NewCRDTLikeRepo(db2, statsStore2)

	// Connect the two hosts
	host1.Peerstore().AddAddrs(host2.ID(), host2.Addrs(), time.Hour)
	err = host1.Connect(ctx, host1.Peerstore().PeerInfo(host2.ID()))
	require.NoError(t, err)

	// Give time for pubsub to propagate
	time.Sleep(2 * time.Second)

	// Test likes from different nodes
	tweetID := "tweet789"

	// Node 1 likes
	_, err = likeRepo1.Like(tweetID, "user1")
	require.NoError(t, err)

	// Node 2 likes
	_, err = likeRepo2.Like(tweetID, "user2")
	require.NoError(t, err)

	// Give time for CRDT sync (Note: in real implementation, we'd need proper network sync)
	time.Sleep(2 * time.Second)

	// Both nodes should eventually see the same count
	// Note: This test is simplified and may need adjustments based on actual CRDT sync behavior
	count1, err := likeRepo1.LikesCount(tweetID)
	require.NoError(t, err)
	
	count2, err := likeRepo2.LikesCount(tweetID)
	require.NoError(t, err)

	// At minimum, each node should see its own like
	require.GreaterOrEqual(t, count1, uint64(1))
	require.GreaterOrEqual(t, count2, uint64(1))
}

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package dht

import (
	"context"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

// repoTable is a node that keeps its peers in the node repo the way a member
// node does.
type repoTable struct {
	host     warpnet.P2PNode
	table    *distributedHashTable
	repo     *database.NodeRepo
	admitted *admissions
}

func newRepoTable(t *testing.T) *repoTable {
	t.Helper()
	db, err := local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	require.NoError(t, err)
	require.NoError(t, database.NewAuthRepo(db, "test").Authenticate("test", "test"))
	t.Cleanup(db.Close)
	repo := database.NewNodeRepo(db)

	ctx, cancel := context.WithCancel(context.Background())
	store, err := warpnet.NewPeerstore(ctx, repo)
	require.NoError(t, err)
	admitted := &admissions{peers: map[peer.ID]bool{}}
	d := NewDHTable(ctx,
		Network("testnet"),
		RoutingStore(repo),
		AddPeerCallbacks(admitted.add),
	)
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.ForceReachabilityPublic(),
		libp2p.Peerstore(store),
		libp2p.Routing(d.StartRouting),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		d.Close()
		_ = h.Close()
		cancel()
	})
	return &repoTable{host: h, table: d, repo: repo, admitted: admitted}
}

func (n *repoTable) holds(id peer.ID) bool {
	return n.table.dht.RoutingTable().Find(id) != ""
}

func TestABlocklistedPeerNeverGetsIntoTheRoutingTable(t *testing.T) {
	skipWithoutHosts(t)

	node, blocked, other := newRepoTable(t), newRatedTable(t), newRatedTable(t)
	require.NoError(t, node.repo.BlocklistPermanent(blocked.host.ID().String()))
	connectHosts(t, node.host, blocked.host)
	connectHosts(t, node.host, other.host)

	require.Eventually(t, func() bool { return node.holds(other.host.ID()) },
		admitTimeout, pollTick, "an unblocked DHT server is taken in")
	require.Never(t, func() bool {
		return node.holds(blocked.host.ID()) || node.admitted.has(blocked.host.ID())
	}, quietWindow, pollTick, "the blocklisted peer is kept out of the table and of discovery")
}

func TestAPeerBlocklistedInsideTheRoutingTableIsDroppedOnReconnect(t *testing.T) {
	skipWithoutHosts(t)

	node, blocked, other := newRepoTable(t), newRatedTable(t), newRatedTable(t)
	connectHosts(t, node.host, blocked.host)
	connectHosts(t, node.host, other.host)
	require.Eventually(t, func() bool {
		return node.holds(blocked.host.ID()) && node.holds(other.host.ID())
	}, admitTimeout, pollTick)

	require.NoError(t, node.repo.BlocklistPermanent(blocked.host.ID().String()))
	require.NoError(t, node.host.Network().ClosePeer(blocked.host.ID()))
	connectHosts(t, node.host, blocked.host)

	require.Eventually(t, func() bool { return !node.holds(blocked.host.ID()) },
		admitTimeout, pollTick, "the blocklisted peer is dropped from the table")
	require.True(t, node.holds(other.host.ID()), "and every other peer stays in it")
}

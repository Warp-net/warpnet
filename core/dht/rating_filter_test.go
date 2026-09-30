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
package dht

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/stretchr/testify/require"
)

const (
	admitTimeout = 30 * time.Second
	quietWindow  = 3 * time.Second
	pollTick     = 50 * time.Millisecond
)

func skipWithoutHosts(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("runs live libp2p hosts")
	}
}

// admissions is every peer the routing table's add callbacks were handed.
type admissions struct {
	mx    sync.Mutex
	peers map[peer.ID]bool
}

func (a *admissions) add(id warpnet.WarpPeerID) {
	a.mx.Lock()
	defer a.mx.Unlock()
	a.peers[id] = true
}

func (a *admissions) has(id peer.ID) bool {
	a.mx.Lock()
	defer a.mx.Unlock()
	return a.peers[id]
}

// ratedTable is a node that routes the way a member node does: its host is
// routed through the table, and the table reads a real rating.
type ratedTable struct {
	host     warpnet.P2PNode
	table    *distributedHashTable
	ratings  *rating.PeersRatings
	admitted *admissions
}

func newRatedTable(t *testing.T) *ratedTable {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ratings := rating.NewPeersRatings()
	admitted := &admissions{peers: map[peer.ID]bool{}}
	d := NewDHTable(ctx,
		Network("testnet"),
		RoutingStore(memStore()),
		Ratings(ratings),
		AddPeerCallbacks(admitted.add),
	)
	// A node that knows itself reachable serves the DHT, and only DHT
	// servers get into each other's routing tables.
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.ForceReachabilityPublic(),
		libp2p.Routing(d.StartRouting),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		d.Close()
		_ = h.Close()
		cancel()
	})
	return &ratedTable{host: h, table: d, ratings: ratings, admitted: admitted}
}

func (n *ratedTable) holds(id peer.ID) bool {
	return n.table.dht.RoutingTable().Find(id) != ""
}

func (n *ratedTable) refresh(t *testing.T) {
	t.Helper()
	select {
	case <-n.table.dht.RefreshRoutingTable():
	case <-time.After(admitTimeout):
		t.Fatal("the routing table refresh never finished")
	}
}

// closestHeard runs a lookup for key and reports what it returned and every
// peer the queried servers handed out on the way.
func (n *ratedTable) closestHeard(t *testing.T, key peer.ID) (closest []peer.ID, heard map[peer.ID]bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), admitTimeout)
	ctx, events := routing.RegisterForQueryEvents(ctx)
	heard = map[peer.ID]bool{}
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for ev := range events {
			for _, info := range ev.Responses {
				heard[info.ID] = true
			}
		}
	}()
	closest, err := n.table.dht.GetClosestPeers(ctx, string(key))
	cancel()
	<-drained
	require.NoError(t, err)
	return closest, heard
}

func connectHosts(t *testing.T, from, to warpnet.P2PNode) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, from.Connect(ctx, peer.AddrInfo{ID: to.ID(), Addrs: to.Addrs()}))
}

func requireHolds(t *testing.T, n *ratedTable, id peer.ID, msgAndArgs ...any) {
	t.Helper()
	require.Eventually(t, func() bool { return n.holds(id) }, admitTimeout, pollTick, msgAndArgs...)
}

// The table meets a floored peer both on connection and in the lookups of a
// refresh, and keeps it out either way; a refresh after its rating recovers
// lets it in.
func TestTheRoutingTableKeepsAFlooredPeerOut(t *testing.T) {
	skipWithoutHosts(t)

	node, floored, trusted := newRatedTable(t), newRatedTable(t), newRatedTable(t)
	node.ratings.Rate(floored.host.ID(), rating.TierFloor)
	connectHosts(t, node.host, floored.host)
	connectHosts(t, node.host, trusted.host)
	connectHosts(t, floored.host, trusted.host)

	requireHolds(t, node, trusted.host.ID(), "a trusted DHT server is taken in")
	requireHolds(t, trusted, floored.host.ID(), "the floored peer serves the DHT like any other")

	closest, heard := node.closestHeard(t, trusted.host.ID())
	require.True(t, heard[floored.host.ID()], "the trusted peer hands the floored one out")
	require.NotContains(t, closest, floored.host.ID(), "and the lookup drops it")

	node.refresh(t)
	require.Never(t, func() bool {
		return node.holds(floored.host.ID()) || node.admitted.has(floored.host.ID())
	}, quietWindow, pollTick, "the floored peer is kept out of the table")

	node.ratings.Rate(floored.host.ID(), rating.TierTrusted)
	node.refresh(t)
	requireHolds(t, node, floored.host.ID(), "a refresh lets a recovered peer in")
}

// A peer floored while it sits in the routing table is taken out by the
// next sweep, and lookups stop returning it.
func TestAPeerFlooredInsideTheRoutingTableIsTakenOut(t *testing.T) {
	skipWithoutHosts(t)

	node, rated, other := newRatedTable(t), newRatedTable(t), newRatedTable(t)
	connectHosts(t, node.host, rated.host)
	connectHosts(t, node.host, other.host)
	connectHosts(t, rated.host, other.host)
	requireHolds(t, node, rated.host.ID(), "an unrated DHT server is taken in")
	requireHolds(t, node, other.host.ID())

	node.ratings.Rate(rated.host.ID(), rating.TierFloor)
	node.table.removeDisallowedPeers()
	require.False(t, node.holds(rated.host.ID()), "the sweep takes a floored peer out of the table")
	require.True(t, node.holds(other.host.ID()), "and leaves every other peer in it")

	closest, _ := node.closestHeard(t, other.host.ID())
	require.NotContains(t, closest, rated.host.ID(), "lookups no longer return it")
}

// silentDHTServer answers DHT queries but runs no lookups of its own, so it
// never dials the node under test by itself.
func silentDHTServer(t *testing.T) warpnet.P2PNode {
	t.Helper()
	h := newHost(t)
	server, err := dht.New(h, dht.Mode(dht.ModeServer), dht.ProtocolPrefix("/testnet"), dht.DisableAutoRefresh())
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })
	return h
}

// A routed host with no address for a peer looks it up in the DHT, and a
// lookup lets in the peer that answers it past the table's filter. A floored
// peer let in that way is handed to nobody downstream, and the next sweep
// takes it out.
func TestAFlooredPeerALookupLetsInIsNeitherDiscoveredNorKept(t *testing.T) {
	skipWithoutHosts(t)

	node, known := newRatedTable(t), newRatedTable(t)
	floored := silentDHTServer(t)
	node.ratings.Rate(floored.ID(), rating.TierFloor)
	connectHosts(t, node.host, known.host)
	connectHosts(t, floored, known.host)
	requireHolds(t, node, known.host.ID())
	requireHolds(t, known, floored.ID(), "another peer knows where the floored one is")
	require.Empty(t, node.host.Peerstore().Addrs(floored.ID()), "the node has no address for the floored peer")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	require.NoError(t, node.host.Connect(ctx, peer.AddrInfo{ID: floored.ID()}),
		"the routed host finds the floored peer through the DHT")
	requireHolds(t, node, floored.ID(), "the lookup lets the floored peer into the table")
	require.False(t, node.admitted.has(floored.ID()), "but discovery never hears of it")

	node.table.removeDisallowedPeers()
	require.False(t, node.holds(floored.ID()), "and the sweep takes it out")
}

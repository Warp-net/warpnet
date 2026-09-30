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
package node

import (
	"context"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/ratelimit"
	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p"
	"github.com/stretchr/testify/require"
)

const (
	tagTimeout = 30 * time.Second
	pollTick   = 50 * time.Millisecond
)

func skipWithoutHosts(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("runs live libp2p hosts")
	}
}

// ratedNode is a node whose connection manager learns from a real rating
// what each peer is worth.
func ratedNode(t *testing.T) (*WarpNode, *rating.PeersRatings) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ratings := rating.NewPeersRatings()
	n, err := NewWarpNode(ctx, ratings, ratelimit.Settings{}, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(n.StopNode)
	return n, ratings
}

func newPeerHost(t *testing.T) warpnet.P2PNode {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func addrInfo(h warpnet.P2PNode) warpnet.WarpAddrInfo {
	return warpnet.WarpAddrInfo{ID: h.ID(), Addrs: h.Addrs()}
}

// ratingTagOf is the rating tag the connection manager holds on a peer.
func ratingTagOf(n *WarpNode, id warpnet.WarpPeerID) (int, bool) {
	info := n.Node().ConnManager().GetTagInfo(id)
	if info == nil {
		return 0, false
	}
	tag, ok := info.Tags[ratingTag]
	return tag, ok
}

func requireTagged(t *testing.T, n *WarpNode, id warpnet.WarpPeerID, want int, msgAndArgs ...any) {
	t.Helper()
	require.Eventually(t, func() bool {
		tag, ok := ratingTagOf(n, id)
		return ok && tag == want
	}, tagTimeout, pollTick, msgAndArgs...)
}

func TestAConnectingPeerIsWorthToTheConnectionManagerWhatItsTierIs(t *testing.T) {
	skipWithoutHosts(t)
	n, ratings := ratedNode(t)

	for _, tier := range []rating.Tier{rating.TierFloor, rating.TierDegraded, rating.TierWatched, rating.TierTrusted} {
		p := newPeerHost(t)
		ratings.Rate(p.ID(), tier)
		require.NoError(t, n.Connect(addrInfo(p)))
		requireTagged(t, n, p.ID(), tier.ConnTag(), "a %s peer is worth %d", tier, tier.ConnTag())
	}

	unrated := newPeerHost(t)
	require.NoError(t, n.Connect(addrInfo(unrated)))
	requireTagged(t, n, unrated.ID(), rating.TierTrusted.ConnTag(), "a peer nobody has rated is worth a trusted one")
}

// A rating that moves while its peer stays connected reaches the connection
// manager on the next tagging pass, without the peer having to reconnect.
func TestARatingChangeReachesTheConnectionManagerWhileThePeerStaysConnected(t *testing.T) {
	skipWithoutHosts(t)
	n, ratings := ratedNode(t)
	p := newPeerHost(t)

	require.NoError(t, n.Connect(addrInfo(p)))
	requireTagged(t, n, p.ID(), rating.TierTrusted.ConnTag())

	ratings.Rate(p.ID(), rating.TierFloor)
	n.tagPeers()
	requireTagged(t, n, p.ID(), rating.TierFloor.ConnTag(), "a peer floored while connected is worth its tier now")

	ratings.Rate(p.ID(), rating.TierTrusted)
	n.tagPeers()
	requireTagged(t, n, p.ID(), rating.TierTrusted.ConnTag(), "and one that recovers is worth more again")
}

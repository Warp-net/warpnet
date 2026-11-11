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

package node_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/handler"
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/ratelimit"
	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

type staticBlocklist map[string]bool

func (b staticBlocklist) IsBlocklisted(peerId string) bool { return b[peerId] }

func newRelayTestNode(t *testing.T, opts ...warpnet.WarpOption) *node.WarpNode {
	t.Helper()
	opts = append(opts, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	n, err := node.NewWarpNode(t.Context(), nil, ratelimit.Settings{}, opts...)
	require.NoError(t, err)
	t.Cleanup(n.StopNode)
	return n
}

func newPlainHost(t *testing.T) host.Host {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func dialThroughRelay(h host.Host, relayAddr ma.Multiaddr, dest peer.ID) error {
	h.Network().(*swarm.Swarm).Backoff().Clear(dest)
	ctx, cancel := context.WithTimeout(network.WithAllowLimitedConn(context.Background(), "test"), 5*time.Second)
	defer cancel()
	return h.Connect(ctx, peer.AddrInfo{ID: dest, Addrs: []ma.Multiaddr{relayAddr}})
}

func TestRelayRefusesAPeerTheDestinationBlocked(t *testing.T) {
	relayNode := newRelayTestNode(t)
	mw := middleware.NewWarpMiddleware(
		relayNode.Node().ID(), nil, ratelimit.NewStreamLimiter(ratelimit.Settings{}, rating.NewPeersRatings()),
	)
	t.Cleanup(mw.Close)
	relayNode.SetStreamMiddlewares(mw.AuthMiddleware)
	relayNode.SetStreamHandlers(warpnet.WarpStreamHandler{
		Path:    event.PUBLIC_POST_RELAY_REFUSE,
		Handler: handler.StreamRefusePeerHandler(relayNode),
	})

	blocked, allowed := newPlainHost(t), newPlainHost(t)
	dest := newRelayTestNode(t, node.NewBlocklistOption(staticBlocklist{blocked.ID().String(): true}))

	relayInfo := peer.AddrInfo{ID: relayNode.Node().ID(), Addrs: relayNode.Node().Addrs()}
	require.NoError(t, dest.Connect(relayInfo))
	_, err := client.Reserve(t.Context(), dest.Node(), relayInfo)
	require.NoError(t, err)

	circuit, err := ma.NewMultiaddr(relayInfo.Addrs[0].String() + "/p2p/" + relayInfo.ID.String() + "/p2p-circuit")
	require.NoError(t, err)

	_ = dialThroughRelay(blocked, circuit, dest.Node().ID())

	require.Eventually(t, func() bool {
		err := dialThroughRelay(blocked, circuit, dest.Node().ID())
		return err != nil && strings.Contains(err.Error(), "PERMISSION_DENIED")
	}, 10*time.Second, 100*time.Millisecond)

	require.NotEqual(t, network.Connected, dest.Node().Network().Connectedness(blocked.ID()))

	require.NoError(t, dialThroughRelay(allowed, circuit, dest.Node().ID()))
	require.Eventually(t, func() bool {
		return dest.Node().Network().Connectedness(allowed.ID()) == network.Limited
	}, 5*time.Second, 50*time.Millisecond)
}

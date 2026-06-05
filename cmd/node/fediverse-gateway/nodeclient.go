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

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p"
	log "github.com/sirupsen/logrus"
)

// nodeClient is the gateway's libp2p connection into the Warpnet network: a
// minimal client peer (same PSK / transport / security as a member node) that
// joins through the network's bootstrap nodes and routes requests to whichever
// entry peer answers. It is agnostic to any specific node — the public routes
// relay to the user's own node — so the gateway stores no node/profile state.
type nodeClient struct {
	wn    *node.WarpNode
	peers []warpnet.WarpPeerID
}

// networkEntries are the peers the gateway uses to enter Warpnet: the configured
// network's bootstrap nodes (NODE_NETWORK, default "warpnet") plus an optional
// explicit GATEWAY_NODE_ADDR. None of them is privileged — any one is just an
// entry point into the network.
func networkEntries() ([]warpnet.WarpAddrInfo, error) {
	var entries []warpnet.WarpAddrInfo
	for _, s := range config.Config().Node.Bootstrap {
		ai, err := warpnet.AddrInfoFromString(s)
		if err != nil {
			log.Warnf("nodeclient: bad bootstrap %q: %v", s, err)
			continue
		}
		entries = append(entries, *ai)
	}
	if extra := envOr("GATEWAY_NODE_ADDR", ""); extra != "" {
		ai, err := warpnet.AddrInfoFromString(extra)
		if err != nil {
			return nil, fmt.Errorf("bad GATEWAY_NODE_ADDR: %w", err)
		}
		entries = append(entries, *ai)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no Warpnet entry peers (set NODE_NETWORK or GATEWAY_NODE_ADDR)")
	}
	return entries, nil
}

// connectNetwork joins Warpnet through the configured network and returns a
// node-agnostic client.
func connectNetwork(ctx context.Context) (*nodeClient, error) {
	entries, err := networkEntries()
	if err != nil {
		return nil, err
	}
	return newNodeClient(ctx, config.Config().Node.Network, entries)
}

func newNodeClient(ctx context.Context, network string, entries []warpnet.WarpAddrInfo) (*nodeClient, error) {
	psk, err := security.GeneratePSK(network, config.Config().Version)
	if err != nil {
		return nil, fmt.Errorf("nodeclient: psk: %w", err)
	}
	_, identity, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("nodeclient: identity: %w", err)
	}

	opts := make([]warpnet.WarpOption, 0, 3+len(node.CommonOptions))
	opts = append(opts,
		node.WarpIdentity(identity),
		libp2p.PrivateNetwork(warpnet.PSK(psk)),
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
	)
	opts = append(opts, node.CommonOptions...)

	wn, err := node.NewWarpNode(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("nodeclient: new node: %w", err)
	}

	var peers []warpnet.WarpPeerID
	for _, e := range entries {
		if cerr := wn.Connect(e); cerr != nil {
			log.Warnf("nodeclient: connect %s: %v", e.ID, cerr)
			continue
		}
		peers = append(peers, e.ID)
	}
	if len(peers) == 0 {
		wn.StopNode()
		return nil, fmt.Errorf("nodeclient: no Warpnet entry peer reachable")
	}
	log.Infof("nodeclient: joined Warpnet (%s) via %d entry peer(s)", network, len(peers))

	return &nodeClient{wn: wn, peers: peers}, nil
}

// request streams to entry peers in turn until one answers; the public handlers
// relay to the owning node, so any reachable peer can serve the route.
func (c *nodeClient) request(route stream.WarpRoute, payload any) ([]byte, error) {
	var lastErr error
	for _, p := range c.peers {
		bt, err := c.wn.Stream(p, route, payload)
		if err == nil {
			return bt, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("nodeclient: %s failed on all entry peers: %w", route, lastErr)
}

func (c *nodeClient) close() {
	if c != nil && c.wn != nil {
		c.wn.StopNode()
	}
}

// nodeSource reads any requested user's profile live from the Warpnet network
// via PUBLIC_GET_USER, so the gateway is agnostic to which user it serves and
// stores no profile of its own.
type nodeSource struct {
	client *nodeClient
}

func (s nodeSource) GetUser(preferredUsername string) (warpnetUser, bool) {
	bt, err := s.client.request(event.PUBLIC_GET_USER, event.GetUserEvent{UserId: preferredUsername})
	if err != nil {
		log.Errorf("nodesource: get user %s: %v", preferredUsername, err)
		return warpnetUser{}, false
	}
	var u domain.User
	if uerr := json.Unmarshal(bt, &u); uerr != nil || u.Id == "" {
		return warpnetUser{}, false
	}
	return warpnetUser{
		ID:                u.Id,
		PreferredUsername: u.Id,
		DisplayName:       u.Username,
		Summary:           u.Bio,
	}, true
}

// runProbe joins Warpnet and fetches the GATEWAY_USER profile — a smoke test of
// the node-agnostic connector path.
func runProbe() {
	user := envOr("GATEWAY_USER", "")
	if user == "" {
		log.Errorln("probe: set GATEWAY_USER (and optionally GATEWAY_NODE_ADDR)")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cl, err := connectNetwork(ctx)
	if err != nil {
		log.Errorf("probe: connect: %v", err)
		return
	}
	defer cl.close()

	u, ok := nodeSource{client: cl}.GetUser(user)
	if !ok {
		log.Errorln("probe: user not found / unreadable")
		return
	}
	log.Infof("probe: OK — user id=%s name=%q bio=%q", u.ID, u.DisplayName, u.Summary)
}

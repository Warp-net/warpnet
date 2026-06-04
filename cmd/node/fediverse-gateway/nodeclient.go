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
// dials a target node and calls its routes. Profile and follower state live on
// the node; the gateway keeps only keys.
type nodeClient struct {
	wn       *node.WarpNode
	targetID warpnet.WarpPeerID
}

func newNodeClient(ctx context.Context, network string, bootstrap []warpnet.WarpAddrInfo, target warpnet.WarpAddrInfo) (*nodeClient, error) {
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

	for _, b := range bootstrap {
		if cerr := wn.Connect(b); cerr != nil {
			log.Warnf("nodeclient: bootstrap %s: %v", b.ID, cerr)
		}
	}
	if err := wn.Connect(target); err != nil {
		wn.StopNode()
		return nil, fmt.Errorf("nodeclient: connect %s: %w", target.ID, err)
	}

	return &nodeClient{wn: wn, targetID: target.ID}, nil
}

func (c *nodeClient) request(route stream.WarpRoute, payload any) ([]byte, error) {
	return c.wn.Stream(c.targetID, route, payload)
}

func (c *nodeClient) close() {
	if c != nil && c.wn != nil {
		c.wn.StopNode()
	}
}

// nodeSource reads the bridged user's profile live from a Warpnet node via
// PUBLIC_GET_USER, so the gateway stores no profile of its own.
type nodeSource struct {
	client *nodeClient
	userID string
}

func (s nodeSource) GetUser(preferredUsername string) (warpnetUser, bool) {
	if preferredUsername != s.userID {
		return warpnetUser{}, false
	}
	bt, err := s.client.request(event.PUBLIC_GET_USER, event.GetUserEvent{UserId: s.userID})
	if err != nil {
		log.Errorf("nodesource: get user %s: %v", s.userID, err)
		return warpnetUser{}, false
	}
	var u domain.User
	if uerr := json.Unmarshal(bt, &u); uerr != nil || u.Id == "" {
		log.Errorf("nodesource: decode user %s: %v (%s)", s.userID, uerr, string(bt))
		return warpnetUser{}, false
	}
	return warpnetUser{
		ID:                u.Id,
		PreferredUsername: u.Id,
		DisplayName:       u.Username,
		Summary:           u.Bio,
	}, true
}

// runProbe connects to the Warpnet node at GATEWAY_NODE_ADDR and fetches the
// GATEWAY_USER profile — a node-agnostic smoke test of the connector path.
func runProbe() {
	nodeAddr := envOr("GATEWAY_NODE_ADDR", "")
	user := envOr("GATEWAY_USER", "")
	if nodeAddr == "" || user == "" {
		log.Errorln("probe: set GATEWAY_NODE_ADDR and GATEWAY_USER")
		return
	}
	target, err := warpnet.AddrInfoFromString(nodeAddr)
	if err != nil {
		log.Errorf("probe: bad GATEWAY_NODE_ADDR: %v", err)
		return
	}
	log.Infof("probe: dialing Warpnet node %s", target.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cl, err := newNodeClient(ctx, envOr("NODE_NETWORK", defaultNetwork), nil, *target)
	if err != nil {
		log.Errorf("probe: connect: %v", err)
		return
	}
	defer cl.close()

	src := nodeSource{client: cl, userID: user}
	u, ok := src.GetUser(user)
	if !ok {
		log.Errorln("probe: user not found / unreadable")
		return
	}
	log.Infof("probe: OK — user id=%s name=%q bio=%q", u.ID, u.DisplayName, u.Summary)
}

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

// echoOwnerID is the fixed owner user id of the deployed testnet echo node
// (see cmd/node/member/echo-member.go).
const echoOwnerID = "01KSGHBHKG0N77T6A3RZV8WSH5"

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

	opts := []warpnet.WarpOption{
		node.WarpIdentity(identity),
		libp2p.PrivateNetwork(warpnet.PSK(psk)),
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
	}
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

// echoAddrInfo builds the dial address of the deployed testnet echo node from
// its seed (NODE_SEED=echo) and well-known host:port (see deploy/).
func echoAddrInfo() (warpnet.WarpAddrInfo, error) {
	priv, err := security.GenerateKeyFromSeed([]byte("echo"))
	if err != nil {
		return warpnet.WarpAddrInfo{}, err
	}
	id, err := warpnet.IDFromPublicKey(priv.Public().(ed25519.PublicKey))
	if err != nil {
		return warpnet.WarpAddrInfo{}, err
	}
	info, err := warpnet.AddrInfoFromString(fmt.Sprintf("/ip4/130.94.88.38/tcp/4012/p2p/%s", id))
	if err != nil {
		return warpnet.WarpAddrInfo{}, err
	}
	return *info, nil
}

// runEchoProbe connects to the live testnet echo node and fetches its owner
// profile through nodeSource — a smoke test of the whole connector path.
func runEchoProbe() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	target, err := echoAddrInfo()
	if err != nil {
		log.Fatalf("probe: echo addr: %v", err)
	}
	log.Infof("probe: dialing testnet echo node %s", target.ID)

	cl, err := newNodeClient(ctx, "testnet", nil, target)
	if err != nil {
		log.Fatalf("probe: connect: %v", err)
	}
	defer cl.close()

	src := nodeSource{client: cl, userID: echoOwnerID}
	u, ok := src.GetUser(echoOwnerID)
	if !ok {
		log.Fatalln("probe: echo user not found / unreadable")
	}
	log.Infof("probe: OK — echo user id=%s name=%q bio=%q", u.ID, u.DisplayName, u.Summary)
}

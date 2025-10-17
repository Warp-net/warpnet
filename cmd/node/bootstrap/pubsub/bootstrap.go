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

package pubsub

import (
	"context"

	"github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	log "github.com/sirupsen/logrus"
)

type PubsubServerNodeConnector interface {
	Node() warpnet.P2PNode
	NodeInfo() warpnet.NodeInfo
	SelfStream(path stream.WarpRoute, data any) (_ []byte, err error)
	GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error)
}

type bootstrapPubSub struct {
	ctx    context.Context
	pubsub *pubsub.Gossip
}

func NewPubSubBootstrap(ctx context.Context, handlers ...pubsub.TopicHandler) *bootstrapPubSub {
	bps := &bootstrapPubSub{
		ctx: ctx,
	}
	bps.pubsub = pubsub.NewGossip(ctx, handlers...)
	return bps
}

func (g *bootstrapPubSub) Run(node PubsubServerNodeConnector) {
	if g.pubsub.IsGossipRunning() {
		return
	}

	if err := g.pubsub.Run(node); err != nil {
		log.Errorf("pubsub: failed to run: %v", err)
		return
	}
}

func (g *bootstrapPubSub) OwnerID() string {
	return warpnet.BootstrapOwner
}

func (g *bootstrapPubSub) Close() (err error) {
	return g.pubsub.Close()
}

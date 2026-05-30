/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as Published by
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
	"github.com/Warp-net/warpnet/event"
)

type PubsubServerNodeConnector interface {
	Node() warpnet.P2PNode
	NodeInfo() warpnet.NodeInfo
	SelfStream(path stream.WarpRoute, data any) (_ []byte, err error)
	GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error)
}

type moderatorPubSub struct {
	pubsub *pubsub.Gossip
}

func NewPubSub(ctx context.Context) *moderatorPubSub {
	mps := &moderatorPubSub{}

	mps.pubsub = pubsub.NewGossip(ctx, pubsub.NewDiscoveryRelayTopicHandler())
	return mps
}

func (g *moderatorPubSub) Run(node PubsubServerNodeConnector) error {
	if g.pubsub.IsGossipRunning() {
		return nil
	}

	return g.pubsub.Run(node)
}

// PublishUpdateToFollowers publishes an isolation verdict on the offender's
// followers topic. The shared gossip implementation owns the topic naming and
// envelope assembly.
func (g *moderatorPubSub) PublishUpdateToFollowers(ownerId, dest string, body any) error {
	return g.pubsub.PublishUpdateToFollowers(ownerId, dest, body)
}

// SubscribeReports listens on the global reports topic; the shared gossip
// implementation verifies each envelope and hands up one ReportEvent.
func (g *moderatorPubSub) SubscribeReports(h func(ev event.ReportEvent) error) error {
	return g.pubsub.SubscribeReports(h)
}

func (g *moderatorPubSub) Close() (err error) {
	return g.pubsub.Close()
}

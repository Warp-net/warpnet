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
	"fmt"
	"time"

	"github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/google/uuid"
)

const (
	// prefixes
	userUpdateTopicPrefix = "user-update"
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

func NewPubSub(ctx context.Context, handlers ...pubsub.TopicHandler) *moderatorPubSub {
	mps := &moderatorPubSub{}

	mps.pubsub = pubsub.NewGossip(ctx, handlers...)
	return mps
}

func (g *moderatorPubSub) Run(node PubsubServerNodeConnector) error {
	if g.pubsub.IsGossipRunning() {
		return nil
	}

	return g.pubsub.Run(node)
}

func (g *moderatorPubSub) PublishUpdateToFollowers(ownerId, dest string, bt []byte) (err error) {
	if g == nil || !g.pubsub.IsGossipRunning() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	topicName := fmt.Sprintf("%s-%s", userUpdateTopicPrefix, ownerId)

	body := json.RawMessage(bt)
	msg := event.Message{
		Body:        body,
		NodeId:      g.pubsub.NodeInfo().ID.String(),
		Destination: dest,
		Timestamp:   time.Now(),
		MessageId:   uuid.New().String(),
		Version:     "0.0.0", // TODO manage protocol versions properly
	}

	return g.pubsub.Publish(msg, topicName)
}

func (g *moderatorPubSub) Close() (err error) {
	return g.pubsub.Close()
}

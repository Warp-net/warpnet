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
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"sync"
	"time"
)

type PubsubServerNodeConnector interface {
	Node() warpnet.P2PNode
	NodeInfo() warpnet.NodeInfo
	SelfStream(path stream.WarpRoute, data any) (_ []byte, err error)
	GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error)
}

type moderatorPubSub struct {
	ctx    context.Context
	pubsub *pubsub.Gossip

	mx *sync.Mutex
}

func NewPubSubModerator(ctx context.Context) *moderatorPubSub {
	bps := &moderatorPubSub{
		ctx: ctx,
		mx:  new(sync.Mutex),
	}
	bps.pubsub = pubsub.NewGossip(ctx)
	return bps
}

func (g *moderatorPubSub) Run(node PubsubServerNodeConnector) {
	if g.pubsub.IsGossipRunning() {
		return
	}

	if err := g.pubsub.Run(node); err != nil {
		log.Errorf("moderator pubsub: failed to run: %v", err)
		return
	}
}

func (g *moderatorPubSub) PublishValidationRequest(bt []byte) (err error) {
	if g == nil || !g.pubsub.IsGossipRunning() {
		return warpnet.WarpError("moderator pubsub: service not initialized")
	}
	body := json.RawMessage(bt)

	msg := event.Message{
		Body:        body,
		Destination: event.INTERNAL_POST_NODE_VALIDATE,
		NodeId:      g.OwnerID(),
		Timestamp:   time.Now(),
		Version:     "0.0.0", // TODO manage protocol versions properly
		MessageId:   uuid.New().String(),
	}

	return g.pubsub.Publish(msg, pubsub.PubSubConsensusTopic)
}

func (g *moderatorPubSub) SubscribeModerationTopic() error {
	if g == nil || !g.pubsub.IsGossipRunning() {
		return warpnet.WarpError("moderator pubsub: service not initialized")
	}

	return g.pubsub.Subscribe(pubsub.TopicHandler{
		TopicName: pubsub.PubSubModerationTopic,
		Handler: func(data []byte) error {
			if !g.mx.TryLock() {
				return warpnet.WarpError("moderator pubsub: moderation topic is busy")
			}
			g.mx.Unlock()
			return g.pubsub.SelfPublish(data)
		},
	})
}

func (g *moderatorPubSub) SubscribeConsensusTopic() error {
	if g == nil || !g.pubsub.IsGossipRunning() {
		return warpnet.WarpError("moderator pubsub: service not initialized")
	}

	return g.pubsub.Subscribe(pubsub.TopicHandler{
		TopicName: pubsub.PubSubConsensusTopic,
		Handler:   g.pubsub.SelfPublish,
	})
}

func (g *moderatorPubSub) GetConsensusTopicSubscribers() []warpnet.WarpAddrInfo {
	if g == nil || !g.pubsub.IsGossipRunning() {
		panic("moderator pubsub: get consensus subscribers: service not initialized")
	}

	return g.pubsub.Subscribers(pubsub.PubSubConsensusTopic)
}

func (g *moderatorPubSub) OwnerID() string {
	return warpnet.ModeratorOwner
}

func (g *moderatorPubSub) Close() (err error) {
	return g.pubsub.Close()
}

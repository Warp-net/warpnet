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
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/google/uuid"
	jsoniter "github.com/json-iterator/go"
	log "github.com/sirupsen/logrus"
	"time"
)

type bootstrapPubSub struct {
	ctx    context.Context
	pubsub *gossip
}

func NewPubSubBootstrap(ctx context.Context, handlers ...TopicHandler) *bootstrapPubSub {
	bps := &bootstrapPubSub{
		ctx: ctx,
	}
	bps.pubsub = newGossip(ctx, handlers...)
	return bps
}

func (g *bootstrapPubSub) Run(node PubsubServerNodeConnector) {
	if g.pubsub.isGossipRunning() {
		return
	}

	if err := g.pubsub.run(node); err != nil {
		log.Errorf("pubsub: failed to run: %v", err)
		return
	}
}

func (g *bootstrapPubSub) PublishValidationRequest(bt []byte) (err error) {
	if g == nil || !g.pubsub.isGossipRunning() {
		return warpnet.WarpError("bootstrap pubsub: service not initialized")
	}

	msg := event.Message{
		Body:        jsoniter.RawMessage(bt),
		Destination: event.INTERNAL_POST_NODE_VALIDATE,
		NodeId:      g.OwnerID(),
		Timestamp:   time.Now(),
		Version:     "0.0.0", // TODO manage protocol versions properly
		MessageId:   uuid.New().String(),
	}

	return g.pubsub.publish(msg, pubSubConsensusTopic)
}

func (g *bootstrapPubSub) SubscribeConsensusTopic() error {
	if g == nil || !g.pubsub.isGossipRunning() {
		return warpnet.WarpError("bootstrap pubsub: service not initialized")
	}

	return g.pubsub.subscribe(TopicHandler{
		TopicName: pubSubConsensusTopic,
		Handler:   g.pubsub.selfStream,
	})
}

func (g *bootstrapPubSub) GetConsensusTopicSubscribers() []warpnet.WarpAddrInfo {
	if g == nil || !g.pubsub.isGossipRunning() {
		panic("bootstrap pubsub: get consensus subscribers: service not initialized")
	}

	return g.pubsub.subscribers(pubSubConsensusTopic)
}

func (g *bootstrapPubSub) OwnerID() string {
	return warpnet.BootstrapOwner
}

func (g *bootstrapPubSub) Close() (err error) {
	return g.pubsub.close()
}

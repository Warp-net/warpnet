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
	log "github.com/sirupsen/logrus"
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

var NewBootstrapDiscoveryTopicHandler = pubsub.NewDiscoveryTopicHandler

type memberPubSub struct {
	ctx    context.Context
	pubsub *pubsub.Gossip
}

func NewPubSub(ctx context.Context, handlers ...pubsub.TopicHandler) *memberPubSub {
	mps := &memberPubSub{
		ctx: ctx,
	}

	mps.pubsub = pubsub.NewGossip(ctx, handlers...)
	return mps
}

func (g *memberPubSub) Run(node PubsubServerNodeConnector) {
	if g.pubsub.IsGossipRunning() {
		return
	}

	if err := g.pubsub.Run(node); err != nil {
		log.Errorf("pubsub: failed to run: %v", err)
		return
	}
}

func (g *memberPubSub) OwnerID() string {
	if g == nil || g.pubsub == nil {
		return ""
	}
	return g.pubsub.NodeInfo().OwnerId
}

func (g *memberPubSub) NodeID() string {
	if g == nil || g.pubsub == nil {
		return ""
	}
	return g.pubsub.NodeInfo().ID.String()
}

func PrefollowHandlers(userIds ...string) (handlers []pubsub.TopicHandler) {
	for _, userId := range userIds {
		handler := pubsub.TopicHandler{
			TopicName: fmt.Sprintf("%s-%s", userUpdateTopicPrefix, userId),
			Handler:   nil,
		}
		handlers = append(handlers, handler)
	}

	return handlers
}

// SubscribeUserUpdate - follow someone
func (g *memberPubSub) SubscribeUserUpdate(userId string) (err error) {
	if g == nil || g.pubsub == nil || !g.pubsub.IsGossipRunning() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	ownerId := g.pubsub.NodeInfo().ID.String()
	if ownerId == userId {
		return warpnet.WarpError("pubsub: can't subscribe to own user")
	}

	handler := pubsub.TopicHandler{
		TopicName: fmt.Sprintf("%s-%s", userUpdateTopicPrefix, userId),
		Handler:   g.pubsub.SelfPublish,
	}
	return g.pubsub.Subscribe(handler)
}

// UnsubscribeUserUpdate - unfollow someone
func (g *memberPubSub) UnsubscribeUserUpdate(userId string) (err error) {
	if g == nil || !g.pubsub.IsGossipRunning() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	topicName := fmt.Sprintf("%s-%s", userUpdateTopicPrefix, userId)
	return g.pubsub.Unsubscribe(topicName)
}

// PublishUpdateToFollowers - Publish for followers
func (g *memberPubSub) PublishUpdateToFollowers(ownerId, dest string, bt []byte) (err error) {
	if g == nil || !g.pubsub.IsGossipRunning() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	topicName := fmt.Sprintf("%s-%s", userUpdateTopicPrefix, ownerId)

	body := json.RawMessage(bt)
	msg := event.Message{
		Body:        body,
		NodeId:      g.NodeID(),
		Destination: dest,
		Timestamp:   time.Now(),
		MessageId:   uuid.New().String(),
		Version:     "0.0.0", // TODO manage protocol versions properly
	}

	return g.pubsub.Publish(msg, topicName)
}

func (g *memberPubSub) Close() (err error) {
	return g.pubsub.Close()
}

// Gossip returns the underlying Gossip instance for CRDT integration
func (g *memberPubSub) Gossip() *pubsub.Gossip {
	if g == nil {
		return nil
	}
	return g.pubsub
}

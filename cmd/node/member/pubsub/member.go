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
	"errors"
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

type memberPubSub struct {
	ctx    context.Context
	pubsub *pubsub.Gossip
}

type DiscoveryHandler func(warpnet.WarpAddrInfo)

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

func PrefollowUsers(userIds ...string) (handlers []pubsub.TopicHandler) {
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
		return warpnet.WarpError("pubsub: can't Subscribe to own user")
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

func (g *memberPubSub) runPeerInfoPublishing() {
	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()

	log.Infoln("pubsub: Publisher started")
	defer log.Infoln("pubsub: Publisher stopped")

	if err := g.PublishPeerInfo(); err != nil { // initial Publishing
		log.Errorf("pubsub: failed to Publish peer info: %v", err)
	}

	for {
		if !g.pubsub.IsGossipRunning() {
			return
		}

		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			if err := g.PublishPeerInfo(); err != nil {
				log.Errorf("pubsub: failed to Publish peer info: %v", err)
				continue
			}
		}
	}
}

const publishPeerInfoLimit = 10

func (g *memberPubSub) PublishPeerInfo() error {
	myInfo := g.pubsub.NodeInfo()
	addrInfosMessage := []warpnet.WarpPubInfo{{
		ID:    myInfo.ID,
		Addrs: myInfo.Addresses,
	}}

	limit := publishPeerInfoLimit
	for _, pi := range g.pubsub.Subscribers(pubsub.PubSubDiscoveryTopic) {
		if limit == 0 {
			break
		}
		if pi.ID.String() == "" {
			continue
		}
		addrInfo := warpnet.WarpPubInfo{ID: pi.ID, Addrs: make([]string, 0, len(pi.Addrs))}
		for _, addr := range pi.Addrs {
			addrInfo.Addrs = append(addrInfo.Addrs, addr.String())
		}
		addrInfosMessage = append(addrInfosMessage, addrInfo)
		limit--
	}

	data, err := json.Marshal(addrInfosMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal peer info message: %v", err)
	}

	msg := event.Message{
		Body:        json.RawMessage(data),
		MessageId:   uuid.New().String(),
		NodeId:      g.pubsub.NodeInfo().ID.String(),
		Destination: "none",
		Timestamp:   time.Now(),
		Version:     "0.0.0", // TODO
	}

	err = g.pubsub.Publish(msg, pubsub.PubSubDiscoveryTopic)
	if errors.Is(err, pubsub.ErrTopicClosed) {
		return nil
	}
	return err
}

func (g *memberPubSub) Close() (err error) {
	return g.pubsub.Close()
}

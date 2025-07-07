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
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/google/uuid"
	jsoniter "github.com/json-iterator/go"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	log "github.com/sirupsen/logrus"
	"time"
)

type PubsubServerNodeConnector interface {
	Node() warpnet.P2PNode
	NodeInfo() warpnet.NodeInfo
	SelfStream(path stream.WarpRoute, data any) (_ []byte, err error)
	GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error)
}

type memberPubSub struct {
	ctx    context.Context
	pubsub *gossip
}

type DiscoveryHandler func(warpnet.WarpAddrInfo)

func NewPubSub(ctx context.Context, handlers ...TopicHandler) *memberPubSub {
	mps := &memberPubSub{
		ctx: ctx,
	}

	mps.pubsub = newGossip(ctx, handlers...)
	return mps
}

func (g *memberPubSub) Run(node PubsubServerNodeConnector) {
	if g.pubsub.isGossipRunning() {
		return
	}

	if err := g.pubsub.run(node); err != nil {
		log.Errorf("pubsub: failed to run: %v", err)
		return
	}
}

func (g *memberPubSub) OwnerID() string {
	if g == nil || g.pubsub == nil {
		return ""
	}
	return g.pubsub.nodeInfo().OwnerId
}

func (g *memberPubSub) NodeID() string {
	if g == nil || g.pubsub == nil {
		return ""
	}
	return g.pubsub.nodeInfo().ID.String()
}

func PrefollowUsers(userIds ...string) (handlers []TopicHandler) {
	for _, userId := range userIds {
		handler := TopicHandler{
			TopicName: fmt.Sprintf("%s-%s", userUpdateTopicPrefix, userId),
			Handler:   nil,
		}
		handlers = append(handlers, handler)
	}

	return handlers
}

func (g *memberPubSub) SubscribeConsensusTopic() error {
	if g == nil || !g.pubsub.isGossipRunning() {
		return warpnet.WarpError("member pubsub: service not initialized")
	}

	return g.pubsub.subscribe(TopicHandler{
		TopicName: pubSubConsensusTopic,
		Handler:   g.pubsub.selfStream,
	})
}

func (g *memberPubSub) GetConsensusTopicSubscribers() []warpnet.WarpAddrInfo {
	return g.pubsub.subscribers(pubSubConsensusTopic)
}

// SubscribeUserUpdate - follow someone
func (g *memberPubSub) SubscribeUserUpdate(userId string) (err error) {
	if g == nil || g.pubsub == nil || !g.pubsub.isGossipRunning() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	ownerId := g.pubsub.nodeInfo().ID.String()
	if ownerId == userId {
		return warpnet.WarpError("pubsub: can't subscribe to own user")
	}

	handler := TopicHandler{
		TopicName: fmt.Sprintf("%s-%s", userUpdateTopicPrefix, userId),
		Handler:   g.pubsub.selfStream,
	}
	return g.pubsub.subscribe(handler)
}

// UnsubscribeUserUpdate - unfollow someone
func (g *memberPubSub) UnsubscribeUserUpdate(userId string) (err error) {
	if g == nil || !g.pubsub.isGossipRunning() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	topicName := fmt.Sprintf("%s-%s", userUpdateTopicPrefix, userId)
	return g.pubsub.unsubscribe(topicName)
}

func (g *memberPubSub) PublishValidationRequest(bt []byte) (err error) {
	if g == nil || !g.pubsub.isGossipRunning() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	body := jsoniter.RawMessage(bt)

	msg := event.Message{
		Body:        body,
		Destination: event.INTERNAL_POST_NODE_VALIDATE,
		NodeId:      g.NodeID(),
		Timestamp:   time.Now(),
		Version:     "0.0.0", // TODO manage protocol versions properly
		MessageId:   uuid.New().String(),
	}
	return g.pubsub.publish(msg, pubSubConsensusTopic)
}

func (g *memberPubSub) PublishModerationRequest(bt []byte) (err error) {
	if g == nil || !g.pubsub.isGossipRunning() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	body := jsoniter.RawMessage(bt)

	msg := event.Message{
		Body:        body,
		Destination: event.INTERNAL_POST_MODERATE,
		NodeId:      g.NodeID(),
		Timestamp:   time.Now(),
		Version:     "0.0.0", // TODO manage protocol versions properly
		MessageId:   uuid.New().String(),
	}
	return g.pubsub.publish(msg, pubSubModerationTopic)
}

// PublishUpdateToFollowers - publish for followers
func (g *memberPubSub) PublishUpdateToFollowers(ownerId, dest string, bt []byte) (err error) {
	if g == nil || !g.pubsub.isGossipRunning() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	topicName := fmt.Sprintf("%s-%s", userUpdateTopicPrefix, ownerId)

	body := jsoniter.RawMessage(bt)
	msg := event.Message{
		Body:        body,
		NodeId:      g.NodeID(),
		Destination: dest,
		Timestamp:   time.Now(),
		MessageId:   uuid.New().String(),
		Version:     "0.0.0", // TODO manage protocol versions properly
	}

	return g.pubsub.publish(msg, topicName)
}

func (g *memberPubSub) runPeerInfoPublishing() {
	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()

	log.Infoln("pubsub: publisher started")
	defer log.Infoln("pubsub: publisher stopped")

	if err := g.publishPeerInfo(); err != nil { // initial publishing
		log.Errorf("pubsub: failed to publish peer info: %v", err)
	}

	for {
		if !g.pubsub.isGossipRunning() {
			return
		}

		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			if err := g.publishPeerInfo(); err != nil {
				log.Errorf("pubsub: failed to publish peer info: %v", err)
				continue
			}
		}
	}
}

func (g *memberPubSub) publishPeerInfo() error {
	myInfo := g.pubsub.nodeInfo()
	addrInfosMessage := []warpnet.WarpPubInfo{{
		ID:    myInfo.ID,
		Addrs: myInfo.Addresses,
	}}

	limit := publishPeerInfoLimit
	for _, pi := range g.pubsub.notSubscribers(pubSubDiscoveryTopic) {
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

	data, err := json.JSON.Marshal(addrInfosMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal peer info message: %v", err)
	}

	msg := event.Message{
		Body:        jsoniter.RawMessage(data),
		MessageId:   uuid.New().String(),
		NodeId:      g.pubsub.nodeInfo().ID.String(),
		Destination: "none",
		Timestamp:   time.Now(),
		Version:     "0.0.0", // TODO
	}

	err = g.pubsub.publish(msg, pubSubDiscoveryTopic)
	if errors.Is(err, pubsub.ErrTopicClosed) {
		return nil
	}
	return err
}

func (g *memberPubSub) Close() (err error) {
	return g.pubsub.close()
}

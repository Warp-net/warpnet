/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

package pubsub

import (
	"context"
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	log "github.com/sirupsen/logrus"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type topicPrefix string

func (t topicPrefix) isIn(s string) bool {
	return strings.HasPrefix(s, string(t))
}

const (
	// full names
	pubSubDiscoveryTopic     = "peer-discovery"
	leaderAnnouncementsTopic = "leader-announcements"

	// prefixes
	userUpdateTopicPrefix topicPrefix = "user-update"
)

type PubsubServerNodeConnector interface {
	Node() warpnet.P2PNode
	NodeInfo() warpnet.NodeInfo
}

type PubsubClientNodeStreamer interface {
	ClientStream(nodeId string, path string, data any) (_ []byte, err error)
}

type PubsubFollowingStorer interface {
	GetFollowees(userId string, limit *uint64, cursor *string) ([]domain.Following, string, error)
}

type warpPubSub struct {
	ctx        context.Context
	pubsub     *pubsub.PubSub
	serverNode PubsubServerNodeConnector
	clientNode PubsubClientNodeStreamer
	followRepo PubsubFollowingStorer

	ownerId string

	mx               *sync.RWMutex
	subs             []*pubsub.Subscription
	relayCancelFuncs map[string]pubsub.RelayCancelFunc
	topics           map[string]*pubsub.Topic
	discoveryHandler discovery.DiscoveryHandler

	isRunning *atomic.Bool
}

func NewPubSub(
	ctx context.Context,
	followRepo PubsubFollowingStorer,
	ownerId string,
	discoveryHandler discovery.DiscoveryHandler,
) *warpPubSub {

	g := &warpPubSub{
		ctx:              ctx,
		pubsub:           nil,
		serverNode:       nil,
		clientNode:       nil,
		discoveryHandler: discoveryHandler,
		mx:               new(sync.RWMutex),
		subs:             []*pubsub.Subscription{},
		topics:           map[string]*pubsub.Topic{},
		relayCancelFuncs: map[string]pubsub.RelayCancelFunc{},
		followRepo:       followRepo,
		ownerId:          ownerId,
		isRunning:        new(atomic.Bool),
	}

	return g
}

func NewPubSubBootstrap(
	ctx context.Context,
	discoveryHandler discovery.DiscoveryHandler,
) *warpPubSub {
	return NewPubSub(ctx, nil, warpnet.BootstrapOwner, discoveryHandler)
}

func (g *warpPubSub) Run(
	serverNode PubsubServerNodeConnector, clientNode PubsubClientNodeStreamer,
) {
	if g.isRunning.Load() {
		return
	}

	g.clientNode = clientNode
	g.serverNode = serverNode

	if err := g.runPubSub(serverNode); err != nil {
		log.Errorf("pubsub: failed to run: %v", err)
		return
	}
	if err := g.subscribeFollowees(); err != nil {
		log.Errorf("pubsub: presubscribe: %v", err)
		return
	}

	go func() {
		if err := g.runListener(); err != nil {
			log.Errorf("pubsub: listener: %v", err)
			return
		}
		log.Infoln("pubsub: listener stopped")
	}()
}

func (g *warpPubSub) runListener() error {
	if g == nil {
		return warpnet.WarpError("pubsub: service not initialized properly")
	}
	for {
		if !g.isRunning.Load() {
			return nil
		}
		if err := g.ctx.Err(); err != nil {
			return err
		}
		g.mx.RLock()
		subs := make([]*pubsub.Subscription, len(g.subs))
		copy(subs, g.subs)
		g.mx.RUnlock()

		for _, sub := range subs { // TODO scale this!
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)

			msg, err := sub.Next(ctx)
			cancel()
			if errors.Is(err, pubsub.ErrSubscriptionCancelled) {
				continue
			}
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				continue
			}
			if err != nil {
				log.Errorf("pubsub: failed to listen subscription to topic: %v", err)
				continue
			}
			if msg.Topic == nil {
				continue
			}

			// full topic names match
			switch strings.TrimSpace(*msg.Topic) {
			case pubSubDiscoveryTopic:
				g.handlePubSubDiscovery(msg)
				continue
			case leaderAnnouncementsTopic:
				log.WithField("topic", *msg.Topic).Info("pubsub: leader announcement:", string(msg.Data))
				continue
			default:
			}

			// topic prefixes match
			switch {
			case userUpdateTopicPrefix.isIn(*msg.Topic):
				if err := g.handleUserUpdate(msg); err != nil {
					log.Infof("pubsub: user update error: %v", err)
				}
				continue
			default:
				log.Warnf("pubsub: unknown topic: %s, message: %s", *msg.Topic, string(msg.Data))
			}
		}
	}
}

func (g *warpPubSub) runPubSub(n PubsubServerNodeConnector) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pubsub: recovered from panic: %v", r)
		}
	}()
	if g == nil {
		return warpnet.WarpError("pubsub: service not initialized properly")
	}

	g.pubsub, err = pubsub.NewGossipSub(g.ctx, n.Node())
	if err != nil {
		return err
	}
	g.isRunning.Store(true)

	if err := g.subscribe(pubSubDiscoveryTopic); err != nil {
		return err
	}
	if err := g.subscribe(leaderAnnouncementsTopic); err != nil {
		return err
	}

	go g.runPeerInfoPublishing()

	log.Infoln("pubsub: started")

	return nil
}

func (g *warpPubSub) subscribeFollowees() error {
	if g == nil {
		return warpnet.WarpError("pubsub: service not initialized properly")
	}
	if g.ownerId == "" {
		return nil
	}
	if g.followRepo == nil {
		return nil
	}

	var (
		nextCursor string
		limit      = uint64(20)
	)
	for {
		followees, cur, err := g.followRepo.GetFollowees(g.ownerId, &limit, &nextCursor)
		if err != nil {
			return err
		}
		for _, f := range followees {
			if err := g.SubscribeUserUpdate(f.Followee); err != nil {
				return err
			}

		}
		if len(followees) < int(limit) {
			break
		}
		nextCursor = cur
	}

	log.Infoln("pubsub: followees presubscribed")
	return nil
}

func (g *warpPubSub) subscribe(topics ...string) (err error) {
	if g == nil || !g.isRunning.Load() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	g.mx.Lock()
	defer g.mx.Unlock()

	for _, topicName := range topics {
		if topicName == "" {
			return warpnet.WarpError("pubsub: topic name is empty")
		}

		topic, ok := g.topics[topicName]
		if !ok {
			topic, err = g.pubsub.Join(topicName)
			if err != nil {
				return err
			}
			g.topics[topicName] = topic
		}

		relayCancel, err := topic.Relay()
		if err != nil {
			return err
		}

		sub, err := topic.Subscribe()
		if err != nil {
			return err
		}

		log.Infof("pubsub: subscribed to topic: %s", topicName)

		g.relayCancelFuncs[topicName] = relayCancel
		g.subs = append(g.subs, sub)
	}

	return nil
}

func (g *warpPubSub) GetSubscribers() []warpnet.WarpPeerID {
	g.mx.RLock()
	defer g.mx.RUnlock()

	topicName := fmt.Sprintf("%s-%s", userUpdateTopicPrefix, g.ownerId)
	topic, ok := g.topics[topicName]
	if !ok {
		return nil
	}

	return topic.ListPeers()
}

func (g *warpPubSub) unsubscribe(topics ...string) (err error) {
	if g == nil || !g.isRunning.Load() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	g.mx.Lock()
	defer g.mx.Unlock()

	for _, topicName := range topics {
		topic, ok := g.topics[topicName]
		if !ok {
			return nil
		}

		for i, s := range g.subs {
			if s.Topic() == topicName {
				s.Cancel()
				g.subs = slices.Delete(g.subs, i, i+1)
				break
			}
		}

		if err = topic.Close(); err != nil {
			return err
		}
		delete(g.topics, topicName)

		if _, ok := g.relayCancelFuncs[topicName]; ok {
			g.relayCancelFuncs[topicName]()
		}
		delete(g.relayCancelFuncs, topicName)
	}

	return err
}

func (g *warpPubSub) publish(msg event.Message, topics ...string) (err error) {
	if g == nil || !g.isRunning.Load() {
		return warpnet.WarpError("pubsub: service not initialized")
	}

	g.mx.Lock()
	defer g.mx.Unlock()

	for _, topicName := range topics {
		topic, ok := g.topics[topicName]
		if !ok {
			topic, err = g.pubsub.Join(topicName)
			if err != nil {
				return err
			}
			g.topics[topicName] = topic
		}

		data, err := json.JSON.Marshal(msg)
		if err != nil {
			log.Errorf("pubsub: failed to marshal owner update message: %v", err)
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		err = topic.Publish(ctx, data)
		cancel()
		if err != nil && !errors.Is(err, pubsub.ErrTopicClosed) {
			log.Errorf("pubsub: failed to publish owner update message: %v", err)
			return err
		}
	}

	return nil
}

// SubscribeUserUpdate - follow someone
func (g *warpPubSub) SubscribeUserUpdate(userId string) (err error) {
	if g == nil || !g.isRunning.Load() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	if g.ownerId == userId {
		return warpnet.WarpError("pubsub: can't subscribe to own user")
	}

	topicName := fmt.Sprintf("%s-%s", userUpdateTopicPrefix, userId)
	return g.subscribe(topicName)
}

// UnsubscribeUserUpdate - unfollow someone
func (g *warpPubSub) UnsubscribeUserUpdate(userId string) (err error) {
	if g == nil || !g.isRunning.Load() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	topicName := fmt.Sprintf("%s-%s", userUpdateTopicPrefix, userId)
	return g.unsubscribe(topicName)
}

func (g *warpPubSub) handleUserUpdate(msg *pubsub.Message) error {
	var simulatedMessage event.Message
	if err := json.JSON.Unmarshal(msg.Data, &simulatedMessage); err != nil {
		log.Errorf("pubsub: failed to decode user update message: %v %s", err, msg.Data)
		return err
	}
	if simulatedMessage.NodeId == g.serverNode.NodeInfo().ID.String() {
		return nil
	}

	if simulatedMessage.Path == "" {
		log.Errorln("pubsub: user update message has no path", simulatedMessage.Path)
		return fmt.Errorf("pubsub: user update message has no path: %s", string(msg.Data))
	}
	if simulatedMessage.Body == nil {
		return nil
	}
	if stream.WarpRoute(simulatedMessage.Path).IsGet() { // only store data
		return nil
	}

	if g.clientNode == nil {
		return nil
	}

	log.Infof("pubsub: new user update: %s", *simulatedMessage.Body)

	_, err := g.clientNode.ClientStream( // send it to self
		g.serverNode.NodeInfo().ID.String(),
		simulatedMessage.Path,
		*simulatedMessage.Body,
	)
	return err
}

func (g *warpPubSub) handlePubSubDiscovery(msg *pubsub.Message) {
	var discoveryAddrInfos []warpnet.WarpAddrInfo

	outerErr := json.JSON.Unmarshal(msg.Data, &discoveryAddrInfos)
	if outerErr != nil {
		var single warpnet.WarpAddrInfo
		if innerErr := json.JSON.Unmarshal(msg.Data, &single); innerErr != nil {
			log.Errorf("pubsub: discovery: failed to decode discovery message: %v %s", innerErr, msg.Data)
			return
		}
		discoveryAddrInfos = []warpnet.WarpAddrInfo{single}
	}
	if len(discoveryAddrInfos) == 0 {
		return
	}

	for _, info := range discoveryAddrInfos {
		if info.ID == "" {
			log.Errorf("pubsub: discovery: message has no ID: %s", string(msg.Data))
			return
		}
		if info.ID == g.serverNode.NodeInfo().ID {
			return
		}

		peerInfo := warpnet.PeerAddrInfo{
			ID:    info.ID,
			Addrs: make([]warpnet.WarpAddress, 0, len(info.Addrs)),
		}

		for _, addr := range info.Addrs {
			ma, _ := warpnet.NewMultiaddr(addr)
			peerInfo.Addrs = append(peerInfo.Addrs, ma)
		}

		if g.discoveryHandler != nil {
			g.discoveryHandler(peerInfo)
		}
	}
}

// PublishOwnerUpdate - publish for followers
func (g *warpPubSub) PublishOwnerUpdate(ownerId string, msg event.Message) (err error) {
	if g == nil || !g.isRunning.Load() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	topicName := fmt.Sprintf("%s-%s", userUpdateTopicPrefix, ownerId)

	return g.publish(msg, topicName)
}

func (g *warpPubSub) runPeerInfoPublishing() {
	g.mx.RLock()
	discTopic, ok := g.topics[pubSubDiscoveryTopic]
	g.mx.RUnlock()
	if !ok {
		log.Fatalf("pubsub: discovery topic not found: %s", pubSubDiscoveryTopic)
	}
	defer func() {
		_ = discTopic.Close()
	}()

	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()

	log.Infoln("pubsub: publisher started")
	defer log.Infoln("pubsub: publisher stopped")

	if err := g.publishPeerInfo(discTopic); err != nil { // initial publishing
		log.Errorf("pubsub: failed to publish peer info: %v", err)
	}

	for {
		if !g.isRunning.Load() {
			return
		}

		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			if err := g.publishPeerInfo(discTopic); err != nil {
				log.Errorf("pubsub: failed to publish peer info: %v", err)
				continue
			}
		}
	}
}

const publishPeerInfoLimit = 10

func (g *warpPubSub) publishPeerInfo(topic *pubsub.Topic) error {
	myInfo := g.serverNode.NodeInfo()
	addrInfosMessage := []warpnet.WarpAddrInfo{{
		ID:    myInfo.ID,
		Addrs: myInfo.Addresses,
	}}

	limit := publishPeerInfoLimit
	for _, id := range g.serverNode.Node().Peerstore().PeersWithAddrs() {
		if limit == 0 {
			break
		}
		pi := g.serverNode.Node().Peerstore().PeerInfo(id)
		addrInfo := warpnet.WarpAddrInfo{ID: pi.ID, Addrs: make([]string, 0, len(pi.Addrs))}
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
	err = topic.Publish(g.ctx, data)
	if err != nil && !errors.Is(err, pubsub.ErrTopicClosed) {
		return err
	}
	return nil
}

func (g *warpPubSub) Close() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	if !g.isRunning.Load() {
		return
	}

	g.mx.Lock()
	defer g.mx.Unlock()

	for t := range g.relayCancelFuncs {
		g.relayCancelFuncs[t]()
	}

	for _, sub := range g.subs {
		sub.Cancel()
	}

	for _, topic := range g.topics {
		_ = topic.Close()
	}

	g.isRunning.Store(false)

	g.pubsub = nil
	g.relayCancelFuncs = nil
	g.topics = nil
	g.subs = nil
	log.Infoln("pubsub: closed")
	return
}

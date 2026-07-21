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
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/google/uuid"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	log "github.com/sirupsen/logrus"
)

const (
	pubSubDiscoveryTopic = "/warpnet/discovery/1.0.0"

	ErrPubsubNotInit      warpnet.WarpError = "gossip: service not initialized"
	ErrAlreadyRunning     warpnet.WarpError = "gossip: pubsub is already running"
	ErrListenerMalformed  warpnet.WarpError = "gossip: pubsub listener not initialized properly"
	ErrPubsubEmptyTopic   warpnet.WarpError = "gossip: topic name is empty"
	ErrPubsubNoPathFound  warpnet.WarpError = "gossip: user update message has no path"
	ErrPubsubEmptyMessage warpnet.WarpError = "gossip: empty message"
)

type GossipNodeConnector interface {
	Node() warpnet.P2PNode
	NodeInfo() warpnet.NodeInfo
	SelfStream(path stream.WarpRoute, data any) (_ []byte, err error)
}

type topicHandler func(data []byte) error

type Gossip struct {
	ctx    context.Context
	pubsub *pubsub.PubSub
	node   GossipNodeConnector

	mx               *sync.RWMutex
	subs             []*pubsub.Subscription
	relayCancelFuncs map[string]pubsub.RelayCancelFunc
	topics           map[string]*pubsub.Topic
	// handlersMap holds every handler registered for a topic. A topic may have
	// more than one handler (e.g. discovery + offline-retry both watch the
	// discovery topic); each received message is dispatched to all of them.
	// A topic present with no non-nil handler falls back to SelfPublish.
	handlersMap map[string][]topicHandler
	isRunning   *atomic.Bool
	privKey     ed25519.PrivateKey
}

type TopicHandler struct {
	TopicName string
	Handler   topicHandler
}

func NewGossip(
	ctx context.Context,
	handlers ...TopicHandler,
) *Gossip {
	handlersMap := make(map[string][]topicHandler)
	for _, h := range handlers {
		// Record the topic even for a nil handler so Run still subscribes to it
		// (nil handler => default SelfPublish dispatch).
		if _, ok := handlersMap[h.TopicName]; !ok {
			handlersMap[h.TopicName] = nil
		}
		if h.Handler != nil {
			handlersMap[h.TopicName] = append(handlersMap[h.TopicName], h.Handler)
		}
	}

	return &Gossip{
		ctx:              ctx,
		mx:               new(sync.RWMutex),
		subs:             []*pubsub.Subscription{},
		handlersMap:      handlersMap,
		topics:           map[string]*pubsub.Topic{},
		relayCancelFuncs: map[string]pubsub.RelayCancelFunc{},
		isRunning:        new(atomic.Bool),
	}
}

func (g *Gossip) Run(node GossipNodeConnector) (err error) {
	if g.isRunning.Load() {
		return ErrAlreadyRunning
	}

	g.node = node

	g.privKey, err = g.node.Node().Peerstore().PrivKey(g.node.Node().ID()).Raw()
	if err != nil {
		return err
	}

	if err := g.runGossip(); err != nil {
		return fmt.Errorf("gossip: failed to run: %w", err)
	}

	// Handlers were already registered in NewGossip; here we only need to join
	// and subscribe to each distinct topic once.
	g.mx.RLock()
	topicNames := make([]string, 0, len(g.handlersMap))
	for name := range g.handlersMap {
		topicNames = append(topicNames, name)
	}
	g.mx.RUnlock()

	for _, name := range topicNames {
		if err := g.joinTopic(name); err != nil {
			return fmt.Errorf("gossip: presubscribe: %w", err)
		}
	}

	go func() {
		if err := g.runListener(); err != nil {
			log.Errorf("gossip: listener: %v", err)
			return
		}
		log.Infoln("gossip: listener stopped")
	}()

	return nil
}

func (g *Gossip) runListener() error {
	if g == nil {
		return ErrListenerMalformed
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
				log.Errorf("gossip: failed to listen subscription to topic: %v", err)
				continue
			}
			if msg == nil || msg.Topic == nil {
				continue
			}

			log.Debugf("gossip: received message: %s", string(msg.Data))

			topicName := strings.TrimSpace(*msg.Topic)
			g.mx.RLock()
			handlers := g.handlersMap[topicName]
			g.mx.RUnlock()
			if len(handlers) == 0 {
				// default behavior: no registered handler for this topic
				if err := g.SelfPublish(msg.Data); err != nil {
					log.Errorf("gossip: self stream: %v", err)
				}
				continue
			}
			for _, handlerF := range handlers {
				if handlerF == nil {
					continue
				}
				if err := handlerF(msg.Data); err != nil {
					log.Errorf(
						"gossip: failed to handle peer %s message from topic %s: %v",
						msg.ReceivedFrom.String(), *msg.Topic, err,
					)
				}
			}
		}
	}
}

func (g *Gossip) runGossip() (err error) {
	defer func() {
		if r := recover(); r != nil {
			warpErr := warpnet.WarpError(fmt.Sprintf("%v", r))
			err = fmt.Errorf("gossip: recovered from panic: %w", warpErr)
		}
	}()
	if g == nil || g.node == nil {
		return warpnet.WarpError("gossip: service not initialized properly")
	}

	g.pubsub, err = pubsub.NewGossipSub(g.ctx, g.node.Node())
	if err != nil {
		return err
	}
	g.isRunning.Store(true)

	go g.runPeerInfoPublishing(time.Minute * 5)
	log.Infoln("gossip: started")

	return
}

func (g *Gossip) Subscribe(handlers ...TopicHandler) (err error) {
	if g == nil || !g.isRunning.Load() {
		return ErrPubsubNotInit
	}

	for _, h := range handlers {
		if err := g.SubscribeRaw(h.TopicName, h.Handler); err != nil {
			return err
		}
	}
	return nil
}

func (g *Gossip) SubscribeRaw(topicName string, h func([]byte) error) (err error) {
	if g == nil || !g.isRunning.Load() {
		return ErrPubsubNotInit
	}
	if topicName == "" {
		return ErrPubsubEmptyTopic
	}

	if err := g.joinTopic(topicName); err != nil {
		return err
	}

	g.mx.Lock()
	defer g.mx.Unlock()
	if _, ok := g.handlersMap[topicName]; !ok {
		g.handlersMap[topicName] = nil
	}
	if h != nil {
		g.handlersMap[topicName] = append(g.handlersMap[topicName], h)
	}
	return nil
}

// joinTopic joins, relays and subscribes to a topic exactly once. Repeated
// calls for the same topic are a no-op, so several handlers can share a single
// subscription.
func (g *Gossip) joinTopic(topicName string) (err error) {
	g.mx.Lock()
	defer g.mx.Unlock()

	if _, ok := g.topics[topicName]; ok {
		return nil
	}

	topic, err := g.pubsub.Join(topicName)
	if err != nil {
		return err
	}
	g.topics[topicName] = topic

	relayCancel, err := topic.Relay()
	if err != nil {
		return err
	}

	sub, err := topic.Subscribe()
	if err != nil {
		return err
	}

	log.Infof("gossip: subscribed to topic: %s", topicName)

	g.relayCancelFuncs[topicName] = relayCancel
	g.subs = append(g.subs, sub)
	return nil
}

func (g *Gossip) Unsubscribe(topics ...string) (err error) {
	if g == nil || !g.isRunning.Load() {
		return ErrPubsubNotInit
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
		delete(g.handlersMap, topicName)
	}

	return err
}

func (g *Gossip) Subscribers(topicName string) []warpnet.WarpAddrInfo {
	g.mx.RLock()
	defer g.mx.RUnlock()

	topic, ok := g.topics[topicName]
	if !ok {
		return []warpnet.WarpAddrInfo{}
	}

	ids := topic.ListPeers()

	infos := make([]warpnet.WarpAddrInfo, 0, len(ids))
	for _, id := range ids {
		info := g.node.Node().Peerstore().PeerInfo(id)
		infos = append(infos, info)
	}
	return infos
}

func (g *Gossip) NotSubscribers(topicName string) []warpnet.WarpAddrInfo {
	g.mx.RLock()
	defer g.mx.RUnlock()

	topic, ok := g.topics[topicName]
	if !ok {
		return []warpnet.WarpAddrInfo{}
	}

	ids := topic.ListPeers()
	idsMap := make(map[warpnet.WarpPeerID]struct{}, len(ids))
	peers := g.node.Node().Peerstore().Peers()
	infos := make([]warpnet.WarpAddrInfo, 0, len(peers))

	for _, id := range peers {
		if _, ok := idsMap[id]; ok {
			continue
		}
		info := g.node.Node().Peerstore().PeerInfo(id)
		infos = append(infos, info)
	}
	return infos
}

func (g *Gossip) Publish(msg event.Message, topics ...string) (err error) {
	if g == nil || !g.isRunning.Load() {
		return ErrPubsubNotInit
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

		if msg.MessageId == "" {
			msg.MessageId = uuid.New().String()
		}
		if msg.NodeId == "" {
			msg.NodeId = g.node.NodeInfo().ID.String()
		}
		if msg.Version == "" {
			msg.Version = "0.0.0" // TODO
		}
		if msg.Timestamp.IsZero() {
			msg.Timestamp = time.Now()
		}
		msg.Timestamp = msg.Timestamp.UTC()
		msg.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(g.privKey, msg.SigningBytes()))

		data, err := json.Marshal(msg)
		if err != nil {
			log.Errorf("gossip: failed to marshal owner update message: %v", err)
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		err = topic.Publish(ctx, data)
		cancel()
		if err != nil && !errors.Is(err, pubsub.ErrTopicClosed) {
			log.Errorf("gossip: failed to publish owner update message: %v", err)
			return err
		}
	}
	return nil
}

func (g *Gossip) PublishRaw(topicName string, data []byte) (err error) {
	if g == nil || !g.isRunning.Load() {
		return ErrPubsubNotInit
	}

	g.mx.Lock()
	defer g.mx.Unlock()

	topic, ok := g.topics[topicName]
	if !ok {
		topic, err = g.pubsub.Join(topicName)
		if err != nil {
			return err
		}
		g.topics[topicName] = topic
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	err = topic.Publish(ctx, data)
	cancel()
	if err != nil && !errors.Is(err, pubsub.ErrTopicClosed) {
		log.Errorf("gossip: failed to publish owner update message: %v", err)
		return err
	}
	return nil
}

func (g *Gossip) SelfPublish(data []byte) error {
	var simulatedStreamMessage event.Message
	if err := json.Unmarshal(data, &simulatedStreamMessage); err != nil {
		log.Errorf("gossip: failed to decode user update message: %v %s", err, data)
		return err
	}

	if simulatedStreamMessage.Destination == "" {
		log.Warningln("gossip: user update message has no destination")
		return fmt.Errorf("gossip: %w: %s", ErrPubsubNoPathFound, string(data))
	}

	route := stream.WarpRoute(simulatedStreamMessage.Destination)

	if route.IsGet() { // only store data
		return nil
	}

	simulatedStreamMessage.Signature = base64.StdEncoding.EncodeToString(
		ed25519.Sign(g.privKey, simulatedStreamMessage.SigningBytes()),
	)
	data, err := json.Marshal(simulatedStreamMessage)
	if err != nil {
		log.Errorf("gossip: failed to re-sign user update message: %v", err)
		return err
	}

	_, err = g.node.SelfStream(route, data)
	return err
}

func (g *Gossip) NodeInfo() warpnet.NodeInfo {
	if g == nil || g.node == nil {
		return warpnet.NodeInfo{}
	}
	return g.node.NodeInfo()
}

func (g *Gossip) IsGossipRunning() bool {
	return g.isRunning.Load()
}

func (g *Gossip) runPeerInfoPublishing(duration time.Duration) {
	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	log.Infoln("pubsub: publisher started")
	defer log.Infoln("pubsub: publisher stopped")

	if err := g.publishPeerInfo(); err != nil { // initial publishing
		log.Errorf("pubsub: initial publish peer info: %v", err)
	}

	for {
		if !g.IsGossipRunning() {
			return
		}

		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			jitter := time.Second * time.Duration(rand.IntN(60)) //#nosec
			ticker.Reset(duration + jitter)

			err := g.publishPeerInfo()
			if errors.Is(err, pubsub.ErrTopicClosed) {
				return
			}
			if err != nil {
				log.Errorf("pubsub: failed to publish peer info: %v", err)
			}
		}
	}
}

const defaultPublishPeerInfoLimit = 10

func (g *Gossip) publishPeerInfo() error {
	myId := g.node.Node().ID()
	myAddrs := g.node.Node().Addrs()
	peerStore := g.node.Node().Peerstore()
	network := g.node.Node().Network()
	limit := defaultPublishPeerInfoLimit

	addrInfosMessage := []warpnet.WarpAddrInfo{{
		ID:    myId,
		Addrs: myAddrs,
	}}

	peerIds := peerStore.PeersWithAddrs()

	for _, id := range peerIds {
		if limit == 0 {
			break
		}
		if network.Connectedness(id) == warpnet.Disconnected {
			continue
		}
		addrs := peerStore.Addrs(id)
		addrInfosMessage = append(addrInfosMessage, warpnet.WarpAddrInfo{ID: id, Addrs: addrs})
		limit--
	}

	data, err := json.Marshal(addrInfosMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal peer info message: %w", err)
	}

	msg := event.Message{
		Body:        json.RawMessage(data),
		MessageId:   uuid.New().String(),
		NodeId:      g.NodeInfo().ID.String(),
		Destination: pubSubDiscoveryTopic,
		Timestamp:   time.Now(),
		Version:     "0.0.0", // TODO
	}

	return g.Publish(msg, pubSubDiscoveryTopic)
}

func (g *Gossip) Close() (err error) {
	defer func() {
		if r := recover(); r != nil {
			warpErr := warpnet.WarpError(fmt.Sprintf("%v", r))
			err = fmt.Errorf("%w", warpErr)
		}
	}()
	if !g.isRunning.Load() {
		return nil
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
	log.Infoln("gossip: closed")
	return err
}

type pubsubDiscoveryMessage struct {
	Body []warpnet.WarpAddrInfo `json:"body"`
}

func NewDiscoveryTopicHandler(discHandler discovery.DiscoveryHandler) TopicHandler {
	return TopicHandler{
		TopicName: pubSubDiscoveryTopic,
		Handler: func(data []byte) error {
			if len(data) == 0 {
				return nil
			}

			var msg pubsubDiscoveryMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				return fmt.Errorf("pubsub: discovery: unmarshal pubsub message: %w %s", err, data)
			}

			if len(msg.Body) == 0 {
				return fmt.Errorf("pubsub: discovery: %w: %s", ErrPubsubEmptyMessage, string(data))
			}

			for _, info := range msg.Body {
				discHandler(info)
			}
			return nil
		},
	}
}

// NewDiscoveryRelayTopicHandler acts only as relay
func NewDiscoveryRelayTopicHandler() TopicHandler {
	return TopicHandler{
		TopicName: pubSubDiscoveryTopic,
		Handler: func(_ []byte) error {
			return nil
		},
	}
}

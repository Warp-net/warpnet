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
	"math/rand"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/google/uuid"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	log "github.com/sirupsen/logrus"
)

const PubSubDiscoveryTopic = "peer-discovery"

type GossipNodeConnector interface {
	Node() warpnet.P2PNode
	NodeInfo() warpnet.NodeInfo
	SelfStream(path stream.WarpRoute, data any) (_ []byte, err error)
}

type topicHandler func(data []byte) error

func NewDiscoveryTopicHandler(handler func(data []byte) error) TopicHandler {
	return TopicHandler{
		TopicName: PubSubDiscoveryTopic,
		Handler:   handler,
	}
}

type Gossip struct {
	ctx    context.Context
	pubsub *pubsub.PubSub
	node   GossipNodeConnector

	mx               *sync.RWMutex
	subs             []*pubsub.Subscription
	relayCancelFuncs map[string]pubsub.RelayCancelFunc
	topics           map[string]*pubsub.Topic
	handlersMap      map[string]topicHandler
	isRunning        *atomic.Bool
	privKey          ed25519.PrivateKey
}

type TopicHandler struct {
	TopicName string
	Handler   topicHandler
}

func NewGossip(
	ctx context.Context,
	handlers ...TopicHandler,
) *Gossip {
	handlersMap := make(map[string]topicHandler)
	for _, h := range handlers {
		handlersMap[h.TopicName] = h.Handler
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
		return errors.New("gossip: already running")
	}

	g.node = node

	g.privKey, err = g.node.Node().Peerstore().PrivKey(g.node.Node().ID()).Raw()
	if err != nil {
		return err
	}

	if err := g.runGossip(); err != nil {
		return fmt.Errorf("gossip: failed to run: %v", err)
	}

	handlers := make([]TopicHandler, 0, len(g.handlersMap))
	for name, h := range g.handlersMap {
		handlers = append(handlers, TopicHandler{
			TopicName: name,
			Handler:   h,
		})
	}

	if err := g.Subscribe(handlers...); err != nil {
		return fmt.Errorf("gossip: presubscribe: %v", err)
	}

	go func() {
		if err := g.runListener(); err != nil {
			log.Errorf("gossip: listener: %v", err)
			return
		}
		log.Infoln("gossip: listener stopped")
	}()
	handlers = nil
	return nil
}

func (g *Gossip) runListener() error {
	if g == nil {
		return warpnet.WarpError("gossip: run listener: service not initialized properly")
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

			g.mx.RLock()
			handlerF, ok := g.handlersMap[strings.TrimSpace(*msg.Topic)]
			g.mx.RUnlock()
			if !ok || handlerF == nil {
				// default behavior
				if err := g.SelfPublish(msg.Data); err != nil {
					log.Errorf("gossip: self stream: %v", err)
				}
				continue
			}
			if err := handlerF(msg.Data); err != nil {
				log.Errorf(
					"gossip: failed to handle peer %s message from topic %s: %v",
					msg.ReceivedFrom.String(), *msg.Topic, err,
				)
				continue
			}
		}
	}
}

func (g *Gossip) runGossip() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("gossip: recovered from panic: %v", r)
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
		return warpnet.WarpError("gossip: service not initialized")
	}
	g.mx.Lock()
	defer g.mx.Unlock()

	for _, h := range handlers {
		if h.TopicName == "" {
			return warpnet.WarpError("gossip: topic name is empty")
		}

		topic, ok := g.topics[h.TopicName]
		if !ok {
			topic, err = g.pubsub.Join(h.TopicName)
			if err != nil {
				return err
			}
			g.topics[h.TopicName] = topic
		}

		relayCancel, err := topic.Relay()
		if err != nil {
			return err
		}

		sub, err := topic.Subscribe()
		if err != nil {
			return err
		}

		log.Infof("gossip: subscribed to topic: %s", h.TopicName)

		g.relayCancelFuncs[h.TopicName] = relayCancel
		g.subs = append(g.subs, sub)
		g.handlersMap[h.TopicName] = h.Handler
	}
	return nil
}

func (g *Gossip) Unsubscribe(topics ...string) (err error) {
	if g == nil || !g.isRunning.Load() {
		return warpnet.WarpError("gossip: service not initialized")
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
		return warpnet.WarpError("gossip: service not initialized")
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
		msg.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(g.privKey, msg.Body))

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

func (g *Gossip) SelfPublish(data []byte) error {
	var simulatedStreamMessage event.Message
	if err := json.Unmarshal(data, &simulatedStreamMessage); err != nil {
		log.Errorf("gossip: failed to decode user update message: %v %s", err, data)
		return err
	}

	if simulatedStreamMessage.Destination == "" {
		log.Warningln("gossip: user update message has no destination")
		return fmt.Errorf("gossip: user update message has no path: %s", string(data))
	}

	route := stream.WarpRoute(simulatedStreamMessage.Destination)

	if route.IsGet() { // only store data
		return nil
	}

	_, err := g.node.SelfStream(route, data)
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
			jitter := time.Second * time.Duration(rand.Intn(60))
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
		addrs := peerStore.Addrs(id)
		addrInfosMessage = append(addrInfosMessage, warpnet.WarpAddrInfo{ID: id, Addrs: addrs})
		limit--
	}

	data, err := json.Marshal(addrInfosMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal peer info message: %v", err)
	}

	msg := event.Message{
		Body:        json.RawMessage(data),
		MessageId:   uuid.New().String(),
		NodeId:      g.NodeInfo().ID.String(),
		Destination: "None",
		Timestamp:   time.Now(),
		Version:     "0.0.0", // TODO
	}

	return g.Publish(msg, PubSubDiscoveryTopic)
}

func (g *Gossip) Close() (err error) {
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
	log.Infoln("gossip: closed")
	return
}

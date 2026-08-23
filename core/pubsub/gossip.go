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
	"crypto/sha256"
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
	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/google/uuid"
	"github.com/hashicorp/golang-lru/v2/expirable"
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
	SelfStream(from, to warpnet.WarpPeerID, path stream.WarpRoute, data any) (_ []byte, err error)
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
	handlersMap      map[string]topicHandler
	isRunning        *atomic.Bool
	privKey          ed25519.PrivateKey

	// rater is read by the score function on every scoring pass, and
	// is swapped in after the router exists: the rating store is built
	// on top of this very gossip, so it cannot be ready before Run.
	rater atomic.Pointer[raterHolder]
}

type raterHolder struct{ scorer rating.Scorer }

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

	handlers := make([]TopicHandler, 0, len(g.handlersMap))
	for name, h := range g.handlersMap {
		handlers = append(handlers, TopicHandler{
			TopicName: name,
			Handler:   h,
		})
	}

	if err := g.Subscribe(handlers...); err != nil {
		return fmt.Errorf("gossip: presubscribe: %w", err)
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
			warpErr := warpnet.WarpError(fmt.Sprintf("%v", r))
			err = fmt.Errorf("gossip: recovered from panic: %w", warpErr)
		}
	}()
	if g == nil || g.node == nil {
		return warpnet.WarpError("gossip: service not initialized properly")
	}

	g.pubsub, err = pubsub.NewGossipSub(g.ctx, g.node.Node(), g.scoreOptions()...)
	if err != nil {
		return err
	}
	g.isRunning.Store(true)

	go g.runPeerInfoPublishing(time.Minute * 5)
	log.Infoln("gossip: started")

	return
}

// SetRating attaches the node's rating store so a peer's standing
// reaches gossipsub's own scoring. Safe to call after Run: the score
// function reads the current rater on every pass, which it has to,
// because the store is built on top of this gossip and cannot exist
// before it.
func (g *Gossip) SetRating(r rating.Scorer) {
	if g == nil || r == nil {
		return
	}
	g.rater.Store(&raterHolder{scorer: r})
}

func (g *Gossip) scorer() rating.Scorer {
	if g == nil {
		return rating.Nop{}
	}
	if held := g.rater.Load(); held != nil && held.scorer != nil {
		return held.scorer
	}
	return rating.Nop{}
}

// scoreOptions wires the rating into gossipsub's AppSpecificScore, the
// hook the router already consults when deciding whom to mesh with,
// whom to gossip to and whom to stop reading from entirely.
//
// Only the app-specific term is set. gossipsub's own per-topic
// delivery statistics stay at their defaults: they measure a different
// thing (is this peer useful in this mesh) and mixing our weights into
// them would make both harder to reason about.
func (g *Gossip) scoreOptions() []pubsub.Option {
	if g == nil {
		return nil
	}
	params := &pubsub.PeerScoreParams{
		AppSpecificScore: func(p warpnet.WarpPeerID) float64 {
			// EffectiveBand is BandTrusted in shadow mode and for the
			// Nop rater, both of which map to 0 — exactly what an
			// unrated peer gets, so scoring is inert until a store is
			// attached and enforcing.
			return rating.GossipAppScore(g.scorer().EffectiveBand(p))
		},
		AppSpecificWeight: 1,
		DecayInterval:     time.Minute,
		DecayToZero:       0.01,
		Topics:            map[string]*pubsub.TopicScoreParams{},
	}
	thresholds := &pubsub.PeerScoreThresholds{
		GossipThreshold:   -10,
		PublishThreshold:  -50,
		GraylistThreshold: rating.GossipGraylistThreshold,
		AcceptPXThreshold: 0,
	}
	return []pubsub.Option{pubsub.WithPeerScore(params, thresholds)}
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
	g.mx.Lock()
	defer g.mx.Unlock()

	if topicName == "" {
		return ErrPubsubEmptyTopic
	}
	if g.pubsub == nil {
		return ErrPubsubNotInit
	}

	topic, ok := g.topics[topicName]
	if !ok {
		topic, err = g.pubsub.Join(topicName)
		if err != nil {
			return err
		}
		g.topics[topicName] = topic
	}

	if _, subscribed := g.relayCancelFuncs[topicName]; subscribed {
		g.handlersMap[topicName] = h
		return nil
	}

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
	g.handlersMap[topicName] = h

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

		if _, ok := g.relayCancelFuncs[topicName]; ok {
			g.relayCancelFuncs[topicName]()
		}
		delete(g.relayCancelFuncs, topicName)

		if err = topic.Close(); err != nil {
			return err
		}
		delete(g.topics, topicName)
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

	if g.pubsub == nil {
		return ErrPubsubNotInit
	}

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

	if g.pubsub == nil {
		return ErrPubsubNotInit
	}

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

	data, err := json.Marshal(simulatedStreamMessage)
	if err != nil {
		log.Errorf("gossip: failed to re-sign user update message: %v", err)
		return err
	}

	_, err = g.node.SelfStream(
		warpnet.FromStringToPeerID(simulatedStreamMessage.NodeId), g.node.NodeInfo().ID, route, data,
	)
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

type pubsubDiscoveryEnvelope struct {
	Body   json.RawMessage `json:"body"`
	NodeId string          `json:"node_id"`
}

const discoveryEchoCacheSize = 512

// discoveryEchoTTL is how long an announcement is remembered as
// already handled. Slightly longer than the publish interval plus its
// jitter, so a republication of an unchanged peer list is recognised
// while a genuinely new list still gets through.
const discoveryEchoTTL = 7 * time.Minute

func NewDiscoveryTopicHandler(discHandler discovery.DiscoveryHandler) TopicHandler {
	// Every topic is relayed, so one announcement reaches a node by
	// several paths, and the publisher repeats it every few minutes
	// whether or not anything changed. Feeding each copy into
	// discovery meant re-learning the same peers over and over. Key
	// the dedup on the announcement's content, not on a sequence
	// number: an unsigned counter on the wire would let anyone
	// suppress a peer's announcements by publishing a huge one under
	// its id.
	seen := expirable.NewLRU[string, [sha256.Size]byte](
		discoveryEchoCacheSize, nil, discoveryEchoTTL,
	)

	return TopicHandler{
		TopicName: pubSubDiscoveryTopic,
		Handler: func(data []byte) error {
			if len(data) == 0 {
				return nil
			}

			var envelope pubsubDiscoveryEnvelope
			if err := json.Unmarshal(data, &envelope); err != nil {
				return fmt.Errorf("pubsub: discovery: unmarshal pubsub message: %w %s", err, data)
			}

			if len(envelope.Body) == 0 {
				return fmt.Errorf("pubsub: discovery: %w: %s", ErrPubsubEmptyMessage, string(data))
			}

			var infos []warpnet.WarpAddrInfo
			if err := json.Unmarshal(envelope.Body, &infos); err != nil {
				return fmt.Errorf("pubsub: discovery: unmarshal peer infos: %w %s", err, data)
			}
			if len(infos) == 0 {
				return fmt.Errorf("pubsub: discovery: %w: %s", ErrPubsubEmptyMessage, string(data))
			}

			if envelope.NodeId != "" {
				digest := sha256.Sum256(envelope.Body)
				if previous, ok := seen.Get(envelope.NodeId); ok && previous == digest {
					log.Debugf("pubsub: discovery: unchanged announcement from %s, skipped", envelope.NodeId)
					return nil
				}
				seen.Add(envelope.NodeId, digest)
			}

			for _, info := range infos {
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

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

	"github.com/Warp-net/warpnet/cmd/node/moderator/vote"
	"github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
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

type moderatorPubSub struct {
	pubsub *pubsub.Gossip
}

func NewPubSub(ctx context.Context) *moderatorPubSub {
	mps := &moderatorPubSub{}

	mps.pubsub = pubsub.NewGossip(ctx, pubsub.NewDiscoveryRelayTopicHandler())
	return mps
}

func (g *moderatorPubSub) Run(node PubsubServerNodeConnector) error {
	if g.pubsub.IsGossipRunning() {
		return nil
	}

	return g.pubsub.Run(node)
}

func (g *moderatorPubSub) PublishUpdateToFollowers(ownerId, dest string, body any) (err error) {
	if g == nil || !g.pubsub.IsGossipRunning() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	topicName := fmt.Sprintf("%s-%s", userUpdateTopicPrefix, ownerId)

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}
	msg := event.Message{
		Body:        bodyBytes,
		NodeId:      g.pubsub.NodeInfo().ID.String(),
		Destination: dest,
		Timestamp:   time.Now(),
		MessageId:   uuid.New().String(),
		Version:     "0.0.0", // TODO manage protocol versions properly
	}

	return g.pubsub.Publish(msg, topicName)
}

func (g *moderatorPubSub) SubscribeReports(h func(ev event.ReportEvent) error) error {
	if g == nil || !g.pubsub.IsGossipRunning() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	return g.pubsub.SubscribeRaw(event.ReportsTopic, func(data []byte) error {
		msg, err := verifiedEnvelope("reports", data)
		if err != nil || msg == nil {
			return err
		}

		var ev event.ReportEvent
		if err := json.Unmarshal(msg.Body, &ev); err != nil {
			return fmt.Errorf("pubsub: reports: payload unmarshal: %w", err)
		}
		return h(ev)
	})
}

// PublishVote publishes this moderator's verdict on one report round. The
// envelope is signed by Gossip.Publish with the node key, which is what
// SubscribeVotes authenticates the voter by.
func (g *moderatorPubSub) PublishVote(ev vote.Event) error {
	if g == nil || !g.pubsub.IsGossipRunning() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	bodyBytes, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	msg := event.Message{
		Body:      bodyBytes,
		NodeId:    g.pubsub.NodeInfo().ID.String(),
		Timestamp: time.Now(),
		MessageId: uuid.New().String(),
		Version:   "0.0.0",
	}
	return g.pubsub.Publish(msg, vote.Topic)
}

func (g *moderatorPubSub) SubscribeVotes(h func(ev vote.Event) error) error {
	if g == nil || !g.pubsub.IsGossipRunning() {
		return warpnet.WarpError("pubsub: service not initialized")
	}
	return g.pubsub.SubscribeRaw(vote.Topic, func(data []byte) error {
		msg, err := verifiedEnvelope("votes", data)
		if err != nil || msg == nil {
			return err
		}

		var ev vote.Event
		if err := json.Unmarshal(msg.Body, &ev); err != nil {
			return fmt.Errorf("pubsub: votes: payload unmarshal: %w", err)
		}
		// The voter identity is the signature-verified envelope sender;
		// whatever the payload claimed is discarded.
		ev.ModeratorID = domain.ID(msg.NodeId)
		return h(ev)
	})
}

// verifiedEnvelope unmarshals a gossip envelope and checks its signature
// against the pubkey recovered from the sender's peer id. A nil, nil return
// means "drop silently" (malformed or forged message).
func verifiedEnvelope(topic string, data []byte) (*event.Message, error) {
	var msg event.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("pubsub: %s: envelope unmarshal: %w", topic, err)
	}

	peerID := warpnet.FromStringToPeerID(msg.NodeId)
	if peerID == "" {
		log.Warnf("pubsub: %s: dropping message with malformed NodeId=%q", topic, msg.NodeId)
		return nil, nil
	}
	pubKey := warpnet.FromIDToPubKey(peerID)
	if len(pubKey) == 0 {
		log.Warnf("pubsub: %s: dropping message: cannot derive pubkey from %s", topic, msg.NodeId)
		return nil, nil
	}
	if err := security.VerifySignature(pubKey, msg.SigningBytes(), msg.Signature); err != nil {
		log.Warnf("pubsub: %s: dropping message from %s: signature invalid: %v", topic, msg.NodeId, err)
		return nil, nil
	}
	return &msg, nil
}

func (g *moderatorPubSub) Close() (err error) {
	return g.pubsub.Close()
}

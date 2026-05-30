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

package node

import (
	"context"
	"fmt"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/moderator/moderator"
	"github.com/Warp-net/warpnet/config"
	corePubsub "github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// userUpdateTopicPrefix matches the member and moderator pubsub packages: a
// per-user followers topic is "user-update-<userId>". Duplicated here (as it
// already is between member and moderator pubsub) so the isolation verdict
// lands on the same topic observers listen on.
const userUpdateTopicPrefix = "user-update"

// moderationPubSub adapts the node's single gossip instance to the two slices
// of pubsub the moderator needs: publishing isolation verdicts on a user's
// followers topic, and subscribing to the global reports topic. It mirrors
// cmd/node/moderator/pubsub but reuses the gossip the business node already
// runs instead of standing up a second gossipsub router (a host can host only
// one).
type moderationPubSub struct {
	gossip *corePubsub.Gossip
}

func newModerationPubSub(g *corePubsub.Gossip) *moderationPubSub {
	return &moderationPubSub{gossip: g}
}

// PublishUpdateToFollowers marshals body to a real JSON object and publishes
// it on ownerId's followers topic. The isolation protocol passes a struct, so
// the signature takes `any` (unlike MemberPubSub's []byte variant).
func (g *moderationPubSub) PublishUpdateToFollowers(ownerId, dest string, body any) (err error) {
	if g == nil || g.gossip == nil || !g.gossip.IsGossipRunning() {
		return warpnet.WarpError("business: moderation pubsub not initialized")
	}
	topicName := fmt.Sprintf("%s-%s", userUpdateTopicPrefix, ownerId)

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}
	msg := event.Message{
		Body:        bodyBytes,
		NodeId:      g.gossip.NodeInfo().ID.String(),
		Destination: dest,
		Timestamp:   time.Now(),
		MessageId:   uuid.New().String(),
		Version:     "0.0.0",
	}
	return g.gossip.Publish(msg, topicName)
}

// SubscribeReports listens on the open reports topic. The envelope is verified
// against a pubkey derived from msg.NodeId (the topic is open — anyone may
// publish — so this path can't lean on AuthMiddleware); anything that fails to
// verify is dropped silently. Mirrors moderator pubsub's SubscribeReports.
func (g *moderationPubSub) SubscribeReports(h func(ev event.ReportEvent) error) error {
	if g == nil || g.gossip == nil || !g.gossip.IsGossipRunning() {
		return warpnet.WarpError("business: moderation pubsub not initialized")
	}
	return g.gossip.SubscribeRaw(event.ReportsTopic, func(data []byte) error {
		var msg event.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf("business: reports: envelope unmarshal: %w", err)
		}

		peerID := warpnet.FromStringToPeerID(msg.NodeId)
		if peerID == "" {
			log.Warnf("business: reports: dropping message with malformed NodeId=%q", msg.NodeId)
			return nil
		}
		pubKey := warpnet.FromIDToPubKey(peerID)
		if len(pubKey) == 0 {
			log.Warnf("business: reports: dropping message: cannot derive pubkey from %s", msg.NodeId)
			return nil
		}
		if err := security.VerifySignature(pubKey, msg.Body, msg.Signature); err != nil {
			log.Warnf("business: reports: dropping message from %s: signature invalid: %v", msg.NodeId, err)
			return nil
		}

		var ev event.ReportEvent
		if err := json.Unmarshal(msg.Body, &ev); err != nil {
			return fmt.Errorf("business: reports: payload unmarshal: %w", err)
		}
		return h(ev)
	})
}

// StartModerator wires the shared moderator engine and report subscription
// onto the business node, exactly as the standalone moderator binary does
// (same engineReadyChan startup, same engine.Moderate interface, same
// isolation protocol). It is a no-op when no model path is configured so the
// business node still serves content and relays without a model present.
//
// moderator.Start blocks until the engine reports ready (or forever when the
// binary is built without the `llama` tag), so it runs on its own goroutine
// and never blocks node startup.
func (m *BusinessNode) StartModerator(ctx context.Context) error {
	if config.Config().Node.Moderator.Path == "" {
		log.Warnln("business: moderator model path is empty; moderation disabled")
		return nil
	}
	g := m.Gossip()
	if g == nil {
		return warpnet.WarpError("business: moderator: gossip not running")
	}

	pub := newModerationPubSub(g)
	moder, err := moderator.NewModerator(ctx, m, pub, pub)
	if err != nil {
		return fmt.Errorf("business: init moderator: %w", err)
	}
	m.moder = moder

	go func() {
		log.Infoln("business: starting moderator engine...")
		if err := moder.Start(); err != nil {
			log.Errorf("business: moderator start: %v", err)
		}
	}()
	return nil
}

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

	member "github.com/Warp-net/warpnet/cmd/node/member/node"
	"github.com/Warp-net/warpnet/cmd/node/moderator/moderator"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
)

// moderationPubSub adapts the node's existing member pubsub to the two slices
// the moderator needs: publishing isolation verdicts on a user's followers
// topic, and subscribing to the global reports topic. It reuses the gossip the
// business node already runs (a host can host only one gossipsub router) — the
// publish path delegates to MemberPubSub, only the verdict's `any` body is
// marshalled here to fit the isolation Publisher interface.
type moderationPubSub struct {
	ps member.PubSubProvider
}

func newModerationPubSub(ps member.PubSubProvider) *moderationPubSub {
	return &moderationPubSub{ps: ps}
}

// PublishUpdateToFollowers marshals the verdict (isolation passes a struct, not
// bytes) and hands it to the member pubsub, which owns the topic naming.
func (g *moderationPubSub) PublishUpdateToFollowers(ownerId, dest string, body any) error {
	bt, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return g.ps.PublishUpdateToFollowers(ownerId, dest, bt)
}

// SubscribeReports listens on the open reports topic. The topic is open — anyone
// may publish — so this path can't lean on AuthMiddleware: the envelope is
// verified against a pubkey derived from msg.NodeId, and anything that fails is
// dropped silently. Mirrors moderator pubsub's SubscribeReports.
func (g *moderationPubSub) SubscribeReports(h func(ev event.ReportEvent) error) error {
	gossip := g.ps.Gossip()
	if gossip == nil || !gossip.IsGossipRunning() {
		return warpnet.WarpError("business: moderation pubsub not initialized")
	}
	return gossip.SubscribeRaw(event.ReportsTopic, func(data []byte) error {
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

// StartModerator wires the shared moderator engine and report subscription onto
// the business node, exactly as the standalone moderator binary does (same
// engineReadyChan startup, same engine.Moderate interface, same isolation
// protocol). It is a no-op when no model path is configured so the node still
// serves content and relays without a model present.
//
// moderator.Start blocks until the engine reports ready (or forever when the
// binary is built without the `llama` tag), so it runs on its own goroutine and
// never blocks node startup.
func (b *BusinessNode) StartModerator(ctx context.Context) error {
	if config.Config().Node.Moderator.Path == "" {
		log.Warnln("business: moderator model path is empty; moderation disabled")
		return nil
	}
	ps := b.PubSub()
	if ps == nil || ps.Gossip() == nil {
		return warpnet.WarpError("business: moderator: gossip not running")
	}

	pub := newModerationPubSub(ps)
	moder, err := moderator.NewModerator(ctx, b, pub, pub)
	if err != nil {
		return fmt.Errorf("business: init moderator: %w", err)
	}
	b.moder = moder

	go func() {
		log.Infoln("business: starting moderator engine...")
		if err := moder.Start(); err != nil {
			log.Errorf("business: moderator start: %v", err)
		}
	}()
	return nil
}

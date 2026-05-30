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

	"github.com/Warp-net/warpnet/cmd/node/moderator/moderator"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/warpnet"
	log "github.com/sirupsen/logrus"
)

// StartModerator wires the shared moderator engine and report subscription onto
// the business node, exactly as the standalone moderator binary does (same
// engineReadyChan startup, same engine.Moderate interface, same isolation
// protocol). The node's own gossip is both the verdict Publisher and the report
// ReportSubscriber — core/pubsub.Gossip implements both — so no adapter is
// needed and no second gossipsub router is stood up.
//
// It is a no-op when no model path is configured so the node still serves
// content and relays without a model. moderator.Start blocks until the engine
// is ready (or forever without the `llama` build tag), so it runs on its own
// goroutine and never blocks node startup.
func (b *BusinessNode) StartModerator(ctx context.Context) error {
	if config.Config().Node.Moderator.Path == "" {
		log.Warnln("business: moderator model path is empty; moderation disabled")
		return nil
	}
	ps := b.PubSub()
	if ps == nil || ps.Gossip() == nil {
		return warpnet.WarpError("business: moderator: gossip not running")
	}
	g := ps.Gossip()

	moder, err := moderator.NewModerator(ctx, b, g, g)
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

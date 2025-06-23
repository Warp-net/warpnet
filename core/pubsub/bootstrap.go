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
	"fmt"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/json"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	log "github.com/sirupsen/logrus"
)

type bootstrapPubSub struct {
	ctx              context.Context
	pubsub           *gossip
	discoveryHandler discovery.DiscoveryHandler
}

func NewPubSubBootstrap(ctx context.Context, discoveryHandler discovery.DiscoveryHandler) *bootstrapPubSub {
	bps := &bootstrapPubSub{
		ctx:              ctx,
		discoveryHandler: discoveryHandler,
	}
	h := TopicHandler{
		TopicName: pubSubDiscoveryTopic,
		Handler:   bps.handlePubSubDiscovery,
	}
	bps.pubsub = newGossip(ctx, h)
	return bps
}

func (g *bootstrapPubSub) handlePubSubDiscovery(msg *pubsub.Message) error {
	if msg == nil {
		return nil
	}
	log.Infof("bootstrap pubsub: received discovery message: %s", msg.ID)

	var discoveryAddrInfos []warpnet.WarpPubInfo

	outerErr := json.JSON.Unmarshal(msg.Data, &discoveryAddrInfos)
	if outerErr != nil {
		var single warpnet.WarpPubInfo
		if innerErr := json.JSON.Unmarshal(msg.Data, &single); innerErr != nil {
			return fmt.Errorf("pubsub: discovery: failed to decode discovery message: %v %s", innerErr, msg.Data)
		}
		discoveryAddrInfos = []warpnet.WarpPubInfo{single}
	}
	if len(discoveryAddrInfos) == 0 {
		return nil
	}

	for _, info := range discoveryAddrInfos {
		if info.ID == "" {
			log.Errorf("pubsub: discovery: message has no ID: %s", string(msg.Data))
			continue
		}
		if info.ID == g.pubsub.nodeInfo().ID {
			continue
		}

		peerInfo := warpnet.WarpAddrInfo{
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
	return nil
}

func (g *bootstrapPubSub) Run(node PubsubServerNodeConnector) {
	if g.pubsub.isGossipRunning() {
		return
	}

	if err := g.pubsub.run(node); err != nil {
		log.Errorf("pubsub: failed to run: %v", err)
		return
	}
}

func (g *bootstrapPubSub) Close() (err error) {
	return g.pubsub.close()
}

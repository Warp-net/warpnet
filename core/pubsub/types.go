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
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"strings"
	"sync"
	"sync/atomic"
)

// TODO clean up the mess
type topicPrefix string

func (t topicPrefix) isIn(s string) bool {
	return strings.HasPrefix(s, string(t))
}

const (
	// full names
	pubSubModerationTopic = "moderation-text"

	pubSubDiscoveryTopic = "peer-discovery"
	pubSubConsensusTopic = "peer-consensus"
	// prefixes
	userUpdateTopicPrefix topicPrefix = "user-update"

	publishPeerInfoLimit = 10
)

type PubsubServerNodeConnector interface {
	Node() warpnet.P2PNode
	NodeInfo() warpnet.NodeInfo
}

type PubsubClientNodeStreamer interface {
	ClientStream(nodeId string, path string, data any) (_ []byte, err error)
	IsRunning() bool
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

type PubsubModeratorConnector interface {
	Node() warpnet.P2PNode
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
}

type warpModeratorPubSub struct {
	ctx        context.Context
	pubsub     *pubsub.PubSub
	serverNode PubsubModeratorConnector

	mx    *sync.RWMutex
	sub   *pubsub.Subscription
	topic *pubsub.Topic

	isRunning *atomic.Bool
}

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
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
)

func TestNewGossip(t *testing.T) {
	g := NewGossip(context.Background(), TopicHandler{
		TopicName: "topic-a",
		Handler:   func([]byte) error { return nil },
	})
	assert.NotNil(t, g)
	assert.False(t, g.IsGossipRunning())
	assert.Contains(t, g.handlersMap, "topic-a")
}

func TestGossip_NodeInfo_NilSafe(t *testing.T) {
	assert.Equal(t, warpnet.NodeInfo{}, (&Gossip{}).NodeInfo())
	assert.Equal(t, warpnet.NodeInfo{}, (*Gossip)(nil).NodeInfo())
}

// TestGossip_NotInitializedGuards verifies the public API refuses to operate
// until Run has flipped isRunning, rather than dereferencing a nil pubsub.
func TestGossip_NotInitializedGuards(t *testing.T) {
	g := NewGossip(context.Background())
	h := TopicHandler{TopicName: "t", Handler: func([]byte) error { return nil }}

	assert.ErrorIs(t, g.Subscribe(h), ErrPubsubNotInit)
	assert.ErrorIs(t, g.SubscribeRaw("t", func([]byte) error { return nil }), ErrPubsubNotInit)
	assert.ErrorIs(t, g.Unsubscribe("t"), ErrPubsubNotInit)
	assert.ErrorIs(t, g.Publish(event.Message{}, "t"), ErrPubsubNotInit)
	assert.ErrorIs(t, g.PublishRaw("t", []byte("{}")), ErrPubsubNotInit)
}

func TestGossip_Subscribers_UnknownTopic(t *testing.T) {
	g := NewGossip(context.Background())
	assert.Empty(t, g.Subscribers("missing"))
	assert.Empty(t, g.NotSubscribers("missing"))
}

// TestSelfPublish_Validation covers the message-validation branches that run
// before SelfPublish touches the node, so a zero-value Gossip is enough.
func TestSelfPublish_Validation(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		err := (&Gossip{}).SelfPublish([]byte("{"))
		assert.Error(t, err)
	})

	t.Run("empty destination", func(t *testing.T) {
		data, _ := json.Marshal(event.Message{Destination: ""})
		err := (&Gossip{}).SelfPublish(data)
		assert.ErrorIs(t, err, ErrPubsubNoPathFound)
	})

	t.Run("get route is a no-op", func(t *testing.T) {
		// IsGet() short-circuits before the node is used: a GET destination
		// is store-only and must not be re-streamed.
		data, _ := json.Marshal(event.Message{Destination: event.PUBLIC_GET_USER})
		err := (&Gossip{}).SelfPublish(data)
		assert.NoError(t, err)
	})
}

func TestNewDiscoveryTopicHandler(t *testing.T) {
	peerID, err := peer.Decode("12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j")
	assert.NoError(t, err)

	t.Run("empty data is ignored", func(t *testing.T) {
		called := false
		th := NewDiscoveryTopicHandler(func(warpnet.WarpAddrInfo) { called = true })
		assert.NoError(t, th.Handler(nil))
		assert.False(t, called)
	})

	t.Run("invalid json", func(t *testing.T) {
		th := NewDiscoveryTopicHandler(func(warpnet.WarpAddrInfo) {})
		assert.Error(t, th.Handler([]byte("{")))
	})

	t.Run("empty body", func(t *testing.T) {
		th := NewDiscoveryTopicHandler(func(warpnet.WarpAddrInfo) {})
		data, _ := json.Marshal(pubsubDiscoveryMessage{Body: nil})
		assert.ErrorIs(t, th.Handler(data), ErrPubsubEmptyMessage)
	})

	t.Run("valid body fans out to discovery handler", func(t *testing.T) {
		var got []peer.ID
		th := NewDiscoveryTopicHandler(func(info warpnet.WarpAddrInfo) {
			got = append(got, info.ID)
		})
		data, err := json.Marshal(pubsubDiscoveryMessage{
			Body: []warpnet.WarpAddrInfo{{ID: peerID}},
		})
		assert.NoError(t, err)
		assert.NoError(t, th.Handler(data))
		assert.Equal(t, []peer.ID{peerID}, got)
	})
}

func TestNewDiscoveryRelayTopicHandler(t *testing.T) {
	th := NewDiscoveryRelayTopicHandler()
	assert.Equal(t, pubSubDiscoveryTopic, th.TopicName)
	assert.NoError(t, th.Handler([]byte("anything")))
	assert.NoError(t, th.Handler(nil))
}

func TestGossipErrors(t *testing.T) {
	// Sanity-check the sentinel error strings so errors.Is targets stay stable.
	assert.True(t, errors.Is(ErrPubsubNotInit, ErrPubsubNotInit))
	assert.Equal(t, "gossip: topic name is empty", ErrPubsubEmptyTopic.Error())
}

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

//nolint:all
package pubsub

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/libp2p/go-libp2p"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type liveConnector struct {
	host    warpnet.P2PNode
	ownerId string
}

func (c *liveConnector) Node() warpnet.P2PNode { return c.host }

func (c *liveConnector) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{ID: c.host.ID(), OwnerId: c.ownerId}
}

func (c *liveConnector) RelayStream(_ warpnet.WarpPeerID, path stream.WarpRoute, data any) ([]byte, error) {
	return c.SelfStream(path, data)
}

func (c *liveConnector) SelfStream(path stream.WarpRoute, data any) ([]byte, error) {
	return nil, nil
}

func (c *liveConnector) GenericStream(nodeIdStr string, path stream.WarpRoute, data any) ([]byte, error) {
	return nil, nil
}

func timeoutAfter() <-chan time.Time {
	return time.After(15 * time.Second)
}

func newConnector(t *testing.T, ownerId string) *liveConnector {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return &liveConnector{host: h, ownerId: ownerId}
}

func newRunningPubSub(t *testing.T, ownerId string) (*MemberPubSub, *liveConnector) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	conn := newConnector(t, ownerId)
	ps := NewPubSub(ctx)
	ps.Run(conn)
	t.Cleanup(func() { _ = ps.Close() })

	require.True(t, ps.Gossip().IsGossipRunning())
	return ps, conn
}

func TestMemberPubSub_NilAndUnstartedAreInert(t *testing.T) {
	var nilPS *MemberPubSub
	assert.Empty(t, nilPS.OwnerID())
	assert.Empty(t, nilPS.NodeID())
	assert.Nil(t, nilPS.Gossip())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ps := NewPubSub(ctx)
	require.NotNil(t, ps.Gossip())
	assert.False(t, ps.Gossip().IsGossipRunning())

	assert.Error(t, ps.SubscribeUserUpdate("someone"))
	assert.Error(t, ps.UnsubscribeUserUpdate("someone"))
	assert.Error(t, ps.PublishReport(event.ReportEvent{}))
	assert.Error(t, ps.PublishUpdateToFollowers("owner", "/dest", []byte(`{}`)))
}

func TestMemberPubSub_RunIsIdempotent(t *testing.T) {
	ps, conn := newRunningPubSub(t, "owner-1")

	assert.NotPanics(t, func() { ps.Run(conn) })
	assert.True(t, ps.Gossip().IsGossipRunning())

	assert.Equal(t, "owner-1", ps.OwnerID())
	assert.Equal(t, conn.host.ID().String(), ps.NodeID())
}

func TestMemberPubSub_FollowAndUnfollowTopicLifecycle(t *testing.T) {
	ps, _ := newRunningPubSub(t, "owner-1")

	require.NoError(t, ps.SubscribeUserUpdate("author-42"))

	assert.NoError(t, ps.SubscribeUserUpdate("author-42"))

	assert.NoError(t, ps.UnsubscribeUserUpdate("author-42"))

	assert.NoError(t, ps.UnsubscribeUserUpdate("never-followed"))
}

func TestMemberPubSub_CannotFollowSelf(t *testing.T) {
	ps, conn := newRunningPubSub(t, "owner-1")

	err := ps.SubscribeUserUpdate(conn.host.ID().String())
	assert.Error(t, err, "a node must not subscribe to its own update topic")
}

func TestPrefollowHandlers_BuildsOneTopicPerUser(t *testing.T) {
	handlers := PrefollowHandlers("a", "b", "c")
	require.Len(t, handlers, 3)

	seen := make(map[string]struct{}, 3)
	for _, h := range handlers {
		assert.True(t, strings.HasSuffix(h.TopicName, "-a") ||
			strings.HasSuffix(h.TopicName, "-b") ||
			strings.HasSuffix(h.TopicName, "-c"))
		seen[h.TopicName] = struct{}{}
	}
	assert.Len(t, seen, 3, "each followed user gets a distinct topic")

	assert.Empty(t, PrefollowHandlers(), "no follows means no topics")
}

func TestMemberPubSub_PublishReportStampsReporterIdentity(t *testing.T) {
	ps, conn := newRunningPubSub(t, "owner-1")

	received := make(chan []byte, 4)
	require.NoError(t, ps.Gossip().SubscribeRaw(event.ReportsTopic, func(data []byte) error {
		select {
		case received <- data:
		default:
		}
		return nil
	}))

	err := ps.PublishReport(event.ReportEvent{
		ReporterID:     "i-am-somebody-else",
		ReporterNodeID: "spoofed-node",
		Reason:         "spam",
	})
	require.NoError(t, err)

	select {
	case raw := <-received:
		assert.Contains(t, string(raw), "owner-1", "the node's own owner id must be stamped in")
		assert.Contains(t, string(raw), conn.host.ID().String())
		assert.NotContains(t, string(raw), "i-am-somebody-else",
			"a spoofed reporter id must be overwritten")
		assert.NotContains(t, string(raw), "spoofed-node")
	case <-timeoutAfter():
		t.Fatal("report was never published")
	}
}

func TestMemberPubSub_PublishUpdateToFollowersAddressesTheOwnerTopic(t *testing.T) {
	ps, conn := newRunningPubSub(t, "owner-1")

	topic := "user-update-owner-1"
	received := make(chan []byte, 4)
	require.NoError(t, ps.Gossip().SubscribeRaw(topic, func(data []byte) error {
		select {
		case received <- data:
		default:
		}
		return nil
	}))

	require.NoError(t, ps.PublishUpdateToFollowers("owner-1", "/public/post/tweet", []byte(`{"text":"hi"}`)))

	select {
	case raw := <-received:
		assert.Contains(t, string(raw), `"text":"hi"`)
		assert.Contains(t, string(raw), "/public/post/tweet", "followers need the destination route")
		assert.Contains(t, string(raw), conn.host.ID().String())
	case <-timeoutAfter():
		t.Fatal("follower update was never published")
	}
}

func TestMemberPubSub_CloseIsIdempotentAndDisablesPublishing(t *testing.T) {
	ps, _ := newRunningPubSub(t, "owner-1")

	require.NoError(t, ps.Close())
	assert.NoError(t, ps.Close())

	assert.Error(t, ps.PublishReport(event.ReportEvent{}))
	assert.Error(t, ps.PublishUpdateToFollowers("owner-1", "/dest", []byte(`{}`)))
	assert.Error(t, ps.SubscribeUserUpdate("author"))
}

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package pubsub

import (
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/json"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPeerID(t *testing.T) peer.ID {
	t.Helper()
	id, err := peer.Decode("12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j")
	require.NoError(t, err)
	return id
}

func testOtherPeerID(t *testing.T) peer.ID {
	t.Helper()
	id, err := peer.Decode("12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo")
	require.NoError(t, err)
	return id
}

func announcement(t *testing.T, nodeID string, ids ...peer.ID) []byte {
	t.Helper()
	infos := make([]warpnet.WarpAddrInfo, 0, len(ids))
	for _, id := range ids {
		infos = append(infos, warpnet.WarpAddrInfo{ID: id})
	}
	body, err := json.Marshal(infos)
	require.NoError(t, err)
	data, err := json.Marshal(pubsubDiscoveryEnvelope{Body: body, NodeId: nodeID})
	require.NoError(t, err)
	return data
}

func TestUnchangedAnnouncementIsHandledOnce(t *testing.T) {
	peerID := testPeerID(t)

	var fanout []peer.ID
	th := NewDiscoveryTopicHandler(func(info warpnet.WarpAddrInfo) {
		fanout = append(fanout, info.ID)
	})

	data := announcement(t, "publisher-1", peerID)
	for range 5 {
		require.NoError(t, th.Handler(data))
	}

	assert.Equal(t, []peer.ID{peerID}, fanout,
		"a republished, unchanged announcement must not re-enter discovery")
}

func TestChangedAnnouncementGetsThrough(t *testing.T) {
	peerID := testPeerID(t)
	other := testOtherPeerID(t)

	var fanout []peer.ID
	th := NewDiscoveryTopicHandler(func(info warpnet.WarpAddrInfo) {
		fanout = append(fanout, info.ID)
	})

	require.NoError(t, th.Handler(announcement(t, "publisher-1", peerID)))
	require.NoError(t, th.Handler(announcement(t, "publisher-1", peerID, other)))

	assert.Equal(t, []peer.ID{peerID, peerID, other}, fanout,
		"a genuinely new peer list must still be acted on")
}

func TestEchoDedupIsPerPublisher(t *testing.T) {
	peerID := testPeerID(t)

	var fanout []peer.ID
	th := NewDiscoveryTopicHandler(func(info warpnet.WarpAddrInfo) {
		fanout = append(fanout, info.ID)
	})

	require.NoError(t, th.Handler(announcement(t, "publisher-1", peerID)))
	require.NoError(t, th.Handler(announcement(t, "publisher-2", peerID)))

	assert.Len(t, fanout, 2)
}

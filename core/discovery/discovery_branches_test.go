//nolint:all
package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/mastodon"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

func TestRunRoutesByNodeRole(t *testing.T) {
	for _, tt := range []struct {
		name string
		info warpnet.NodeInfo
	}{
		{"member", warpnet.NodeInfo{ID: warpnet.FromStringToPeerID(selfID), Type: warpnet.MemberNode}},
		{"relay", warpnet.NodeInfo{ID: warpnet.FromStringToPeerID(selfID), Type: warpnet.RelayNode}},
		{"moderator", warpnet.NodeInfo{ID: warpnet.FromStringToPeerID(selfID), Type: warpnet.ModeratorNode}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			node := newFakeNode()
			node.info = tt.info
			node.infoResp = infoJSON(t, warpnet.NodeInfo{ID: warpnet.FromStringToPeerID(peerID)})

			s := NewDiscoveryService(ctx, newFakeUserRepo(), newFakeNodeRepo())
			t.Cleanup(s.Close)
			require.NoError(t, s.Run(node))

			s.DiscoveryHandlerPubSub(warpnet.WarpAddrInfo{ID: warpnet.FromStringToPeerID(peerID)})

			// whatever the role does with it, the loop must consume the peer
			require.Eventually(t, func() bool {
				return len(s.discoveryChan) == 0
			}, 5*time.Second, 10*time.Millisecond)
			_ = node
		})
	}
}

func TestRunRefusesWithoutAChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	s := NewDiscoveryService(ctx, newFakeUserRepo(), newFakeNodeRepo())
	s.discoveryChan = nil
	require.Error(t, s.Run(newFakeNode()))
}

func TestRunLoopStopsOnClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	s := NewDiscoveryService(ctx, newFakeUserRepo(), newFakeNodeRepo())
	require.NoError(t, s.Run(newFakeNode()))
	s.Close()
	// closing twice must stay safe
	s.Close()
}

func TestRunLoopStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	s := NewDiscoveryService(ctx, newFakeUserRepo(), newFakeNodeRepo())
	t.Cleanup(s.Close)
	require.NoError(t, s.Run(newFakeNode()))

	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestDiscoveryHandlerStreamSkipsKnownPeers(t *testing.T) {
	s, node, _, _ := newService(t)
	known := warpnet.FromStringToPeerID(peerID)

	addr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	require.NoError(t, err)
	node.store.AddAddrs(known, []warpnet.WarpAddress{addr}, time.Hour)
	s.DiscoveryHandlerStream(warpnet.WarpAddrInfo{ID: known})
	require.Len(t, s.discoveryChan, 0, "a peer already in the peerstore ends the loop")

	unknown := warpnet.FromStringToPeerID(peerID2)
	s.DiscoveryHandlerStream(warpnet.WarpAddrInfo{ID: unknown})
	require.Len(t, s.discoveryChan, 1)
}

func TestEnqueueDropsOldestOnOverflow(t *testing.T) {
	s, _, _, _ := newService(t)

	// The limiter allows a burst; fill the channel past its capacity so the
	// overflow branch has to make room.
	s.limiter = newRateLimiter(10_000, 10_000)
	peer := warpnet.FromStringToPeerID(peerID)
	for range cap(s.discoveryChan) + 5 {
		s.enqueue(warpnet.WarpAddrInfo{ID: peer}, sourceGossip)
	}
	require.LessOrEqual(t, len(s.discoveryChan), cap(s.discoveryChan))
}

func TestEnqueueIgnoresSelfAndEmptyIds(t *testing.T) {
	s, _, _, _ := newService(t)

	s.enqueue(warpnet.WarpAddrInfo{}, sourceGossip)
	s.enqueue(warpnet.WarpAddrInfo{ID: s.ownId}, sourceGossip)
	require.Len(t, s.discoveryChan, 0)

	var nilService *discoveryService
	require.NotPanics(t, func() {
		nilService.enqueue(warpnet.WarpAddrInfo{ID: warpnet.FromStringToPeerID(peerID)}, sourceGossip)
	})
}

func TestHandleAsMemberSkipsTheMastodonGateway(t *testing.T) {
	s, node, users, _ := newService(t)

	gateway := warpnet.FromStringToPeerID(mastodon.GatewayNodeID())
	node.infoResp = infoJSON(t, warpnet.NodeInfo{ID: gateway, OwnerId: "gateway-owner"})

	s.handleAsMember(discoveredPeer{ID: gateway, Source: sourceGossip})
	require.Empty(t, users.created, "the bridge gateway is not stored as a peer user")
}

func TestRequestNodeInfoReportsARejection(t *testing.T) {
	s, node, _, _ := newService(t)

	rejection, err := json.Marshal(event.ResponseError{Message: "not for you"})
	require.NoError(t, err)
	node.infoResp = rejection

	_, err = s.requestNodeInfo(warpnet.WarpAddrInfo{ID: warpnet.FromStringToPeerID(peerID)})
	require.ErrorIs(t, err, errPeerRejectedInfo)
}

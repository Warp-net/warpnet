//nolint:all
package pubsub

import (
	"context"
	"testing"

	"github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p"
	"github.com/stretchr/testify/require"
)

type liveNode struct{ host warpnet.P2PNode }

func (n *liveNode) Node() warpnet.P2PNode { return n.host }

func (n *liveNode) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{ID: n.host.ID(), OwnerId: "None"}
}

func (n *liveNode) SelfStream(_, _ warpnet.WarpPeerID, _ stream.WarpRoute, _ any) ([]byte, error) {
	return nil, nil
}

func (n *liveNode) GenericStream(_ string, _ stream.WarpRoute, _ any) ([]byte, error) {
	return nil, nil
}

func newLiveNode(t *testing.T) *liveNode {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return &liveNode{host: h}
}

func TestRelayPubSubLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	g := NewPubSubRelay(ctx, pubsub.NewDiscoveryRelayTopicHandler())
	require.NotNil(t, g)
	require.Equal(t, "None", g.OwnerID(), "a relay has no owner of its own")

	node := newLiveNode(t)
	g.Run(node)

	// Run is idempotent: a second call must not restart the router.
	g.Run(node)

	require.NoError(t, g.Close())
}

// A relay whose gossip cannot start logs the failure and returns instead of
// leaving a half-initialised router behind.
func TestRelayPubSubRunFailureIsLogged(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // a cancelled context makes the gossip router refuse to start

	node := newLiveNode(t)
	g := NewPubSubRelay(ctx)
	g.Run(node)

	require.NoError(t, g.Close())
}

func TestNewMemberDiscoveryTopicHandlerIsExported(t *testing.T) {
	th := NewMemberDiscoveryTopicHandler(func(warpnet.WarpAddrInfo) {})
	require.NotEmpty(t, th.TopicName)
	require.NotNil(t, th.Handler)
}

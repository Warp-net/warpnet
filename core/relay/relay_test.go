//nolint:all
package relay

import (
	"context"
	"fmt"
	"testing"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	relayclient "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	"github.com/Warp-net/warpnet/core/warpnet"
)

// A member behind a NAT is reached through a circuit, and a circuit whose
// relay advertises a limit is marked limited on both ends. libp2p then treats
// it as no connection: it refuses to open a stream on it, and bitswap drops
// such a peer in its own notifiee, so the CRDT blocks that member holds can
// never be fetched - which the DAG syncer only ever reports as a timeout.
func TestDefaultResourcesLeaveCircuitsUnlimited(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	newHost := func() warpnet.P2PNode {
		h, err := warpnet.NewP2PNode(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		require.NoError(t, err)
		t.Cleanup(func() { _ = h.Close() })
		return h
	}

	relayHost := newHost()
	relayService, err := NewRelay(relayHost)
	require.NoError(t, err)
	t.Cleanup(func() { _ = relayService.Close() })
	relayInfo := peer.AddrInfo{ID: relayHost.ID(), Addrs: relayHost.Addrs()}

	// the holder is only ever reachable through its reservation
	holder := newHost()
	require.NoError(t, holder.Connect(ctx, relayInfo))
	_, err = relayclient.Reserve(ctx, holder, relayInfo)
	require.NoError(t, err)

	const proto = protocol.ID("/warpnet/test/circuit/1.0.0")
	holder.SetStreamHandler(proto, func(s network.Stream) { _ = s.Close() })

	seeker := newHost()
	require.NoError(t, seeker.Connect(ctx, relayInfo))
	circuit, err := multiaddr.NewMultiaddr(fmt.Sprintf(
		"%s/p2p/%s/p2p-circuit/p2p/%s", relayHost.Addrs()[0], relayHost.ID(), holder.ID(),
	))
	require.NoError(t, err)
	require.NoError(t, seeker.Connect(ctx, peer.AddrInfo{
		ID: holder.ID(), Addrs: []multiaddr.Multiaddr{circuit},
	}))

	require.Equal(t, network.Connected, seeker.Network().Connectedness(holder.ID()),
		"a circuit through our own relay must count as a connection, not as network.Limited")

	// the plain context is the point: every caller that does not know to ask
	// for a limited connection - bitswap above all - dials exactly like this
	stream, err := seeker.NewStream(ctx, holder.ID(), proto)
	require.NoError(t, err, "a stream must open on the circuit without the caller allowing a limited connection")
	require.NoError(t, stream.Close())
}

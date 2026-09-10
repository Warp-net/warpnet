//nolint:all
package node

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	"github.com/stretchr/testify/require"
)

func TestHolePunchTracerCoversEveryEvent(t *testing.T) {
	tr := holePunchTracer{}
	remote := peer.ID("12D3KooWTracedPeer")

	require.NotPanics(t, func() { tr.Trace(nil) })

	events := []any{
		&holepunch.StartHolePunchEvt{RTT: 10 * time.Millisecond, RemoteAddrs: []string{"/ip4/1.2.3.4/tcp/1"}},
		&holepunch.HolePunchAttemptEvt{Attempt: 2},
		&holepunch.EndHolePunchEvt{Success: true, EllapsedTime: time.Second},
		&holepunch.EndHolePunchEvt{Success: false, EllapsedTime: time.Second, Error: "timeout"},
		&holepunch.DirectDialEvt{Success: true, EllapsedTime: time.Second},
		&holepunch.DirectDialEvt{Success: false, EllapsedTime: time.Second},
		&holepunch.ProtocolErrorEvt{Error: "unsupported"},
		// an event this tracer does not model must fall through silently
		struct{}{},
	}
	for _, e := range events {
		require.NotPanics(t, func() {
			tr.Trace(&holepunch.Event{Remote: remote, Evt: e})
		})
	}
}

func TestConnTracerListenHooksAreInert(t *testing.T) {
	tr := connTracer{}
	require.NotPanics(t, func() {
		tr.Listen(nil, nil)
		tr.ListenClose(nil, nil)
		tr.Connected(nil, nil)
		tr.Disconnected(nil, nil)
	})
}

// TestConnTracerOnLiveConnections drives the direct-connection paths against a
// real dial, where the network can be asked for the peer's other connections.
func TestConnTracerOnLiveConnections(t *testing.T) {
	a, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })

	b, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = b.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, a.Connect(ctx, peer.AddrInfo{ID: b.ID(), Addrs: b.Addrs()}))

	conns := a.Network().ConnsToPeer(b.ID())
	require.NotEmpty(t, conns)

	tr := connTracer{}
	require.NotPanics(t, func() {
		tr.Connected(a.Network(), conns[0])
		tr.Disconnected(a.Network(), conns[0])
	})
}

var _ network.Notifiee = connTracer{}

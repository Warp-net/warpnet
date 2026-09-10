//nolint:all
package mdns

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/backoff"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

type stubNode struct {
	host      warpnet.P2PNode
	connectFn func(peer.AddrInfo) error
	connected []peer.ID
}

func (s *stubNode) Node() warpnet.P2PNode { return s.host }

func (s *stubNode) Connect(info peer.AddrInfo) error {
	s.connected = append(s.connected, info.ID)
	if s.connectFn != nil {
		return s.connectFn(info)
	}
	return nil
}

func newStubNode(t *testing.T) *stubNode {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return &stubNode{host: h}
}

func TestHandlePeerFoundUsesTheConfiguredHandler(t *testing.T) {
	var got []peer.ID
	m := NewMulticastDNS(context.Background(), func(info warpnet.WarpAddrInfo) {
		got = append(got, info.ID)
	})

	node := newStubNode(t)
	m.service.node = node

	m.service.HandlePeerFound(peer.AddrInfo{ID: "peer-1"})
	require.Equal(t, []peer.ID{"peer-1"}, got)
	require.Empty(t, node.connected, "a custom handler owns the connection decision")
}

func TestHandlePeerFoundFallsBackToConnecting(t *testing.T) {
	t.Run("connects", func(t *testing.T) {
		m := NewMulticastDNS(context.Background(), nil)
		node := newStubNode(t)
		m.service.node = node

		m.service.HandlePeerFound(peer.AddrInfo{ID: "peer-1"})
		require.Equal(t, []peer.ID{"peer-1"}, node.connected)
	})

	t.Run("a backoffed peer is skipped quietly", func(t *testing.T) {
		m := NewMulticastDNS(context.Background(), nil)
		node := newStubNode(t)
		node.connectFn = func(peer.AddrInfo) error { return backoff.ErrBackoffEnabled }
		m.service.node = node

		require.NotPanics(t, func() { m.service.HandlePeerFound(peer.AddrInfo{ID: "peer-1"}) })
	})

	t.Run("a dial failure is logged", func(t *testing.T) {
		m := NewMulticastDNS(context.Background(), nil)
		node := newStubNode(t)
		node.connectFn = func(peer.AddrInfo) error { return errors.New("unreachable") }
		m.service.node = node

		require.NotPanics(t, func() { m.service.HandlePeerFound(peer.AddrInfo{ID: "peer-1"}) })
	})
}

func TestHandlePeerFoundGuards(t *testing.T) {
	var svc *mdnsDiscoveryService
	require.NotPanics(t, func() { svc.HandlePeerFound(peer.AddrInfo{ID: "peer-1"}) })

	m := NewMulticastDNS(context.Background(), nil)
	require.Panics(t, func() { m.service.HandlePeerFound(peer.AddrInfo{ID: "peer-1"}) },
		"discovery before Start attached a node is a programming error")
}

func TestStartAndClose(t *testing.T) {
	m := NewMulticastDNS(context.Background(), nil)
	node := newStubNode(t)

	m.Start(node)
	// Start is idempotent: a second call must not swap the running service out.
	m.Start(node)

	// give the background start a moment so Close has something to close
	time.Sleep(100 * time.Millisecond)

	m.Close()
	// Close is idempotent too.
	m.Close()
}

func TestStartAndCloseAreNilSafe(t *testing.T) {
	var m *MulticastDNS
	require.NotPanics(t, func() { m.Start(nil) })
	require.NotPanics(t, m.Close)

	unstarted := NewMulticastDNS(context.Background(), nil)
	require.NotPanics(t, unstarted.Close)
}

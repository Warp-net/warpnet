//nolint:all
package dht

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/stretchr/testify/require"
)

func newHost(t *testing.T) warpnet.P2PNode {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func memStore() RoutingStorer {
	return dssync.MutexWrap(ds.NewMapDatastore())
}

func TestNewDHTableAppliesOptions(t *testing.T) {
	nodes := []warpnet.WarpAddrInfo{{ID: "peer-1"}}
	d := NewDHTable(context.Background(),
		Network("testnet"),
		RoutingStore(memStore()),
		BootstrapNodes(nodes...),
		AddPeerCallbacks(func(warpnet.WarpPeerID) {}),
		RemovePeerCallbacks(func(warpnet.WarpPeerID) {}),
	)
	require.NotNil(t, d)
	require.Equal(t, "testnet", d.cfg.network)
	require.Equal(t, nodes, d.BootstrapNodes())
	require.Len(t, d.cfg.addCallbacks, 1)
	require.Len(t, d.cfg.removeCallbacks, 1)
}

func TestStartRoutingRequiresNetwork(t *testing.T) {
	d := NewDHTable(context.Background(), RoutingStore(memStore()))
	require.Panics(t, func() { _, _ = d.StartRouting(newHost(t)) })
}

func TestStartRoutingAndClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var (
		mx             sync.Mutex
		added, removed []peer.ID
	)
	d := NewDHTable(ctx,
		Network("testnet"),
		RoutingStore(memStore()),
		// a nil entry alongside a real one exercises the nil guard in the loop
		AddPeerCallbacks(nil, func(id warpnet.WarpPeerID) {
			mx.Lock()
			added = append(added, id)
			mx.Unlock()
		}),
		RemovePeerCallbacks(nil, func(id warpnet.WarpPeerID) {
			mx.Lock()
			removed = append(removed, id)
			mx.Unlock()
		}),
	)

	host := newHost(t)
	routing, err := d.StartRouting(host)
	require.NoError(t, err)
	require.NotNil(t, routing)

	// The routing table owns when these fire; invoke the installed hooks
	// directly so the fan-out to the configured callbacks is covered without
	// depending on DHT server-mode negotiation.
	other := newHost(t)
	d.dht.RoutingTable().PeerAdded(other.ID())
	d.dht.RoutingTable().PeerRemoved(other.ID())

	mx.Lock()
	require.Equal(t, []peer.ID{other.ID()}, added)
	require.Equal(t, []peer.ID{other.ID()}, removed)
	mx.Unlock()

	// An isolated node has no closest peers; the call must still be safe.
	require.Empty(t, d.ClosestPeers())
	require.NotNil(t, d.FindProvidersAsync(ctx, cid.Undef, 1))

	d.Close()
}

func TestStartRoutingWithoutCallbacks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	d := NewDHTable(ctx, Network("testnet"), RoutingStore(memStore()))
	_, err := d.StartRouting(newHost(t))
	require.NoError(t, err)
	t.Cleanup(d.Close)

	require.Empty(t, d.BootstrapNodes())
}

func TestBootstrapSkipsSelfAndTolerantOfDeadPeers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	host := newHost(t)
	unreachable := newHost(t)
	unreachableInfo := warpnet.WarpAddrInfo{ID: unreachable.ID(), Addrs: unreachable.Addrs()}
	require.NoError(t, unreachable.Close())

	d := NewDHTable(ctx,
		Network("testnet"),
		RoutingStore(memStore()),
		BootstrapNodes(
			// this node itself — skipped rather than dialled
			warpnet.WarpAddrInfo{ID: host.ID(), Addrs: host.Addrs()},
			// a peer that is gone: the ping fails without a peer-id mismatch
			unreachableInfo,
		),
	)

	_, err := d.StartRouting(host)
	require.NoError(t, err)
	t.Cleanup(d.Close)

	// Run the bootstrap walk synchronously so the assertions below don't race
	// the background goroutine StartRouting already launched.
	d.bootstrapDHT()

	require.Len(t, d.BootstrapNodes(), 2)
	require.NotEmpty(t, host.Peerstore().Addrs(unreachableInfo.ID),
		"a bootstrap peer's addresses are pinned even when it is unreachable")
}

func TestDefaultCallbacksAreSafe(t *testing.T) {
	require.NotPanics(t, func() {
		defaultNodeAddedCallback("peer-1")
		defaultNodeRemovedCallback("peer-1")
	})
}

func TestStartRendezvousStopsOnSignals(t *testing.T) {
	t.Run("nil table returns immediately", func(t *testing.T) {
		var d *distributedHashTable
		d.startRendezvous(nil)

		started := NewDHTable(context.Background(), Network("testnet"))
		started.startRendezvous(nil)
	})

	t.Run("a cancelled context aborts the wait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		d := NewDHTable(ctx, Network("testnet"), RoutingStore(memStore()))
		_, err := d.StartRouting(newHost(t))
		require.NoError(t, err)
		t.Cleanup(d.Close)

		cancel()
		d.startRendezvous(make(chan struct{})) // never closed: only ctx can end this
	})

	t.Run("a stopped table aborts the wait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		d := NewDHTable(ctx, Network("testnet"), RoutingStore(memStore()))
		_, err := d.StartRouting(newHost(t))
		require.NoError(t, err)

		close(d.stopChan)
		d.startRendezvous(make(chan struct{}))
	})
}

func TestAdvertiseAndFindPeersOnAnEmptyTable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	d := NewDHTable(ctx, Network("testnet"), RoutingStore(memStore()))
	_, err := d.StartRouting(newHost(t))
	require.NoError(t, err)
	t.Cleanup(d.Close)

	rd := drouting.NewRoutingDiscovery(d.dht)
	ns := rendezvousNamespace("testnet")

	// An empty routing table has nowhere to store the record: advertise is a
	// no-op and the peer query yields nothing rather than blocking.
	d.advertise(rd, ns)
	d.findPeers(rd, ns, d.dht.Host().ID())
}

func TestCloseIsNilSafeAndIdempotent(t *testing.T) {
	var d *distributedHashTable
	require.NotPanics(t, d.Close)

	unstarted := NewDHTable(context.Background(), Network("testnet"))
	require.NotPanics(t, unstarted.Close)
}

func TestBootstrapDHTIsNilSafe(t *testing.T) {
	var d *distributedHashTable
	require.NotPanics(t, d.bootstrapDHT)

	unstarted := NewDHTable(context.Background(), Network("testnet"))
	require.NotPanics(t, unstarted.bootstrapDHT)
}

// TestStartRendezvousAdvertisesAndStops drives the rendezvous loop once the
// bootstrap signal is in: it advertises, queries for peers, and then exits on
// the stop channel.
func TestStartRendezvousAdvertisesAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	d := NewDHTable(ctx, Network("testnet"), RoutingStore(memStore()))
	_, err := d.StartRouting(newHost(t))
	require.NoError(t, err)

	// a peer in the routing table takes advertise past its "nowhere to store
	// the record" guard
	other := newHost(t)
	d.dht.Host().Peerstore().AddAddrs(other.ID(), other.Addrs(), time.Hour)
	_, _ = d.dht.RoutingTable().TryAddPeer(other.ID(), true, false)

	bootstrapped := make(chan struct{})
	close(bootstrapped)

	done := make(chan struct{})
	go func() {
		defer close(done)
		d.startRendezvous(bootstrapped)
	}()

	// the first advertise/findPeers pass runs before the ticker; stop the loop
	// as soon as it is under way
	time.Sleep(200 * time.Millisecond)
	close(d.stopChan)

	select {
	case <-done:
	case <-time.After(90 * time.Second):
		t.Fatal("the rendezvous loop did not stop")
	}

	// the table is already stopped: Close must not double-close it
	require.NotPanics(t, func() {
		if err := d.dht.Close(); err != nil {
			t.Logf("dht close: %v", err)
		}
	})
}

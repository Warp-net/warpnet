package dht

import (
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/stretchr/testify/assert"
)

func TestRoutingStore(t *testing.T) {
	cfg := dhtConfig{}
	opt := RoutingStore(nil)
	opt(&cfg)
	assert.Nil(t, cfg.store)
}

func TestRemovePeerCallbacks(t *testing.T) {
	cfg := dhtConfig{}
	cb := func(warpnet.WarpPeerID) {}
	opt := RemovePeerCallbacks(cb)
	opt(&cfg)
	assert.Len(t, cfg.removeCallbacks, 1)
}

func TestAddPeerCallbacks(t *testing.T) {
	cfg := dhtConfig{}
	cb1 := func(warpnet.WarpPeerID) {}
	cb2 := func(warpnet.WarpPeerID) {}
	opt := AddPeerCallbacks(cb1, cb2)
	opt(&cfg)
	assert.Len(t, cfg.addCallbacks, 2)
}

func TestBootstrapNodes(t *testing.T) {
	cfg := dhtConfig{}
	nodes := []warpnet.WarpAddrInfo{{}, {}}
	opt := BootstrapNodes(nodes...)
	opt(&cfg)
	assert.Len(t, cfg.boostrapNodes, 2)
}

func TestNetwork(t *testing.T) {
	cfg := dhtConfig{}
	opt := Network("testnet")
	opt(&cfg)
	assert.Equal(t, "testnet", cfg.network)
}

func TestMultipleOptions(t *testing.T) {
	cfg := dhtConfig{}
	opts := []Option{
		Network("warpnet"),
		AddPeerCallbacks(func(warpnet.WarpPeerID) {}),
		RemovePeerCallbacks(func(warpnet.WarpPeerID) {}),
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	assert.Equal(t, "warpnet", cfg.network)
	assert.Len(t, cfg.addCallbacks, 1)
	assert.Len(t, cfg.removeCallbacks, 1)
}

func TestRendezvousNamespace(t *testing.T) {
	ns := rendezvousNamespace("warpnet")
	assert.Equal(t, "warpnet/rendezvous/warpnet", ns)

	ns = rendezvousNamespace("testnet")
	assert.Equal(t, "warpnet/rendezvous/testnet", ns)
}

//nolint:all
package handler

import (
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
)

type stubStatsProvider struct {
	stats map[string]string
}

func (s stubStatsProvider) Stats() map[string]string {
	if s.stats != nil {
		return s.stats
	}
	return map[string]string{}
}

// stubStatsPeerstore embeds the full interface, overrides only Peers()
type stubStatsPeerstore struct {
	peerstore.Peerstore
	peers []peer.ID
}

func (s stubStatsPeerstore) Peers() peer.IDSlice { return peer.IDSlice(s.peers) }

// stubStatsNetwork embeds the full interface, overrides only Peers()
type stubStatsNetwork struct {
	network.Network
	peers []peer.ID
}

func (s stubStatsNetwork) Peers() []peer.ID { return s.peers }

// stubStatsNode implements StatsNodeInformer
type stubStatsNode struct {
	info      warpnet.NodeInfo
	peerStore stubStatsPeerstore
	net       stubStatsNetwork
}

func (s stubStatsNode) NodeInfo() warpnet.NodeInfo       { return s.info }
func (s stubStatsNode) Peerstore() warpnet.WarpPeerstore { return s.peerStore }
func (s stubStatsNode) Network() warpnet.WarpNetwork     { return s.net }

func TestStreamGetStatsHandler(t *testing.T) {
	t.Run("connected with peers", func(t *testing.T) {
		pid, _ := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
		node := stubStatsNode{
			info: warpnet.NodeInfo{
				OwnerId:   "owner-1",
				Addresses: []string{"/ip4/127.0.0.1/tcp/4001"},
			},
			peerStore: stubStatsPeerstore{peers: []peer.ID{pid}},
			net:       stubStatsNetwork{peers: []peer.ID{pid}},
		}
		h := StreamGetStatsHandler(node, stubStatsProvider{stats: map[string]string{"keys": "100"}})
		resp, err := h(nil, nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		stats := resp.(warpnet.NodeStats)
		if stats.UserId != "owner-1" {
			t.Fatalf("expected user id owner-1, got: %s", stats.UserId)
		}
		if stats.NetworkState != "Connected" {
			t.Fatalf("expected Connected, got: %s", stats.NetworkState)
		}
		if stats.PeersOnline != 1 {
			t.Fatalf("expected 1 peer online, got: %d", stats.PeersOnline)
		}
		if stats.PeersStored != 1 {
			t.Fatalf("expected 1 stored peer, got: %d", stats.PeersStored)
		}
		if stats.PublicAddresses != "/ip4/127.0.0.1/tcp/4001" {
			t.Fatalf("unexpected addresses: %s", stats.PublicAddresses)
		}
		if stats.DatabaseStats["keys"] != "100" {
			t.Fatalf("unexpected db stats: %v", stats.DatabaseStats)
		}
	})

	t.Run("disconnected with no peers", func(t *testing.T) {
		node := stubStatsNode{
			info:      warpnet.NodeInfo{OwnerId: "owner-1"},
			peerStore: stubStatsPeerstore{},
			net:       stubStatsNetwork{},
		}
		h := StreamGetStatsHandler(node, stubStatsProvider{})
		resp, err := h(nil, nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		stats := resp.(warpnet.NodeStats)
		if stats.NetworkState != "Disconnected" {
			t.Fatalf("expected Disconnected, got: %s", stats.NetworkState)
		}
		if stats.PeersOnline != 0 {
			t.Fatalf("expected 0 peers online, got: %d", stats.PeersOnline)
		}
		if stats.PublicAddresses != "Waiting..." {
			t.Fatalf("expected 'Waiting...', got: %s", stats.PublicAddresses)
		}
	})
}

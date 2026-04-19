//nolint:all
package handler

import (
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

type stubNodeInformer struct {
	info warpnet.NodeInfo
}

func (s stubNodeInformer) NodeInfo() warpnet.NodeInfo { return s.info }

// stubInfoConn embeds the interface so we only need to implement methods actually called
type stubInfoConn struct {
	network.Conn
	remotePeerID peer.ID
	remoteAddr   ma.Multiaddr
}

func (c stubInfoConn) RemotePeer() peer.ID           { return c.remotePeerID }
func (c stubInfoConn) RemoteMultiaddr() ma.Multiaddr { return c.remoteAddr }

// stubInfoStream embeds the interface so we only need to implement Conn()
type stubInfoStream struct {
	network.Stream
	conn stubInfoConn
}

func (s stubInfoStream) Conn() network.Conn { return s.conn }

func TestStreamGetInfoHandler(t *testing.T) {
	expectedInfo := warpnet.NodeInfo{
		OwnerId: "owner-1",
	}

	t.Run("returns node info and calls discovery handler", func(t *testing.T) {
		discoveryCalled := false
		addr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
		peerID, _ := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")

		stream := stubInfoStream{conn: stubInfoConn{
			remotePeerID: peerID,
			remoteAddr:   addr,
		}}

		h := StreamGetInfoHandler(stubNodeInformer{info: expectedInfo}, func(info warpnet.WarpAddrInfo) {
			discoveryCalled = true
			if info.ID != peerID {
				t.Fatalf("expected peer ID %s, got %s", peerID, info.ID)
			}
		})
		resp, err := h(nil, stream)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		info := resp.(warpnet.NodeInfo)
		if info.OwnerId != "owner-1" {
			t.Fatalf("expected owner-1, got: %s", info.OwnerId)
		}
		if !discoveryCalled {
			t.Fatal("expected discovery handler to be called")
		}
	})

	t.Run("nil discovery handler does not panic", func(t *testing.T) {
		addr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
		peerID, _ := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")

		stream := stubInfoStream{conn: stubInfoConn{
			remotePeerID: peerID,
			remoteAddr:   addr,
		}}

		h := StreamGetInfoHandler(stubNodeInformer{info: expectedInfo}, nil)
		resp, err := h(nil, stream)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		info := resp.(warpnet.NodeInfo)
		if info.OwnerId != "owner-1" {
			t.Fatalf("expected owner-1, got: %s", info.OwnerId)
		}
	})
}

//nolint:all
package handler

import (
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

type stubPairAuth struct{ token string }

func (s *stubPairAuth) SessionToken() string { return s.token }

type stubDeviceRepo struct{ saved int }

func (s *stubDeviceRepo) SetDevice(_ string, _ domain.Device) error {
	s.saved++
	return nil
}

type stubNodeAddresser struct{}

func (stubNodeAddresser) PublicAddrs() []warpnet.WarpAddress { return nil }

type stubPairConn struct {
	network.Conn
	id peer.ID
}

func (c stubPairConn) LocalPeer() peer.ID  { return c.id }
func (c stubPairConn) RemotePeer() peer.ID { return c.id }

type stubPairStream struct {
	network.Stream
	conn stubPairConn
}

func (s stubPairStream) Conn() network.Conn { return s.conn }

func TestStreamNodesPairingHandler_ReadsLiveToken(t *testing.T) {
	auth := &stubPairAuth{token: "tokenA"}
	devices := &stubDeviceRepo{}
	h := StreamNodesPairingHandler(auth, devices, stubNodeAddresser{})

	peerID, _ := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	stream := stubPairStream{conn: stubPairConn{id: peerID}}
	body, _ := json.Marshal(domain.AuthNodeInfo{Token: "tokenB"})

	if _, err := h(body, stream); err == nil {
		t.Fatal("expected token mismatch while session token is tokenA")
	}

	auth.token = "tokenB"
	if _, err := h(body, stream); err != nil {
		t.Fatalf("expected pairing to succeed after the session token rotated to tokenB, got: %v", err)
	}
	if devices.saved != 1 {
		t.Fatalf("expected device saved once, got %d", devices.saved)
	}
}

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p/core/network"
)

func TestIsFresh(t *testing.T) {
	p := &WarpMiddleware{freshnessWindow: 5 * time.Minute}
	now := time.Now()

	cases := []struct {
		name string
		ts   time.Time
		want bool
	}{
		{"now", now, true},
		{"within past", now.Add(-2 * time.Minute), true},
		{"within future", now.Add(2 * time.Minute), true},
		{"stale", now.Add(-10 * time.Minute), false},
		{"future skew", now.Add(10 * time.Minute), false},
		{"zero", time.Time{}, false},
	}
	for _, c := range cases {
		if got := p.isFresh(c.ts); got != c.want {
			t.Errorf("%s: isFresh=%v want %v", c.name, got, c.want)
		}
	}
}

// A zero freshnessWindow must fall back to the package default.
func TestIsFresh_DefaultsWindow(t *testing.T) {
	p := &WarpMiddleware{}
	if !p.isFresh(time.Now()) {
		t.Fatal("expected now to be fresh with default window")
	}
	if p.isFresh(time.Now().Add(-messageFreshnessWindow - time.Minute)) {
		t.Fatal("expected stale message to fail with default window")
	}
}

type remoteConn struct {
	network.Conn

	local, remote warpnet.WarpPeerID
}

func (c remoteConn) LocalPeer() warpnet.WarpPeerID  { return c.local }
func (c remoteConn) RemotePeer() warpnet.WarpPeerID { return c.remote }

type remoteStream struct {
	warpnet.WarpStream

	conn network.Conn
}

func (s remoteStream) Conn() network.Conn { return s.conn }

func newRemotePeer(t *testing.T) (warpnet.WarpPeerID, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	id, err := warpnet.IDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("peer id: %v", err)
	}
	return id, priv
}

func callAsRemotePeer(
	t *testing.T, mw *WarpMiddleware, ownNodeId, peer warpnet.WarpPeerID,
	privKey ed25519.PrivateKey, route string,
) (reached bool, err error) {
	t.Helper()

	msg := event.Message{
		Body:        json.RawMessage(`{}`),
		MessageId:   "01J0000000000000000000000",
		NodeId:      peer.String(),
		Destination: route,
		Timestamp:   time.Now().UTC(),
	}
	msg.Signature = security.Sign(privKey, msg.SigningBytes())
	payload, merr := json.Marshal(msg)
	if merr != nil {
		t.Fatalf("marshal message: %v", merr)
	}

	client, server := stream.NewLoopbackStream(ownNodeId, ownNodeId, warpnet.WarpProtocolID(route))
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	handler := mw.AuthMiddleware(func(data []byte, s warpnet.WarpStream) (any, error) {
		reached = true
		return []byte(`["ok"]`), nil
	})
	_, err = handler(payload, remoteStream{
		WarpStream: server,
		conn:       remoteConn{local: ownNodeId, remote: peer},
	})
	return reached, err
}

func TestAuthMiddleware_PrivateRouteDeniedForForeignPeer(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	attacker, attackerKey := newRemotePeer(t)

	mw := NewWarpMiddleware(ownNodeId, nil)
	defer mw.Close()

	for _, route := range []string{
		"/private/get/messages/0.0.0",
		"/private/get/notifications/0.0.0",
		"/private/post/user/0.0.0",
		"/private/post/notification/settings/0.0.0",
	} {
		reached, err := callAsRemotePeer(t, mw, ownNodeId, attacker, attackerKey, route)
		if reached {
			t.Errorf("%s: a foreign peer must not reach the handler", route)
		}
		if !errors.Is(err, ErrUnknownClientPeer) {
			t.Errorf("%s: expected an unknown client peer error, got %v", route, err)
		}
	}
}

type stubAliases struct {
	ids []string
	err error
}

func (s stubAliases) GetNodeIDs() ([]string, error) { return s.ids, s.err }

func TestAuthMiddleware_PrivateRouteAllowedForPairedDevice(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	device, deviceKey := newRemotePeer(t)

	route := "/private/get/notifications/0.0.0"

	unpaired := NewWarpMiddleware(ownNodeId, stubAliases{})
	defer unpaired.Close()
	if reached, _ := callAsRemotePeer(t, unpaired, ownNodeId, device, deviceKey, route); reached {
		t.Error("an unpaired device must not reach private routes")
	}

	paired := NewWarpMiddleware(ownNodeId, stubAliases{ids: []string{device.String()}})
	defer paired.Close()
	if reached, _ := callAsRemotePeer(t, paired, ownNodeId, device, deviceKey, route); !reached {
		t.Error("a paired device must reach private routes")
	}
}

func TestAuthMiddleware_UnknownDeviceStaysLockedOut(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	paired, _ := newRemotePeer(t)
	other, otherKey := newRemotePeer(t)

	mw := NewWarpMiddleware(ownNodeId, stubAliases{ids: []string{paired.String()}})
	defer mw.Close()

	reached, _ := callAsRemotePeer(t, mw, ownNodeId, other, otherKey, "/private/get/notifications/0.0.0")
	if reached {
		t.Error("a peer missing from the device store must stay locked out")
	}
}

func TestAuthMiddleware_DeviceLookupFailureDenies(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	device, deviceKey := newRemotePeer(t)

	mw := NewWarpMiddleware(ownNodeId, stubAliases{err: errors.New("db is closed")})
	defer mw.Close()

	reached, _ := callAsRemotePeer(t, mw, ownNodeId, device, deviceKey, "/private/get/notifications/0.0.0")
	if reached {
		t.Error("a failed device lookup must not open the gate")
	}
}

func TestAuthMiddleware_PublicRouteAllowedForForeignPeer(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	other, otherKey := newRemotePeer(t)

	mw := NewWarpMiddleware(ownNodeId, nil)
	defer mw.Close()

	reached, _ := callAsRemotePeer(t, mw, ownNodeId, other, otherKey, "/public/get/tweets/0.0.0")
	if !reached {
		t.Error("public routes must stay open to any peer")
	}
}

func TestAuthMiddleware_PairingStaysOpen(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	other, otherKey := newRemotePeer(t)

	mw := NewWarpMiddleware(ownNodeId, nil)
	defer mw.Close()

	if reached, _ := callAsRemotePeer(t, mw, ownNodeId, other, otherKey, event.PRIVATE_POST_PAIR); !reached {
		t.Errorf("%s: must stay reachable by any peer", event.PRIVATE_POST_PAIR)
	}
}

func TestAuthMiddleware_LegacyReplyRoutesDenied(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	other, otherKey := newRemotePeer(t)

	mw := NewWarpMiddleware(ownNodeId, nil)
	defer mw.Close()

	for _, route := range []string{
		event.PRIVATE_POST_TWEET,
		event.PRIVATE_DELETE_TWEET,
	} {
		if reached, _ := callAsRemotePeer(t, mw, ownNodeId, other, otherKey, route); reached {
			t.Errorf("%s: must not be reachable by a foreign peer", route)
		}
	}
}

func TestAuthMiddleware_TamperedBodyIsRejected(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	peer, key := newRemotePeer(t)

	mw := NewWarpMiddleware(ownNodeId, nil)
	defer mw.Close()

	route := "/public/get/tweets/0.0.0"
	msg := event.Message{
		Body:        json.RawMessage(`{"original":true}`),
		MessageId:   "01J0000000000000000000000",
		NodeId:      peer.String(),
		Destination: route,
		Timestamp:   time.Now().UTC(),
	}
	msg.Signature = security.Sign(key, msg.SigningBytes())
	msg.Body = json.RawMessage(`{"tampered":true}`)
	payload, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}

	_, server := stream.NewLoopbackStream(ownNodeId, ownNodeId, warpnet.WarpProtocolID(route))
	t.Cleanup(func() { _ = server.Close() })

	var reached bool
	_, aerr := mw.AuthMiddleware(func([]byte, warpnet.WarpStream) (any, error) {
		reached = true
		return nil, nil
	})(payload, remoteStream{WarpStream: server, conn: remoteConn{local: ownNodeId, remote: peer}})

	if reached {
		t.Error("a tampered body must not reach the handler")
	}
	if !errors.Is(aerr, ErrInternalNodeError) {
		t.Errorf("expected internal node error, got %v", aerr)
	}
}

func TestIsPrivateRouteAllowed_SelfStream(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	mw := NewWarpMiddleware(ownNodeId, nil)
	defer mw.Close()

	route := stream.WarpRoute("/private/get/messages/0.0.0")
	if !mw.isPrivateRouteAllowed(route, ownNodeId, ownNodeId) {
		t.Error("the node itself must always pass the private route gate")
	}
}

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"io"
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

func TestAuthMiddleware_OversizedPayloadDoesNotDeadlock(t *testing.T) {
	mw := NewWarpMiddleware("peer1")
	defer mw.Close()

	var handlerCalled bool
	handler := mw.AuthMiddleware(func(s warpnet.WarpStream) {
		handlerCalled = true
		_, _ = s.Write([]byte(`{"ok":true}`))
	})

	client, server := stream.NewLoopbackStream("peer1", "/private/post/video/0.0.0")
	go handler(server)

	limit := int64(MaxLimit)
	payload := bytes.Repeat([]byte("A"), int(limit)+4096)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = client.SetDeadline(time.Now().Add(30 * time.Second))
		_, _ = client.Write(payload)
		_ = client.CloseWrite()
		_, _ = io.ReadAll(client)
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("oversized payload deadlocked the caller")
	}

	if handlerCalled {
		t.Error("an over-limit payload must never reach the wrapped handler")
	}
}

func TestAuthMiddleware_PayloadAtLimitIsNotRejectedForSize(t *testing.T) {
	mw := NewWarpMiddleware("peer1")
	defer mw.Close()

	limit := int64(MaxLimit)
	client, server := stream.NewLoopbackStream("peer1", "/private/post/video/0.0.0")
	go mw.AuthMiddleware(func(s warpnet.WarpStream) {})(server)

	payload := bytes.Repeat([]byte("A"), int(limit))

	done := make(chan struct{})
	var resp []byte
	go func() {
		defer close(done)
		_ = client.SetDeadline(time.Now().Add(30 * time.Second))
		_, _ = client.Write(payload)
		_ = client.CloseWrite()
		resp, _ = io.ReadAll(client)
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("payload at the limit deadlocked")
	}

	if len(resp) == 0 {
		t.Error("a payload at the ceiling must still get a response")
	}
}

// remoteConn presents a loopback stream as a connection from another peer, so
// the owner gate sees a remote caller instead of a self-stream.
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

// callAsRemotePeer runs the auth middleware for a request signed by peer and
// arriving over a connection whose local side is ownNodeId, and reports
// whether the wrapped handler was reached.
func callAsRemotePeer(
	t *testing.T, mw *WarpMiddleware, ownNodeId, peer warpnet.WarpPeerID,
	privKey ed25519.PrivateKey, route string,
) (reached bool, resp []byte) {
	t.Helper()

	msg := event.Message{
		Body:        json.RawMessage(`{}`),
		MessageId:   "01J0000000000000000000000",
		NodeId:      peer.String(),
		Destination: route,
		Timestamp:   time.Now().UTC(),
	}
	msg.Signature = security.Sign(privKey, msg.SigningBytes())
	payload, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}

	client, server := stream.NewLoopbackStream(ownNodeId, warpnet.WarpProtocolID(route))
	handler := mw.AuthMiddleware(func(s warpnet.WarpStream) {
		reached = true
		_, _ = s.Write([]byte(`["ok"]`))
		_ = s.Close()
	})
	go handler(remoteStream{
		WarpStream: server,
		conn:       remoteConn{local: ownNodeId, remote: peer},
	})

	_ = client.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := client.Write(payload); err != nil {
		t.Fatalf("write request: %v", err)
	}
	_ = client.CloseWrite()
	resp, _ = io.ReadAll(client)
	return reached, resp
}

func TestAuthMiddleware_PrivateRouteDeniedForForeignPeer(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	attacker, attackerKey := newRemotePeer(t)

	mw := NewWarpMiddleware(ownNodeId)
	defer mw.Close()

	for _, route := range []string{
		"/private/get/messages/0.0.0",
		"/private/get/notifications/0.0.0",
		"/private/post/user/0.0.0",
		"/private/post/notification/settings/0.0.0",
	} {
		reached, resp := callAsRemotePeer(t, mw, ownNodeId, attacker, attackerKey, route)
		if reached {
			t.Errorf("%s: a foreign peer must not reach the handler", route)
		}
		if !bytes.Contains(resp, []byte(ErrUnknownClientPeer.Error())) {
			t.Errorf("%s: expected an unknown client peer response, got %s", route, resp)
		}
	}
}

func TestAuthMiddleware_PrivateRouteAllowedForPairedDevice(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	device, deviceKey := newRemotePeer(t)

	mw := NewWarpMiddleware(ownNodeId)
	defer mw.Close()
	mw.SetPairedDeviceChecker(func(id warpnet.WarpPeerID) bool { return id == device })

	reached, _ := callAsRemotePeer(t, mw, ownNodeId, device, deviceKey, "/private/get/notifications/0.0.0")
	if !reached {
		t.Error("a paired device must reach private routes")
	}
}

func TestAuthMiddleware_PublicRouteAllowedForForeignPeer(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	other, otherKey := newRemotePeer(t)

	mw := NewWarpMiddleware(ownNodeId)
	defer mw.Close()

	reached, _ := callAsRemotePeer(t, mw, ownNodeId, other, otherKey, "/public/get/tweets/0.0.0")
	if !reached {
		t.Error("public routes must stay open to any peer")
	}
}

// Reply forwarding and pairing are node-to-node protocol steps and must keep
// working for peers that own no data on this node.
func TestAuthMiddleware_PeerToPeerPrivateRoutesStayOpen(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	other, otherKey := newRemotePeer(t)

	mw := NewWarpMiddleware(ownNodeId)
	defer mw.Close()

	for _, route := range []string{
		event.PRIVATE_POST_PAIR,
		event.PRIVATE_POST_TWEET,
		event.PRIVATE_DELETE_TWEET,
	} {
		if reached, _ := callAsRemotePeer(t, mw, ownNodeId, other, otherKey, route); !reached {
			t.Errorf("%s: must stay reachable by any peer", route)
		}
	}
}

func TestIsPrivateRouteAllowed_SelfStream(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	mw := NewWarpMiddleware(ownNodeId)
	defer mw.Close()

	route := stream.WarpRoute("/private/get/messages/0.0.0")
	if !mw.isPrivateRouteAllowed(route, ownNodeId, ownNodeId) {
		t.Error("the node itself must always pass the private route gate")
	}
}

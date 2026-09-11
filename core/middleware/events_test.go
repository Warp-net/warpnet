// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// observed drains what the middleware reported about its peers.
func observed(t *testing.T, mw *WarpMiddleware) []warpnet.PeerEvent {
	t.Helper()
	var out []warpnet.PeerEvent
	for {
		select {
		case ev := <-mw.Event():
			out = append(out, ev)
		default:
			return out
		}
	}
}

func callAuth(
	t *testing.T, mw *WarpMiddleware, local, remote warpnet.WarpPeerID, route string, payload []byte,
) {
	t.Helper()
	client, server := stream.NewLoopbackStream(local, remote, warpnet.WarpProtocolID(route))
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	_, _ = mw.AuthMiddleware(func([]byte, warpnet.WarpStream) (any, error) {
		return []byte(`["ok"]`), nil
	})(payload, remoteStream{
		WarpStream: server,
		conn:       remoteConn{local: local, remote: remote},
	})
}

func TestAuthMiddlewareReportsWhatItRefused(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	peer, peerKey := newRemotePeer(t)

	const publicRoute = "/public/post/tweet/0.0.0"
	message := func(route string) event.Message {
		return event.Message{
			Body:        json.RawMessage(`{}`),
			MessageId:   "msg-1",
			NodeId:      peer.String(),
			Destination: route,
			Timestamp:   time.Now().UTC(),
		}
	}
	signed := func(t *testing.T, msg event.Message) []byte {
		t.Helper()
		msg.Signature = security.Sign(peerKey, msg.SigningBytes())
		payload, err := json.Marshal(msg)
		require.NoError(t, err)
		return payload
	}

	for _, tc := range []struct {
		name    string
		route   string
		payload func(t *testing.T) []byte
		want    warpnet.PeerEventType
	}{
		{
			name:    "garbage is a malformed frame",
			route:   publicRoute,
			payload: func(*testing.T) []byte { return []byte("{") },
			want:    warpnet.PeerMalformedFrame,
		},
		{
			name:  "an unsigned message is missing its signature",
			route: publicRoute,
			payload: func(t *testing.T) []byte {
				t.Helper()
				payload, err := json.Marshal(message(publicRoute))
				require.NoError(t, err)
				return payload
			},
			want: warpnet.PeerMissingSignature,
		},
		{
			name:  "a signature that no longer matches is a bad one",
			route: publicRoute,
			payload: func(t *testing.T) []byte {
				t.Helper()
				msg := message(publicRoute)
				msg.Signature = security.Sign(peerKey, msg.SigningBytes())
				msg.Body = json.RawMessage(`{"tampered":true}`) // signed one body, sent another
				payload, err := json.Marshal(msg)
				require.NoError(t, err)
				return payload
			},
			want: warpnet.PeerBadSignature,
		},
		{
			name:  "a message from yesterday is stale",
			route: publicRoute,
			payload: func(t *testing.T) []byte {
				t.Helper()
				msg := message(publicRoute)
				msg.Timestamp = time.Now().Add(-24 * time.Hour).UTC()
				return signed(t, msg)
			},
			want: warpnet.PeerStaleMessage,
		},
		{
			name:  "an unpaired peer may not call a private route",
			route: event.PRIVATE_GET_TIMELINE,
			payload: func(t *testing.T) []byte {
				t.Helper()
				return signed(t, message(event.PRIVATE_GET_TIMELINE))
			},
			want: warpnet.PeerPrivateRouteDenied,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mw := NewWarpMiddleware(ownNodeId, nil)
			t.Cleanup(mw.Close)

			callAuth(t, mw, ownNodeId, peer, tc.route, tc.payload(t))

			events := observed(t, mw)
			require.Len(t, events, 1)
			assert.Equal(t, tc.want, events[0].Type)
			assert.Equal(t, peer.String(), events[0].PeerID, "the remote peer is the one charged")
			assert.Equal(t, tc.route, events[0].Route)
		})
	}
}

func TestAuthMiddlewareReportsNothingAboutASelfStream(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	mw := NewWarpMiddleware(ownNodeId, nil)
	t.Cleanup(mw.Close)

	callAuth(t, mw, ownNodeId, ownNodeId, "/public/post/tweet/0.0.0", []byte("{"))

	assert.Empty(t, observed(t, mw), "a node does not report itself")
}

func TestRateLimiterReportsTheLimitedPeerAndRoute(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	peer, _ := newRemotePeer(t)
	mw := newLimiterMiddlewareForTest(t, ownNodeId)

	route := event.PRIVATE_POST_PAIR // the tightest bucket
	var refused bool
	for range 50 {
		if !callLimited(t, mw, ownNodeId, peer, route) {
			refused = true
			break
		}
	}
	require.True(t, refused, "the bucket must run out")

	events := observed(t, mw)
	require.Len(t, events, 1, "the refusal is reported once")
	assert.Equal(t, warpnet.PeerRateLimited, events[0].Type)
	assert.Equal(t, peer.String(), events[0].PeerID)
	assert.Equal(t, route, events[0].Route,
		"the route travels with it, so a flooded write can be told from a read")
	assert.False(t, stream.WarpRoute(events[0].Route).IsGet())
}

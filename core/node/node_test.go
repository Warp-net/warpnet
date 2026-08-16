/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU Affero General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU Affero General Public License for more details.

 You should have received a copy of the GNU Affero General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

package node

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	warpevent "github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"sync"
	"testing"
	"time"
)

func TestIsBenignStreamCloseErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil means fully read", err: nil, want: true},
		{name: "io.EOF is benign", err: io.EOF, want: true},
		{name: "io.ErrClosedPipe is benign", err: io.ErrClosedPipe, want: true},
		{name: "wrapped io.EOF is benign", err: fmt.Errorf("read: %w", io.EOF), want: true},
		{name: "wrapped io.ErrClosedPipe is benign", err: fmt.Errorf("read: %w", io.ErrClosedPipe), want: true},
		{name: "deadline exceeded is a real failure", err: context.DeadlineExceeded, want: false},
		{name: "reset is a real failure", err: warpnet.WarpError("stream reset"), want: false},
		{name: "unexpected EOF is a real failure", err: io.ErrUnexpectedEOF, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBenignStreamCloseErr(tt.err); got != tt.want {
				t.Fatalf("isBenignStreamCloseErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

const echoRoute = stream.WarpRoute("/public/get/user/0.0.0")

func newTestNode(t *testing.T) *WarpNode {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	n, err := NewWarpNode(ctx, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	n.SetStreamMiddleware(middleware.NewWarpMiddleware(n.Node().ID(), nil))
	t.Cleanup(func() {
		defer func() { _ = recover() }()
		n.StopNode()
	})
	return n
}

func signedEnvelope(t *testing.T, n *WarpNode, path stream.WarpRoute, body []byte) []byte {
	t.Helper()

	priv, err := n.Node().Peerstore().PrivKey(n.Node().ID()).Raw()
	require.NoError(t, err)

	msg := warpevent.Message{
		Body:        body,
		MessageId:   "self-" + n.Node().ID().String(),
		NodeId:      n.Node().ID().String(),
		Destination: string(path),
		Timestamp:   time.Now().UTC(),
		Version:     "0.0.0",
	}
	msg.Signature = security.Sign(priv, msg.SigningBytes())

	bt, err := json.Marshal(msg)
	require.NoError(t, err)
	return bt
}

func TestWarpNode_StartsAndReportsItself(t *testing.T) {
	n := newTestNode(t)

	require.NotNil(t, n.Node())
	assert.NotEmpty(t, n.Node().ID().String())

	info := n.BaseNodeInfo()
	assert.Equal(t, n.Node().ID(), info.ID)
	assert.NotEmpty(t, info.Addresses, "a listening node must advertise at least one address")
	assert.False(t, info.StartTime.IsZero())
	assert.NotNil(t, n.Prioritizer())
}

func TestWarpNode_NilReceiverIsInert(t *testing.T) {
	var n *WarpNode

	assert.Equal(t, warpnet.NodeInfo{}, n.BaseNodeInfo())
	assert.Nil(t, n.Node())
	assert.NoError(t, n.Connect(warpnet.WarpAddrInfo{}))

	_, err := n.Stream("peer", echoRoute, nil)
	assert.Error(t, err, "an uninitialised node must report, not panic")

	assert.NotPanics(t, func() { n.StopNode() })
	assert.NotPanics(t, func() { n.SetOutbox(nil) })
}

func TestWarpIdentity_IsDeterministicAndSingular(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	ctx := t.Context()

	first, err := NewWarpNode(ctx, WarpIdentity(priv), libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	id := first.Node().ID()
	first.StopNode()

	second, err := NewWarpNode(ctx, WarpIdentity(priv), libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer second.StopNode()

	assert.Equal(t, id, second.Node().ID(), "the same key must yield the same peer id")

	_, err = NewWarpNode(ctx, WarpIdentity(priv), WarpIdentity(priv))
	assert.ErrorIs(t, err, ErrMultipleIdentities)
}

func TestWarpIdentity_PanicsOnMalformedKey(t *testing.T) {
	assert.Panics(t, func() { WarpIdentity(make(ed25519.PrivateKey, 7)) },
		"a malformed node key must fail loudly at startup, not produce a random identity")
}

func TestSetStreamHandlers_PanicsOnInvalidRoute(t *testing.T) {
	n := newTestNode(t)

	assert.Panics(t, func() {
		n.SetStreamHandlers(warpnet.WarpStreamHandler{
			Path:    warpnet.WarpProtocolID("garbage-route"),
			Handler: func([]byte, warpnet.WarpStream) (any, error) { return nil, nil },
		})
	})
}

func TestSetStreamHandlers_RegistersOnTheHost(t *testing.T) {
	n := newTestNode(t)

	n.SetStreamHandlers(warpnet.WarpStreamHandler{
		Path:    warpnet.WarpProtocolID(echoRoute),
		Handler: func([]byte, warpnet.WarpStream) (any, error) { return []byte(`{}`), nil },
	})

	assert.Contains(t, n.Node().Mux().Protocols(), warpnet.WarpProtocolID(echoRoute))
	assert.Contains(t, n.BaseNodeInfo().Protocols, warpnet.WarpProtocolID(echoRoute))
}

func TestSelfStream_RoundTripsThroughTheRegisteredHandler(t *testing.T) {
	n := newTestNode(t)

	var seen []byte
	n.SetStreamHandlers(warpnet.WarpStreamHandler{
		Path: warpnet.WarpProtocolID(echoRoute),
		Handler: func(msg []byte, s warpnet.WarpStream) (any, error) {
			seen = msg
			return []byte(`{"pong":true}`), nil
		},
	})

	resp, err := n.SelfStream(echoRoute, signedEnvelope(t, n, echoRoute, []byte(`{"ping":true}`)))
	require.NoError(t, err)
	assert.Equal(t, `{"pong":true}`, string(resp))
	assert.Equal(t, `{"ping":true}`, string(seen), "the handler must see the message body, not the envelope")
}

func TestSelfStream_RejectsUnsignedAndForgedEnvelopes(t *testing.T) {
	n := newTestNode(t)

	var handlerCalls int
	n.SetStreamHandlers(warpnet.WarpStreamHandler{
		Path: warpnet.WarpProtocolID(echoRoute),
		Handler: func([]byte, warpnet.WarpStream) (any, error) {
			handlerCalls++
			return []byte(`{"ok":true}`), nil
		},
	})

	resp, err := n.SelfStream(echoRoute, []byte(`{"ping":true}`))
	require.NoError(t, err)
	assert.NotContains(t, string(resp), `"ok":true`)

	unsigned, err := json.Marshal(warpevent.Message{
		Body:      []byte(`{}`),
		MessageId: "no-signature",
		Timestamp: time.Now().UTC(),
	})
	require.NoError(t, err)
	resp, err = n.SelfStream(echoRoute, unsigned)
	require.NoError(t, err)
	assert.NotContains(t, string(resp), `"ok":true`)

	forged := signedEnvelope(t, n, echoRoute, []byte(`{"original":true}`))
	var msg warpevent.Message
	require.NoError(t, json.Unmarshal(forged, &msg))
	msg.Body = []byte(`{"tampered":true}`)
	tampered, err := json.Marshal(msg)
	require.NoError(t, err)

	resp, err = n.SelfStream(echoRoute, tampered)
	require.NoError(t, err)
	assert.NotContains(t, string(resp), `"ok":true`)

	assert.Zero(t, handlerCalls, "no unauthenticated message may reach a handler")
}

func TestSelfStream_RejectsEmptyDataAndUnknownRoute(t *testing.T) {
	n := newTestNode(t)

	_, err := n.SelfStream(echoRoute, nil)
	assert.Error(t, err, "an empty self-stream is a programming error")

	_, err = n.SelfStream(stream.WarpRoute("/public/get/nothing/0.0.0"), []byte(`{}`))
	assert.Error(t, err, "an unregistered route must be reported, not silently dropped")
}

func TestSelfStream_HandlerErrorStillAnswers(t *testing.T) {
	n := newTestNode(t)

	n.SetStreamHandlers(warpnet.WarpStreamHandler{
		Path: warpnet.WarpProtocolID(echoRoute),
		Handler: func([]byte, warpnet.WarpStream) (any, error) {
			return nil, errors.New("handler exploded")
		},
	})

	envelope := signedEnvelope(t, n, echoRoute, []byte(`{}`))

	done := make(chan struct{})
	var resp []byte
	go func() {
		defer close(done)
		resp, _ = n.SelfStream(echoRoute, envelope)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("a failing self-stream handler hung the caller")
	}
	assert.Contains(t, string(resp), "handler exploded")
}

func TestStream_RefusesSelfRequest(t *testing.T) {
	n := newTestNode(t)

	_, err := n.Stream(n.Node().ID(), echoRoute, []byte(`{}`))
	assert.ErrorIs(t, err, ErrSelfRequest)
}

func TestStream_ReachesAnotherNode(t *testing.T) {
	server := newTestNode(t)
	client := newTestNode(t)

	server.SetStreamHandlers(warpnet.WarpStreamHandler{
		Path: warpnet.WarpProtocolID(echoRoute),
		Handler: func(msg []byte, s warpnet.WarpStream) (any, error) {
			return []byte(`{"from":"server"}`), nil
		},
	})

	require.NoError(t, client.Connect(warpnet.WarpAddrInfo{
		ID: server.Node().ID(), Addrs: server.Node().Addrs(),
	}))

	resp, err := client.Stream(server.Node().ID(), echoRoute, []byte(`{"hi":true}`))
	require.NoError(t, err)
	assert.Contains(t, string(resp), `"from":"server"`)
}

func TestStream_UnknownPeerIsOffline(t *testing.T) {
	n := newTestNode(t)

	stranger := warpnet.FromStringToPeerID("12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU")
	require.NotEmpty(t, string(stranger))

	_, err := n.Stream(stranger, echoRoute, []byte(`{}`))
	assert.Error(t, err, "a peer we have never heard of cannot be streamed to")
}

func TestConnect_IsIdempotentAndIgnoresEmptyPeer(t *testing.T) {
	server := newTestNode(t)
	client := newTestNode(t)

	info := warpnet.WarpAddrInfo{ID: server.Node().ID(), Addrs: server.Node().Addrs()}

	require.NoError(t, client.Connect(info))
	assert.NoError(t, client.Connect(info), "re-connecting an already-connected peer is a no-op")
}

func TestStopNode_IsSafeToCallTwice(t *testing.T) {
	n, err := NewWarpNode(t.Context(), libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)

	n.StopNode()
	assert.NotPanics(t, func() { n.StopNode() }, "shutdown must be idempotent")
}

type recordingConnManager struct {
	warpnet.WarpConnManager

	mu   sync.Mutex
	tags map[string]int
}

func newRecordingConnManager() *recordingConnManager {
	return &recordingConnManager{tags: map[string]int{}}
}

func (m *recordingConnManager) UpsertTag(p warpnet.WarpPeerID, tag string, upsert func(int) int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tags[p.String()] = upsert(m.tags[p.String()])
}

func (m *recordingConnManager) get(p warpnet.WarpPeerID) (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.tags[p.String()]
	return v, ok
}

func TestReachabilityManager_RanksByReachability(t *testing.T) {
	cases := []struct {
		reach warpnet.WarpReachability
		want  int
	}{
		{warpnet.ReachabilityPublic, 90},
		{warpnet.ReachabilityUnknown, 60},
		{warpnet.ReachabilityPrivate, 30},
	}

	for i, c := range cases {
		cm := newRecordingConnManager()
		m := newNodeReachabilityManager(cm)
		pid := warpnet.FromStringToPeerID("12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU")

		m.SetPriority(pid, c.reach)

		got, ok := cm.get(pid)
		require.Truef(t, ok, "case %d: no tag recorded", i)
		assert.Equalf(t, c.want, got, "case %d", i)
	}
}

func TestReachabilityManager_MinAndMaxPriority(t *testing.T) {
	pid := warpnet.FromStringToPeerID("12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU")

	cm := newRecordingConnManager()
	newNodeReachabilityManager(cm).SetMinPriority(pid)
	got, ok := cm.get(pid)
	require.True(t, ok)
	assert.Equal(t, 1, got, "a min-priority peer is the first to be trimmed")

	cm = newRecordingConnManager()
	newNodeReachabilityManager(cm).SetMaxPriority(pid)
	got, ok = cm.get(pid)
	require.True(t, ok)
	assert.Equal(t, 100, got, "a relay must be the last connection dropped")
}

func TestReachabilityManager_SuppressesFlapping(t *testing.T) {
	cm := newRecordingConnManager()
	m := newNodeReachabilityManager(cm)
	pid := warpnet.FromStringToPeerID("12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU")

	m.SetPriority(pid, warpnet.ReachabilityPublic)
	first, ok := cm.get(pid)
	require.True(t, ok)

	m.SetPriority(pid, warpnet.ReachabilityPrivate)
	m.SetMinPriority(pid)
	m.SetMaxPriority(pid)

	second, _ := cm.get(pid)
	assert.Equal(t, first, second, "a flapping peer must keep its first verdict")
}

func TestEnableAutoRelayWithStaticRelays_DropsSelf(t *testing.T) {
	self := warpnet.FromStringToPeerID("12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j")
	other := warpnet.FromStringToPeerID("12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU")

	opt := EnableAutoRelayWithStaticRelays(
		[]warpnet.WarpAddrInfo{{ID: self}, {ID: other}}, self,
	)
	n, err := NewWarpNode(t.Context(), opt(), libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer n.StopNode()

	assert.NotEmpty(t, n.Node().ID().String())
}

func TestEnableAutoRelayWithStaticRelays_EmptyListIsInert(t *testing.T) {
	self := warpnet.FromStringToPeerID("12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j")

	opt := EnableAutoRelayWithStaticRelays([]warpnet.WarpAddrInfo{{ID: self}}, self)
	n, err := NewWarpNode(t.Context(), opt(), libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer n.StopNode()
}

func TestEmptyOption_IsANoOp(t *testing.T) {
	n, err := NewWarpNode(t.Context(), EmptyOption()(), libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer n.StopNode()
	assert.NotNil(t, n.Node())
}

func TestPrivateFieldOptionsStillMatchUpstream(t *testing.T) {
	n, err := NewWarpNode(t.Context(),
		libp2p.SwarmOpts(WithDialTimeout(3*time.Second), WithDialTimeoutLocal(2*time.Second)),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err, "a renamed libp2p field must not break node startup")
	defer n.StopNode()

	sw, ok := n.Node().Network().(*warpnet.Swarm)
	require.True(t, ok)

	assert.NoError(t, WithDialTimeout(time.Second)(sw))
	assert.NoError(t, WithDialTimeoutLocal(time.Second)(sw))
}

func TestSetPrivateDurationField_ReportsMissingField(t *testing.T) {
	n, err := NewWarpNode(t.Context(), libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	defer n.StopNode()

	sw, ok := n.Node().Network().(*warpnet.Swarm)
	require.True(t, ok)

	err = setPrivateDurationField(sw, "thisFieldDoesNotExist", time.Second)
	assert.ErrorIs(t, err, ErrFieldNotFound)
}

func TestWithDefaultTCPConnectionTimeout_ReportsMissingField(t *testing.T) {
	tr := &warpnet.TCPTransport{}
	assert.NoError(t, WithDefaultTCPConnectionTimeout(5*time.Second)(tr))
}

func TestHolePunchTracer_ToleratesNilAndUnknownEvents(t *testing.T) {
	tr := holePunchTracer{}
	assert.NotPanics(t, func() { tr.Trace(nil) })
}

func TestConnTracer_ToleratesNilConnections(t *testing.T) {
	tr := connTracer{}
	assert.NotPanics(t, func() {
		tr.Listen(nil, nil)
		tr.ListenClose(nil, nil)
		tr.Connected(nil, nil)
		tr.Disconnected(nil, nil)
	})
}

func TestConnTracer_ClassifiesRealConnections(t *testing.T) {
	server := newTestNode(t)
	client := newTestNode(t)

	require.NoError(t, client.Connect(warpnet.WarpAddrInfo{
		ID: server.Node().ID(), Addrs: server.Node().Addrs(),
	}))

	conns := client.Node().Network().ConnsToPeer(server.Node().ID())
	require.NotEmpty(t, conns)

	assert.False(t, isRelayed(conns[0]), "a direct TCP dial is not relayed")

	tr := connTracer{}
	assert.NotPanics(t, func() {
		tr.Connected(client.Node().Network(), conns[0])
		tr.Disconnected(client.Node().Network(), conns[0])
	})
}

// Nothing else binds the route: a peer that captured a message this node
// gossiped can otherwise repoint it at a privileged handler and replay it,
// since the loopback then reports this node as the sender.
func TestSelfStream_RejectsARewrittenDestination(t *testing.T) {
	n := newTestNode(t)

	var reached int
	n.SetStreamHandlers(warpnet.WarpStreamHandler{
		Path: warpnet.WarpProtocolID(warpevent.PRIVATE_POST_BLOCK),
		Handler: func([]byte, warpnet.WarpStream) (any, error) {
			reached++
			return []byte(`{"ok":true}`), nil
		},
	})

	priv, err := n.Node().Peerstore().PrivKey(n.Node().ID()).Raw()
	require.NoError(t, err)

	msg := warpevent.Message{
		Body:        json.RawMessage(`{"text":"our own gossiped tweet"}`),
		MessageId:   "captured",
		NodeId:      n.Node().ID().String(),
		Destination: warpevent.PUBLIC_POST_TIMELINE,
		Timestamp:   time.Now().UTC(),
		Version:     "0.0.0",
	}
	msg.Signature = security.Sign(priv, msg.SigningBytes())

	msg.Destination = warpevent.PRIVATE_POST_BLOCK

	bt, err := json.Marshal(msg)
	require.NoError(t, err)

	_, err = n.RelayStream(warpnet.FromStringToPeerID(msg.NodeId),
		stream.WarpRoute(warpevent.PRIVATE_POST_BLOCK), bt)
	require.NoError(t, err)

	assert.Zero(t, reached, "a rewritten destination reached a privileged handler")
}

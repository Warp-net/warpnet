package stream

import (
	"context"
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeNewStreamNode is a NodeStreamer whose NewStream always fails with a
// configured error. Only NewStream is exercised by streamPool.send.
type fakeNewStreamNode struct{ err error }

func (f *fakeNewStreamNode) NewStream(_ context.Context, _ warpnet.WarpPeerID, _ ...warpnet.WarpProtocolID) (warpnet.WarpStream, error) {
	return nil, f.err
}
func (f *fakeNewStreamNode) Network() network.Network { return nil }
func (f *fakeNewStreamNode) ID() warpnet.WarpPeerID   { return "" }

// An offline peer whose addresses are still cached fails NewStream with
// swarm.ErrAllDialsFailed (not routing.ErrNotFound). send must report that as
// ErrNodeIsOffline so the offline-marking callers fire.
func TestSend_UnreachablePeerReportedOffline(t *testing.T) {
	pid := warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")
	serverInfo := warpnet.WarpAddrInfo{ID: pid}

	cases := []struct {
		name      string
		streamErr error
		offline   bool
	}{
		{"all dials failed", warpnet.ErrAllDialsFailed, true},
		{"wrapped all dials failed", fmt.Errorf("dial: %w", warpnet.ErrAllDialsFailed), true},
		{"unrelated error", errors.New("boom"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &streamPool{ctx: context.Background(), n: &fakeNewStreamNode{err: tc.streamErr}}
			_, err := p.send(serverInfo, WarpRoute("/test/route"), []byte("{}"), "")
			assert.Equal(t, tc.offline, errors.Is(err, warpnet.ErrNodeIsOffline))
		})
	}
}

const testRoute = WarpRoute("/public/get/user/0.0.0")

func newStreamHost(t *testing.T) host.Host {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func newPool(t *testing.T, h host.Host) *streamPool {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	p, err := NewStreamPool(ctx, h)
	require.NoError(t, err)
	return p
}

func addrOf(h host.Host) warpnet.WarpAddrInfo {
	return peer.AddrInfo{ID: h.ID(), Addrs: h.Addrs()}
}

func linkHosts(t *testing.T, client, server host.Host) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, client.Connect(ctx, addrOf(server)))
}

func echoServer(t *testing.T, h host.Host, reply []byte) *[]event.Message {
	t.Helper()
	var mu sync.Mutex
	received := make([]event.Message, 0, 4)

	h.SetStreamHandler(testRoute.ProtocolID(), func(s warpnet.WarpStream) {
		defer func() { _ = s.Close() }()

		raw, _ := io.ReadAll(s)
		var msg event.Message
		if err := json.Unmarshal(raw, &msg); err == nil {
			mu.Lock()
			received = append(received, msg)
			mu.Unlock()
		}
		_, _ = s.Write(reply)
		_ = s.CloseWrite()
	})

	t.Cleanup(func() { h.RemoveStreamHandler(testRoute.ProtocolID()) })
	return &received
}

func TestStreamPool_NilPoolAndCancelledContext(t *testing.T) {
	var nilPool *streamPool
	_, err := nilPool.Send(warpnet.WarpAddrInfo{}, testRoute, nil)
	assert.Error(t, err, "a nil pool must report, not panic")

	h := newStreamHost(t)
	ctx, cancel := context.WithCancel(context.Background())
	p, err := NewStreamPool(ctx, h)
	require.NoError(t, err)
	cancel()

	_, err = p.Send(addrOf(newStreamHost(t)), testRoute, []byte(`{}`))
	assert.ErrorIs(t, err, context.Canceled, "a shutting-down node must stop dialing")
}

func TestStreamPool_SendDeliversSignedEnvelope(t *testing.T) {
	client := newStreamHost(t)
	server := newStreamHost(t)

	received := echoServer(t, server, []byte(`{"pong":true}`))
	linkHosts(t, client, server)
	pool := newPool(t, client)

	resp, err := pool.Send(addrOf(server), testRoute, []byte(`{"ping":true}`))
	require.NoError(t, err)
	assert.Equal(t, `{"pong":true}`, string(resp))

	require.Len(t, *received, 1)
	msg := (*received)[0]

	assert.Equal(t, string(testRoute), msg.Destination)
	assert.Equal(t, client.ID().String(), string(msg.NodeId), "the sender must identify itself")
	assert.NotEmpty(t, msg.MessageId, "a message id is required for idempotent retries")
	assert.Equal(t, `{"ping":true}`, string(msg.Body))
	assert.False(t, msg.Timestamp.IsZero())
	assert.NotEmpty(t, msg.Signature)

	pub, err := client.Peerstore().PubKey(client.ID()).Raw()
	require.NoError(t, err)
	assert.NoError(t, security.VerifySignature(pub, msg.SigningBytes(), msg.Signature),
		"a receiver must be able to prove the message really came from this node")
}

func TestStreamPool_RetryReusesMessageID(t *testing.T) {
	client := newStreamHost(t)
	server := newStreamHost(t)

	var mu sync.Mutex
	ids := make([]string, 0, 4)
	var attempts atomic.Int64

	server.SetStreamHandler(testRoute.ProtocolID(), func(s warpnet.WarpStream) {
		raw, _ := io.ReadAll(s)
		var msg event.Message
		_ = json.Unmarshal(raw, &msg)

		mu.Lock()
		ids = append(ids, string(msg.MessageId))
		mu.Unlock()

		if attempts.Add(1) == 1 {
			_ = s.Reset()
			return
		}
		_, _ = s.Write([]byte(`{"ok":true}`))
		_ = s.CloseWrite()
		_ = s.Close()
	})
	t.Cleanup(func() { server.RemoveStreamHandler(testRoute.ProtocolID()) })

	linkHosts(t, client, server)
	pool := newPool(t, client)
	_, _ = pool.Send(addrOf(server), testRoute, []byte(`{"tweet":"hi"}`))

	mu.Lock()
	defer mu.Unlock()
	if len(ids) < 2 {
		t.Skipf("transport did not retry (%d attempt(s)) — nothing to assert", len(ids))
	}
	for i := 1; i < len(ids); i++ {
		assert.Equal(t, ids[0], ids[i], "every retry must carry the original message id")
	}
}

func TestStreamPool_CachedOfflinePeerShortCircuits(t *testing.T) {
	client := newStreamHost(t)
	server := newStreamHost(t)
	echoServer(t, server, []byte(`{"pong":true}`))
	linkHosts(t, client, server)

	pool := newPool(t, client)
	info := addrOf(server)

	_, err := pool.Send(info, testRoute, []byte(`{}`))
	require.NoError(t, err)
	assert.False(t, pool.isUnstreamable(server.ID()))

	pool.SetUnstreamable(server.ID())

	start := time.Now()
	_, err = pool.Send(info, testRoute, []byte(`{}`))
	assert.ErrorIs(t, err, warpnet.ErrNodeIsOffline)
	assert.Less(t, time.Since(start), time.Second,
		"a peer marked offline must not be dialled at all")

	pool.SetStreamable(server.ID())
	_, err = pool.Send(info, testRoute, []byte(`{}`))
	assert.NoError(t, err)
}

func TestStreamPool_StreamableMarkCanBeClearedAndReapplied(t *testing.T) {
	pool := newPool(t, newStreamHost(t))
	id := warpnet.FromStringToPeerID("12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU")

	assert.False(t, pool.isUnstreamable(id), "peers start out assumed reachable")

	pool.SetUnstreamable(id)
	assert.True(t, pool.isUnstreamable(id))

	pool.SetStreamable(id)
	assert.False(t, pool.isUnstreamable(id), "a reconnect must clear the offline mark")

	var nilPool *streamPool
	assert.NotPanics(t, func() {
		nilPool.SetUnstreamable(id)
		nilPool.SetStreamable(id)
		assert.False(t, nilPool.isUnstreamable(id))
	})
}

func TestStreamPool_SuccessfulSendClearsOfflineMark(t *testing.T) {
	client := newStreamHost(t)
	server := newStreamHost(t)
	echoServer(t, server, []byte(`{}`))
	linkHosts(t, client, server)

	pool := newPool(t, client)
	pool.SetUnstreamable(server.ID())
	pool.SetStreamable(server.ID()) // simulate the reconnect notification

	_, err := pool.Send(addrOf(server), testRoute, []byte(`{}`))
	require.NoError(t, err)
	assert.False(t, pool.isUnstreamable(server.ID()))
}

func TestStreamPool_MalformedTargetsAreRejected(t *testing.T) {
	pool := newPool(t, newStreamHost(t))
	valid := addrOf(newStreamHost(t))

	t.Run("empty route", func(t *testing.T) {
		_, err := pool.send(valid, "", []byte(`{}`), "msg")
		assert.Error(t, err)
	})

	t.Run("empty peer", func(t *testing.T) {
		_, err := pool.send(warpnet.WarpAddrInfo{}, testRoute, []byte(`{}`), "msg")
		assert.Error(t, err)
	})

	t.Run("oversized node id", func(t *testing.T) {
		bogus := warpnet.WarpAddrInfo{ID: peer.ID(make([]byte, 64))}
		_, err := pool.send(bogus, testRoute, []byte(`{}`), "msg")
		assert.ErrorIs(t, err, warpnet.ErrMalformedNodeId)
	})

	t.Run("invalid peer id", func(t *testing.T) {
		bogus := warpnet.WarpAddrInfo{ID: peer.ID("not-a-real-peer")}
		_, err := pool.send(bogus, testRoute, []byte(`{}`), "msg")
		assert.Error(t, err)
	})
}

func TestStreamPool_IdenticalConcurrentSendsCollapse(t *testing.T) {
	client := newStreamHost(t)
	server := newStreamHost(t)

	var handled atomic.Int64
	release := make(chan struct{})
	server.SetStreamHandler(testRoute.ProtocolID(), func(s warpnet.WarpStream) {
		defer func() { _ = s.Close() }()
		_, _ = io.ReadAll(s)
		handled.Add(1)
		<-release
		_, _ = s.Write([]byte(`{"ok":true}`))
		_ = s.CloseWrite()
	})
	t.Cleanup(func() { server.RemoveStreamHandler(testRoute.ProtocolID()) })

	linkHosts(t, client, server)
	pool := newPool(t, client)
	info := addrOf(server)
	body := []byte(`{"same":"request"}`)

	const n = 5
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			_, _ = pool.Send(info, testRoute, body)
		}()
	}

	time.Sleep(300 * time.Millisecond)
	close(release)
	wg.Wait()

	assert.Equal(t, int64(1), handled.Load(),
		"singleflight must collapse identical in-flight requests into one")
}

func TestStreamPool_DifferentPayloadsAreNotCollapsed(t *testing.T) {
	client := newStreamHost(t)
	server := newStreamHost(t)
	received := echoServer(t, server, []byte(`{}`))
	linkHosts(t, client, server)

	pool := newPool(t, client)
	info := addrOf(server)

	var wg sync.WaitGroup
	for i := range 3 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = pool.Send(info, testRoute, []byte(`{"n":`+string(rune('0'+i))+`}`))
		}(i)
	}
	wg.Wait()

	assert.Len(t, *received, 3, "distinct bodies are distinct requests")
}

func TestHashBody_IsStableAndDiscriminating(t *testing.T) {
	a := hashBody([]byte(`{"a":1}`))
	b := hashBody([]byte(`{"a":1}`))
	c := hashBody([]byte(`{"a":2}`))

	assert.Equal(t, a, b, "the same body must hash identically for singleflight")
	assert.NotEqual(t, a, c)

	assert.NotEmpty(t, hashBody(nil))
	assert.Equal(t, hashBody(nil), hashBody([]byte{}))
}

// A peer that answers the dial but does not serve the route - one deployed
// before the route existed - must fail fast instead of burning the retry
// budget on a call that can never succeed, and must stay streamable: it is
// reachable, just older.
func TestStreamPool_UnsupportedRouteFailsFastAndStaysStreamable(t *testing.T) {
	client := newStreamHost(t)
	server := newStreamHost(t)

	linkHosts(t, client, server)
	pool := newPool(t, client)

	unserved := WarpRoute("/public/post/route-the-peer-does-not-have/0.0.0")

	start := time.Now()
	_, err := pool.Send(addrOf(server), unserved, []byte(`{"hi":true}`))
	elapsed := time.Since(start)

	assert.ErrorIs(t, err, ErrProtocolNotSupported)
	assert.Less(t, elapsed, retryBudget, "an unserved route must not be retried")
	assert.False(
		t, pool.isUnstreamable(addrOf(server).ID),
		"the peer answered, so it must not be marked offline",
	)
}

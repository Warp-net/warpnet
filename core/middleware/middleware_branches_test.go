//nolint:all
package middleware

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/stretchr/testify/require"
)

func TestLoggingMiddleware(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	mw := NewWarpMiddleware(ownNodeId, nil)
	t.Cleanup(mw.Close)

	client, server := stream.NewLoopbackStream(ownNodeId, ownNodeId, "/public/get/info/0.0.0")
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	t.Run("passes the response through", func(t *testing.T) {
		out, err := mw.LoggingMiddleware(func([]byte, warpnet.WarpStream) (any, error) {
			return "ok", nil
		})(nil, server)
		require.NoError(t, err)
		require.Equal(t, "ok", out)
	})

	t.Run("passes the error through", func(t *testing.T) {
		handlerErr := errors.New("boom")
		_, err := mw.LoggingMiddleware(func([]byte, warpnet.WarpStream) (any, error) {
			return nil, handlerErr
		})(nil, server)
		require.ErrorIs(t, err, handlerErr)
	})
}

func TestIsCacheableResponse(t *testing.T) {
	require.False(t, isCacheableResponse(nil, errors.New("boom")))
	require.False(t, isCacheableResponse(nil, nil))
	require.False(t, isCacheableResponse(event.ResponseError{Message: "nope"}, nil))
	require.True(t, isCacheableResponse([]byte(`{"ok":true}`), nil))
}

// idempotencyCaller returns a caller bound to one loopback stream. The cache
// key includes the remote peer, so every call in a scenario must share it.
func idempotencyCaller(t *testing.T, mw *WarpMiddleware, route string) func(string, warpnet.WarpHandlerFunc) (any, error) {
	t.Helper()

	ownNodeId, _ := newRemotePeer(t)
	client, server := stream.NewLoopbackStream(ownNodeId, ownNodeId, warpnet.WarpProtocolID(route))
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	return func(messageId string, next warpnet.WarpHandlerFunc) (any, error) {
		body := &warpnet.WarpStreamBody{WarpStream: server, MessageId: messageId}
		return mw.IdempotencyMiddleware(next)(nil, body)
	}
}

func TestIdempotencyMiddleware(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)

	t.Run("a plain stream bypasses the cache", func(t *testing.T) {
		mw := NewWarpMiddleware(ownNodeId, nil)
		t.Cleanup(mw.Close)

		client, server := stream.NewLoopbackStream(ownNodeId, ownNodeId, "/private/post/tweet/0.0.0")
		t.Cleanup(func() {
			_ = client.Close()
			_ = server.Close()
		})

		calls := 0
		next := func([]byte, warpnet.WarpStream) (any, error) {
			calls++
			return []byte(`{"id":1}`), nil
		}
		for range 2 {
			_, err := mw.IdempotencyMiddleware(next)(nil, server)
			require.NoError(t, err)
		}
		require.Equal(t, 2, calls, "without a message id there is nothing to deduplicate")
	})

	t.Run("a missing message id bypasses the cache", func(t *testing.T) {
		mw := NewWarpMiddleware(ownNodeId, nil)
		t.Cleanup(mw.Close)

		calls := 0
		next := func([]byte, warpnet.WarpStream) (any, error) {
			calls++
			return []byte(`{"id":1}`), nil
		}
		call := idempotencyCaller(t, mw, "/private/post/tweet/0.0.0")
		for range 2 {
			_, err := call("", next)
			require.NoError(t, err)
		}
		require.Equal(t, 2, calls)
	})

	t.Run("a non-post route bypasses the cache", func(t *testing.T) {
		mw := NewWarpMiddleware(ownNodeId, nil)
		t.Cleanup(mw.Close)

		calls := 0
		next := func([]byte, warpnet.WarpStream) (any, error) {
			calls++
			return []byte(`{"id":1}`), nil
		}
		call := idempotencyCaller(t, mw, "/public/get/info/0.0.0")
		for range 2 {
			_, err := call("msg-1", next)
			require.NoError(t, err)
		}
		require.Equal(t, 2, calls)
	})

	t.Run("a replayed post is answered from the cache", func(t *testing.T) {
		mw := NewWarpMiddleware(ownNodeId, nil)
		t.Cleanup(mw.Close)

		calls := 0
		next := func([]byte, warpnet.WarpStream) (any, error) {
			calls++
			return []byte(`{"id":1}`), nil
		}
		call := idempotencyCaller(t, mw, "/private/post/tweet/0.0.0")
		first, err := call("msg-1", next)
		require.NoError(t, err)
		second, err := call("msg-1", next)
		require.NoError(t, err)

		require.Equal(t, first, second)
		require.Equal(t, 1, calls, "the replay must not re-run the handler")
	})

	t.Run("a failed post is not cached", func(t *testing.T) {
		mw := NewWarpMiddleware(ownNodeId, nil)
		t.Cleanup(mw.Close)

		calls := 0
		next := func([]byte, warpnet.WarpStream) (any, error) {
			calls++
			return nil, errors.New("write failed")
		}
		call := idempotencyCaller(t, mw, "/private/post/tweet/0.0.0")
		for range 2 {
			_, err := call("msg-err", next)
			require.Error(t, err)
		}
		require.Equal(t, 2, calls, "a failure must be retryable")
	})

	t.Run("an error response is not cached", func(t *testing.T) {
		mw := NewWarpMiddleware(ownNodeId, nil)
		t.Cleanup(mw.Close)

		calls := 0
		next := func([]byte, warpnet.WarpStream) (any, error) {
			calls++
			return event.ResponseError{Message: "nope"}, nil
		}
		call := idempotencyCaller(t, mw, "/private/post/tweet/0.0.0")
		for range 2 {
			_, err := call("msg-resp-err", next)
			require.NoError(t, err)
		}
		require.Equal(t, 2, calls)
	})

	t.Run("concurrent replays share one execution", func(t *testing.T) {
		mw := NewWarpMiddleware(ownNodeId, nil)
		t.Cleanup(mw.Close)

		var (
			mu      sync.Mutex
			calls   int
			release = make(chan struct{})
		)
		next := func([]byte, warpnet.WarpStream) (any, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			<-release
			return []byte(`{"id":1}`), nil
		}

		call := idempotencyCaller(t, mw, "/private/post/tweet/0.0.0")

		const followers = 4
		var wg sync.WaitGroup
		wg.Add(followers)
		for range followers {
			go func() {
				defer wg.Done()
				_, _ = call("msg-race", next)
			}()
		}
		time.Sleep(50 * time.Millisecond)
		close(release)
		wg.Wait()

		mu.Lock()
		defer mu.Unlock()
		require.Equal(t, 1, calls, "followers wait on the leader instead of re-running")
	})
}

func TestAuthMiddlewareRejectsMalformedInput(t *testing.T) {
	ownNodeId, key := newRemotePeer(t)
	mw := NewWarpMiddleware(ownNodeId, nil)
	t.Cleanup(mw.Close)

	const route = "/public/post/tweet/0.0.0"
	client, server := stream.NewLoopbackStream(ownNodeId, ownNodeId, route)
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	call := func(payload []byte, conn remoteConn) error {
		_, err := mw.AuthMiddleware(func([]byte, warpnet.WarpStream) (any, error) {
			return nil, nil
		})(payload, remoteStream{WarpStream: server, conn: conn})
		return err
	}

	live := remoteConn{local: ownNodeId, remote: ownNodeId}

	t.Run("garbage payload", func(t *testing.T) {
		require.ErrorIs(t, call([]byte("{"), live), ErrInternalNodeError)
	})

	t.Run("missing message id", func(t *testing.T) {
		payload, err := json.Marshal(event.Message{Body: json.RawMessage(`{}`)})
		require.NoError(t, err)
		require.ErrorIs(t, call(payload, live), ErrInternalNodeError)
	})

	t.Run("missing signature", func(t *testing.T) {
		payload, err := json.Marshal(event.Message{
			Body: json.RawMessage(`{}`), MessageId: "msg-1", Timestamp: time.Now().UTC(),
		})
		require.NoError(t, err)
		require.ErrorIs(t, call(payload, live), ErrInternalNodeError)
	})

	t.Run("empty remote peer", func(t *testing.T) {
		msg := event.Message{
			Body: json.RawMessage(`{}`), MessageId: "msg-1", Timestamp: time.Now().UTC(),
		}
		msg.Signature = security.Sign(key, msg.SigningBytes())
		payload, err := json.Marshal(msg)
		require.NoError(t, err)

		require.ErrorIs(t, call(payload, remoteConn{local: ownNodeId}), ErrInternalNodeError)
	})

	t.Run("stale message from a remote peer", func(t *testing.T) {
		peer, peerKey := newRemotePeer(t)
		msg := event.Message{
			Body:        json.RawMessage(`{}`),
			MessageId:   "msg-stale",
			NodeId:      peer.String(),
			Destination: route,
			Timestamp:   time.Now().Add(-24 * time.Hour).UTC(),
		}
		msg.Signature = security.Sign(peerKey, msg.SigningBytes())
		payload, err := json.Marshal(msg)
		require.NoError(t, err)

		require.ErrorIs(t, call(payload, remoteConn{local: ownNodeId, remote: peer}), ErrStaleMessage)
	})
}

func TestAuthMiddlewareRejectsAnUnreadyConnection(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	mw := NewWarpMiddleware(ownNodeId, nil)
	t.Cleanup(mw.Close)

	client, server := stream.NewLoopbackStream(ownNodeId, ownNodeId, "/public/post/tweet/0.0.0")
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	_, err := mw.AuthMiddleware(func([]byte, warpnet.WarpStream) (any, error) {
		return nil, nil
	})(nil, remoteStream{WarpStream: server, conn: nil})
	require.ErrorIs(t, err, ErrInternalNodeError)
}

func TestCloseIsIdempotent(t *testing.T) {
	ownNodeId, _ := newRemotePeer(t)
	mw := NewWarpMiddleware(ownNodeId, nil)
	require.NotPanics(t, mw.Close)
	require.NotPanics(t, mw.Close)
}

func TestCloseExpirableLRUIgnoresForeignValues(t *testing.T) {
	require.NotPanics(t, func() { closeExpirableLRU(nil) })
	require.NotPanics(t, func() { closeExpirableLRU("not a cache") })
	require.NotPanics(t, func() { closeExpirableLRU((*idempotencyCache)(nil)) })
	require.NotPanics(t, func() { closeExpirableLRU(&struct{ done int }{}) })
}

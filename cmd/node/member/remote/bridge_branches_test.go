//nolint:all
package remote

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/flynn/noise"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestSameOrigin(t *testing.T) {
	req := func(origin string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "http://node.local/ws", nil)
		r.Host = "node.local"
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}

	require.True(t, sameOrigin(req("")), "non-browser clients send no Origin")
	require.True(t, sameOrigin(req("http://node.local")))
	require.True(t, sameOrigin(req("https://node.local")))
	require.False(t, sameOrigin(req("http://evil.example")))
	require.False(t, sameOrigin(req("://not-a-url")))
}

func TestEnrolmentIgnoresEmptyKeys(t *testing.T) {
	b := NewBridgeHandler(nil, nil, nil, nil)

	require.False(t, b.isEnrolled(nil))
	b.enroll(nil)
	require.False(t, b.isEnrolled(nil))

	b.enroll([]byte("client-key"))
	require.True(t, b.isEnrolled([]byte("client-key")))
	require.False(t, b.isEnrolled([]byte("other-key")))
}

func TestHandleRejectsCrossOriginUpgrades(t *testing.T) {
	srv, _, _ := newTestBridge(t)

	header := http.Header{}
	header.Set("Origin", "http://evil.example")

	_, resp, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", header,
	)
	require.Error(t, err, "a page on another origin must not open the bridge")
	if resp != nil {
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
	}
}

func TestHandleRejectsAFailedHandshake(t *testing.T) {
	srv, _, _ := newTestBridge(t)

	conn, resp, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", http.Header{},
	)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	t.Cleanup(func() { _ = conn.Close() })

	// garbage instead of the first Noise message: the server must drop us
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, []byte("not-a-handshake")))

	_, _, err = conn.ReadMessage()
	require.Error(t, err)
}

func TestHandleDropsUndecryptableFrames(t *testing.T) {
	srv, _, _ := newTestBridge(t)
	c := dial(t, srv, clientKey(t))

	require.NoError(t, c.conn.WriteMessage(websocket.BinaryMessage, []byte("garbage")))

	_, _, err := c.conn.ReadMessage()
	require.Error(t, err, "a frame that fails to decrypt closes the connection")
}

func TestHandleSkipsMalformedEnvelopes(t *testing.T) {
	srv, _, _ := newTestBridge(t)
	c := dial(t, srv, clientKey(t))

	sealed, err := c.session.Encrypt([]byte("{"))
	require.NoError(t, err)
	require.NoError(t, c.conn.WriteMessage(websocket.BinaryMessage, sealed))

	// the connection survives: a well-formed request still gets an answer
	resp := c.send(t, pathIsFirstRun, struct{}{})
	require.NotNil(t, resp.Body)
}

func TestLoginRejectsAMalformedPayload(t *testing.T) {
	srv, _, _ := newTestBridge(t)
	c := dial(t, srv, clientKey(t))

	resp := c.send(t, event.PRIVATE_POST_LOGIN, json.RawMessage(`"not-an-object"`))
	require.NotEmpty(t, responseError(t, resp).Message)
}

func TestCallWithoutAnAttachedNode(t *testing.T) {
	staticKey, err := noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256).
		GenerateKeypair(nil)
	require.NoError(t, err)

	auth := newFakeAuth(t)
	// deliberately no AttachNode: the dispatcher must say so instead of panicking
	handler := NewBridgeHandler(
		func(read func() ([]byte, error), write func([]byte) error) (Channel, error) {
			return security.NoiseHandshake(staticKey, read, write)
		},
		auth,
		security.PSK("test-psk"),
		func() bool { return false },
	)
	srv := httptest.NewServer(handler.Handle())
	t.Cleanup(srv.Close)

	c := dial(t, srv, clientKey(t))
	c.send(t, event.PRIVATE_POST_LOGIN, event.LoginEvent{Username: testUsername, Password: testPassword})

	resp := c.send(t, event.PRIVATE_POST_TWEET, struct{}{})
	require.Contains(t, responseError(t, resp).Message, "not attached server node")
}

func TestNewStaticHandler(t *testing.T) {
	h, err := NewStaticHandler()
	require.NoError(t, err)
	require.NotNil(t, h)

	t.Run("serves the index for the root", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("falls back to the index for client-side routes", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/profile/someone", nil))
		require.Equal(t, http.StatusOK, w.Code)
	})
}

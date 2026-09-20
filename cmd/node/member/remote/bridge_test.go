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

package remote

import (
	"crypto/ed25519"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/selfupdate"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/flynn/noise"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testUsername = "owner"
	testPassword = "Owner1234$"
)

type fakeAuth struct {
	mx            sync.Mutex
	authenticated bool
	logins        int
	logouts       int
	priv          ed25519.PrivateKey
}

func newFakeAuth(t *testing.T) *fakeAuth {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	return &fakeAuth{priv: priv}
}

func (f *fakeAuth) AuthLogin(message event.LoginEvent, _ security.PSK) (event.LoginResponse, error) {
	f.mx.Lock()
	defer f.mx.Unlock()
	f.logins++
	if f.authenticated {
		return event.LoginResponse{}, errors.New("already authenticated")
	}
	if message.Username != testUsername || message.Password != testPassword {
		return event.LoginResponse{}, errors.New("authentication failed")
	}
	f.authenticated = true
	return event.LoginResponse{UserId: "owner-id", Token: "pairing-token", ID: "node-id"}, nil
}

func (f *fakeAuth) AuthLogout() {
	f.mx.Lock()
	defer f.mx.Unlock()
	f.logouts++
	f.authenticated = false
}

func (f *fakeAuth) Reset() {
	f.mx.Lock()
	defer f.mx.Unlock()
	f.authenticated = false
}

func (f *fakeAuth) PrivateKey() ed25519.PrivateKey { return f.priv }

func (f *fakeAuth) IsAuthenticated() bool {
	f.mx.Lock()
	defer f.mx.Unlock()
	return f.authenticated
}

type fakeNode struct {
	mx    sync.Mutex
	calls []string
}

func (n *fakeNode) SelfStream(_, _ warpnet.WarpPeerID, path stream.WarpRoute, _ any) ([]byte, error) {
	n.mx.Lock()
	defer n.mx.Unlock()
	n.calls = append(n.calls, string(path))
	return json.RawMessage(`{"ok":true}`), nil
}

func (n *fakeNode) callCount() int {
	n.mx.Lock()
	defer n.mx.Unlock()
	return len(n.calls)
}

func newTestBridge(t *testing.T) (*httptest.Server, *fakeAuth, *fakeNode) {
	t.Helper()
	srv, auth, node, _ := newTestBridgeWithApprover(t)
	return srv, auth, node
}

func newTestBridgeWithApprover(t *testing.T) (*httptest.Server, *fakeAuth, *fakeNode, *selfupdate.UserApprover) {
	t.Helper()

	staticKey, err := noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256).
		GenerateKeypair(nil)
	require.NoError(t, err)

	auth := newFakeAuth(t)
	node := &fakeNode{}
	handler := NewBridgeHandler(
		func(read func() ([]byte, error), write func([]byte) error) (Channel, error) {
			return security.NoiseHandshake(staticKey, read, write)
		},
		auth,
		security.PSK("test-psk"),
		func() bool { return false },
	)
	handler.AttachNode(node)

	approver := selfupdate.NewUserApprover(t.Context())
	handler.AttachUpdater(approver)

	srv := httptest.NewServer(handler.Handle())
	t.Cleanup(srv.Close)
	return srv, auth, node, approver
}

func clientKey(t *testing.T) noise.DHKey {
	t.Helper()
	key, err := security.GenerateNoiseKey()
	require.NoError(t, err)
	return key
}

type testClient struct {
	conn    *websocket.Conn
	session *security.NoiseSession
}

func dial(t *testing.T, srv *httptest.Server, key noise.DHKey) *testClient {
	t.Helper()

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", http.Header{})
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	t.Cleanup(func() { _ = conn.Close() })

	session, err := security.NoiseHandshakeInitiator(key,
		func() ([]byte, error) {
			_, frame, err := conn.ReadMessage()
			return frame, err
		},
		func(msg []byte) error {
			return conn.WriteMessage(websocket.BinaryMessage, msg)
		},
	)
	require.NoError(t, err, "the handshake itself succeeds: what it proves is who the client is")
	return &testClient{conn: conn, session: session}
}

func (c *testClient) send(t *testing.T, path string, body any) event.Message {
	t.Helper()

	raw, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := json.Marshal(event.Message{
		MessageId:   "test-message-id",
		Destination: path,
		Body:        raw,
		Timestamp:   time.Now(),
	})
	require.NoError(t, err)

	sealed, err := c.session.Encrypt(req)
	require.NoError(t, err)
	require.NoError(t, c.conn.WriteMessage(websocket.BinaryMessage, sealed))

	require.NoError(t, c.conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, frame, err := c.conn.ReadMessage()
	require.NoError(t, err)

	plain, err := c.session.Decrypt(frame)
	require.NoError(t, err)

	var resp event.Message
	require.NoError(t, json.Unmarshal(plain, &resp))
	return resp
}

func responseError(t *testing.T, resp event.Message) event.ResponseError {
	t.Helper()
	var e event.ResponseError
	require.NoError(t, json.Unmarshal(resp.Body, &e))
	return e
}

func TestBridge_UnknownClientCannotCallRoutes(t *testing.T) {
	srv, _, node := newTestBridge(t)
	client := dial(t, srv, clientKey(t))

	resp := client.send(t, event.PRIVATE_POST_PAIR, map[string]string{"token": "whatever"})

	assert.Equal(t, http.StatusUnauthorized, responseError(t, resp).Code)
	assert.Zero(t, node.callCount(), "no route may reach the node unauthenticated")
}

func TestBridge_UnknownClientCannotLogTheOwnerOut(t *testing.T) {
	srv, auth, _ := newTestBridge(t)
	owner := dial(t, srv, clientKey(t))
	owner.send(t, event.PRIVATE_POST_LOGIN, event.LoginEvent{Username: testUsername, Password: testPassword})

	attacker := dial(t, srv, clientKey(t))
	resp := attacker.send(t, event.PRIVATE_POST_LOGOUT, struct{}{})

	assert.Equal(t, http.StatusUnauthorized, responseError(t, resp).Code)
	assert.Zero(t, auth.logouts)
	assert.True(t, auth.IsAuthenticated(), "the owner stays signed in")
}

func TestBridge_FirstRunProbeNeedsNoAuthorization(t *testing.T) {
	srv, _, _ := newTestBridge(t)

	resp := dial(t, srv, clientKey(t)).send(t, pathIsFirstRun, struct{}{})

	assert.Equal(t, json.RawMessage(`false`), resp.Body)
}

func TestBridge_LoginEnrollsOnlyTheKeyThatProvedThePassword(t *testing.T) {
	srv, _, node := newTestBridge(t)

	browser := clientKey(t)
	owner := dial(t, srv, browser)
	owner.send(t, event.PRIVATE_POST_LOGIN, event.LoginEvent{Username: testUsername, Password: testPassword})

	assert.Equal(t, json.RawMessage(`{"ok":true}`), owner.send(t, event.PRIVATE_POST_TWEET, struct{}{}).Body)

	reloaded := dial(t, srv, browser)
	assert.Equal(t, json.RawMessage(`{"ok":true}`), reloaded.send(t, event.PRIVATE_POST_TWEET, struct{}{}).Body)

	stranger := dial(t, srv, clientKey(t))
	resp := stranger.send(t, event.PRIVATE_POST_TWEET, struct{}{})
	assert.Equal(t, http.StatusUnauthorized, responseError(t, resp).Code)

	assert.Equal(t, 2, node.callCount())
}

func TestBridge_FailedLoginEnrollsNothing(t *testing.T) {
	srv, _, node := newTestBridge(t)
	browser := clientKey(t)
	client := dial(t, srv, browser)

	client.send(t, event.PRIVATE_POST_LOGIN, event.LoginEvent{Username: testUsername, Password: "wrong-password"})
	assert.Equal(t, http.StatusUnauthorized,
		responseError(t, client.send(t, event.PRIVATE_POST_TWEET, struct{}{})).Code)

	reconnected := dial(t, srv, browser)
	assert.Equal(t, http.StatusUnauthorized,
		responseError(t, reconnected.send(t, event.PRIVATE_POST_TWEET, struct{}{})).Code)
	assert.Zero(t, node.callCount())
}

func TestBridge_SignedInDashboardAnswersForTheWaitingRelease(t *testing.T) {
	srv, _, node, approver := newTestBridgeWithApprover(t)

	owner := dial(t, srv, clientKey(t))
	owner.send(t, event.PRIVATE_POST_LOGIN, event.LoginEvent{Username: testUsername, Password: testPassword})

	verdicts := make(chan bool, 1)
	go func() {
		verdicts <- approver.IsUpdateAllowed(domain.UpdateInfo{CurrentVersion: "0.7.1", NewVersion: "0.7.2"})
	}()

	var pending domain.UpdateInfo
	require.Eventually(t, func() bool {
		resp := owner.send(t, event.PRIVATE_GET_UPDATE, struct{}{})
		require.NoError(t, json.Unmarshal(resp.Body, &pending))
		return pending.NewVersion != ""
	}, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, "0.7.2", pending.NewVersion)

	resp := owner.send(t, event.PRIVATE_POST_UPDATE, event.UpdateEvent{IsAllowed: true})
	assert.Equal(t, json.RawMessage(`["update_answered"]`), resp.Body)

	select {
	case allowed := <-verdicts:
		assert.True(t, allowed)
	case <-time.After(5 * time.Second):
		t.Fatal("the answer never reached the update service")
	}
	assert.Zero(t, node.callCount(), "the update handshake never reaches the node")
}

func TestBridge_UnknownClientCannotAnswerForTheRelease(t *testing.T) {
	srv, _, _, approver := newTestBridgeWithApprover(t)

	verdicts := make(chan bool, 1)
	go func() {
		verdicts <- approver.IsUpdateAllowed(domain.UpdateInfo{CurrentVersion: "0.7.1", NewVersion: "0.7.2"})
	}()
	require.Eventually(t, func() bool {
		return approver.GetPendingUpdate().NewVersion != ""
	}, 5*time.Second, 10*time.Millisecond)

	attacker := dial(t, srv, clientKey(t))
	assert.Equal(t, http.StatusUnauthorized,
		responseError(t, attacker.send(t, event.PRIVATE_GET_UPDATE, struct{}{})).Code)
	assert.Equal(t, http.StatusUnauthorized,
		responseError(t, attacker.send(t, event.PRIVATE_POST_UPDATE, event.UpdateEvent{IsAllowed: true})).Code)

	select {
	case <-verdicts:
		t.Fatal("an unauthenticated client replaced the node's binary")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBridge_LogoutRevokesAuthorityUntilNextLogin(t *testing.T) {
	srv, _, node := newTestBridge(t)

	browser := clientKey(t)
	owner := dial(t, srv, browser)
	owner.send(t, event.PRIVATE_POST_LOGIN, event.LoginEvent{Username: testUsername, Password: testPassword})
	require.Equal(t, json.RawMessage(`["logged_out"]`), owner.send(t, event.PRIVATE_POST_LOGOUT, struct{}{}).Body)

	assert.Equal(t, http.StatusUnauthorized,
		responseError(t, owner.send(t, event.PRIVATE_POST_TWEET, struct{}{})).Code)
	assert.Zero(t, node.callCount())

	owner.send(t, event.PRIVATE_POST_LOGIN, event.LoginEvent{Username: testUsername, Password: testPassword})
	assert.Equal(t, json.RawMessage(`{"ok":true}`), owner.send(t, event.PRIVATE_POST_TWEET, struct{}{}).Body)
}

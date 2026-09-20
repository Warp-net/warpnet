//nolint:all
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	stdjson "encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/member/auth"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/stretchr/testify/require"
)

type stubAuthService struct {
	loginFn    func(event.LoginEvent, security.PSK) (event.LoginResponse, error)
	privateKey ed25519.PrivateKey
	loggedOut  bool
	reset      bool
}

func (s *stubAuthService) AuthLogin(ev event.LoginEvent, psk security.PSK) (event.LoginResponse, error) {
	if s.loginFn != nil {
		return s.loginFn(ev, psk)
	}
	return event.LoginResponse{}, nil
}

func (s *stubAuthService) AuthLogout()                        { s.loggedOut = true }
func (s *stubAuthService) Reset()                             { s.reset = true }
func (s *stubAuthService) PrivateKey() ed25519.PrivateKey     { return s.privateKey }
func (s *stubAuthService) Storage() auth.AuthPersistencyLayer { return nil }

type stubNodeServer struct {
	info       warpnet.NodeInfo
	streamFn   func(path stream.WarpRoute, data any) ([]byte, error)
	stopped    bool
	startCalls int
}

func (s *stubNodeServer) SelfStream(_, _ warpnet.WarpPeerID, path stream.WarpRoute, data any) ([]byte, error) {
	if s.streamFn != nil {
		return s.streamFn(path, data)
	}
	return []byte(`{"ok":true}`), nil
}

func (s *stubNodeServer) NodeInfo() warpnet.NodeInfo { return s.info }
func (s *stubNodeServer) Stop()                      { s.stopped = true }
func (s *stubNodeServer) Start() error               { s.startCalls++; return nil }

type stubNodeUpdater struct {
	pending domain.UpdateInfo
	answers chan bool
	closed  bool
}

func (s *stubNodeUpdater) Run(func())                          {}
func (s *stubNodeUpdater) GetPendingUpdate() domain.UpdateInfo { return s.pending }
func (s *stubNodeUpdater) AnswerUpdate(isAllowed bool)         { s.answers <- isAllowed }
func (s *stubNodeUpdater) Close()                              { s.closed = true }

func testKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return priv
}

// liveApp assembles an App in the state startup leaves it: mutex and channel
// created, auth and node attached.
func liveApp(t *testing.T, authSvc *stubAuthService, node *stubNodeServer) *App {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	id, err := warpnet.IDFromPublicKey(pub)
	require.NoError(t, err)

	if node != nil {
		node.info = warpnet.NodeInfo{ID: id}
	}
	a := NewApp()
	a.mx = new(sync.RWMutex)
	a.readyChan = make(chan domain.AuthNodeInfo, 1)
	a.auth = authSvc
	if node != nil {
		a.node = node
	}
	return a
}

func TestNewApp(t *testing.T) {
	a := NewApp()
	require.NotNil(t, a)
	require.False(t, a.IsFirstRun(), "a pre-startup app has no database to ask")
	require.Equal(t, config.Config().Node.Network, a.Network())
}

func TestSelectNetworkRejectsUnknown(t *testing.T) {
	a := NewApp()
	err := a.SelectNetwork("not-a-network")
	require.ErrorIs(t, err, errUnknownNetwork)
}

func TestDeepLinkPlumbing(t *testing.T) {
	t.Run("nil app is inert", func(t *testing.T) {
		var a *App
		require.NotPanics(t, func() { a.SetPendingDeepLink("warpnet://x") })
		require.Empty(t, a.ConsumePendingDeepLink())
		require.NotPanics(t, func() { a.NotifyDeepLink("warpnet://x") })
	})

	t.Run("pre-startup app stashes without a mutex", func(t *testing.T) {
		a := NewApp()
		a.SetPendingDeepLink("warpnet://cold")
		require.Equal(t, "warpnet://cold", a.deepLink)
		require.Empty(t, a.ConsumePendingDeepLink(), "nothing can be consumed before startup")
	})

	t.Run("started app hands the link over exactly once", func(t *testing.T) {
		a := liveApp(t, &stubAuthService{}, nil)
		a.SetPendingDeepLink("warpnet://link")
		require.Equal(t, "warpnet://link", a.ConsumePendingDeepLink())
		require.Empty(t, a.ConsumePendingDeepLink())
	})

	t.Run("notify without a wails context only stashes", func(t *testing.T) {
		a := liveApp(t, &stubAuthService{}, nil)
		a.NotifyDeepLink("")
		require.Empty(t, a.ConsumePendingDeepLink(), "an empty link is ignored")

		a.NotifyDeepLink("warpnet://notified")
		require.Equal(t, "warpnet://notified", a.ConsumePendingDeepLink())
	})
}

func TestNewErrorResp(t *testing.T) {
	var resp event.ResponseError
	require.NoError(t, json.Unmarshal(newErrorResp("boom"), &resp))
	require.Equal(t, "boom", resp.Message)
	require.Equal(t, 500, resp.Code)
}

func TestAppCall(t *testing.T) {
	errBody := func(t *testing.T, raw stdjson.RawMessage) string {
		t.Helper()
		var resp event.ResponseError
		require.NoError(t, json.Unmarshal(raw, &resp))
		return resp.Message
	}

	t.Run("uninitialised app", func(t *testing.T) {
		var a *App
		resp := a.Call(AppMessage{MessageId: "1", Path: "/x"})
		require.Contains(t, errBody(t, resp.Body), "not ready")

		bare := NewApp()
		resp = bare.Call(AppMessage{MessageId: "1", Path: "/x"})
		require.Contains(t, errBody(t, bare.Call(AppMessage{MessageId: "1"}).Body), "not ready")
		require.Contains(t, errBody(t, resp.Body), "not ready")
	})

	t.Run("missing message id", func(t *testing.T) {
		a := liveApp(t, &stubAuthService{}, nil)
		resp := a.Call(AppMessage{Path: "/x", Body: []byte("{}")})
		require.Contains(t, errBody(t, resp.Body), "message id is empty")
	})

	t.Run("missing body", func(t *testing.T) {
		a := liveApp(t, &stubAuthService{}, nil)
		resp := a.Call(AppMessage{MessageId: "1", Path: "/x"})
		require.Contains(t, errBody(t, resp.Body), "message body is empty")
	})

	t.Run("login with a malformed payload", func(t *testing.T) {
		a := liveApp(t, &stubAuthService{}, nil)
		resp := a.Call(AppMessage{MessageId: "1", Path: event.PRIVATE_POST_LOGIN, Body: []byte("{")})
		require.NotEmpty(t, errBody(t, resp.Body))
	})

	t.Run("login failure is reported", func(t *testing.T) {
		a := liveApp(t, &stubAuthService{loginFn: func(event.LoginEvent, security.PSK) (event.LoginResponse, error) {
			return event.LoginResponse{}, errors.New("wrong password")
		}}, nil)
		resp := a.Call(AppMessage{
			MessageId: "1", Path: event.PRIVATE_POST_LOGIN,
			Body: mustJSON(t, event.LoginEvent{Username: "u", Password: "p"}),
		})
		require.Contains(t, errBody(t, resp.Body), "wrong password")
	})

	t.Run("login success returns the session", func(t *testing.T) {
		a := liveApp(t, &stubAuthService{loginFn: func(event.LoginEvent, security.PSK) (event.LoginResponse, error) {
			return event.LoginResponse{Token: "session-token"}, nil
		}}, nil)
		resp := a.Call(AppMessage{
			MessageId: "1", Path: event.PRIVATE_POST_LOGIN,
			Body: mustJSON(t, event.LoginEvent{Username: "u", Password: "p"}),
		})
		var out event.LoginResponse
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Equal(t, "session-token", out.Token)
		require.Equal(t, "1", resp.MessageId)
	})

	t.Run("logout stops the node", func(t *testing.T) {
		authSvc := &stubAuthService{}
		node := &stubNodeServer{}
		a := liveApp(t, authSvc, node)

		resp := a.Call(AppMessage{MessageId: "1", Path: event.PRIVATE_POST_LOGOUT, Body: []byte("{}")})
		require.JSONEq(t, `["logged_out"]`, string(resp.Body))
		require.True(t, node.stopped)
		require.True(t, authSvc.loggedOut)
		require.True(t, authSvc.reset, "the auth guard must be cleared for the next login")
		require.Nil(t, a.node, "a stopped node must not stay attached")
	})

	t.Run("routed call without an attached node", func(t *testing.T) {
		a := liveApp(t, &stubAuthService{}, nil)
		resp := a.Call(AppMessage{MessageId: "1", Path: "/private/get/info", Body: []byte("{}")})
		require.Contains(t, errBody(t, resp.Body), "not attached server node")
	})

	t.Run("routed call is signed and forwarded", func(t *testing.T) {
		key := testKey(t)
		var got event.Message
		node := &stubNodeServer{streamFn: func(path stream.WarpRoute, data any) ([]byte, error) {
			got = data.(event.Message)
			return []byte(`{"ok":true}`), nil
		}}
		a := liveApp(t, &stubAuthService{privateKey: key}, node)

		resp := a.Call(AppMessage{
			MessageId: "42", Path: "/private/get/info", NodeId: "caller-node",
			Body: []byte(`{"a":1}`), Version: "0.0.1", Timestamp: "2026-01-02T15:04:05Z",
		})
		require.JSONEq(t, `{"ok":true}`, string(resp.Body))
		require.Equal(t, node.info.ID.String(), resp.NodeId)

		require.Equal(t, "/private/get/info", got.Destination)
		require.Equal(t, "caller-node", got.NodeId)
		require.False(t, got.Timestamp.IsZero())
		require.NoError(t, security.VerifySignature(
			key.Public().(ed25519.PublicKey), got.SigningBytes(), got.Signature,
		))
	})

	t.Run("a missing timestamp is stamped now", func(t *testing.T) {
		var got event.Message
		node := &stubNodeServer{streamFn: func(_ stream.WarpRoute, data any) ([]byte, error) {
			got = data.(event.Message)
			return []byte(`{}`), nil
		}}
		a := liveApp(t, &stubAuthService{privateKey: testKey(t)}, node)

		a.Call(AppMessage{MessageId: "1", Path: "/private/get/info", Body: []byte("{}")})
		require.False(t, got.Timestamp.IsZero())
	})

	t.Run("stream failure is reported", func(t *testing.T) {
		node := &stubNodeServer{streamFn: func(stream.WarpRoute, any) ([]byte, error) {
			return nil, errors.New("stream closed")
		}}
		a := liveApp(t, &stubAuthService{privateKey: testKey(t)}, node)

		resp := a.Call(AppMessage{MessageId: "1", Path: "/private/get/info", Body: []byte("{}")})
		require.Contains(t, errBody(t, resp.Body), "stream closed")
	})

	t.Run("a pending release is reported and answered", func(t *testing.T) {
		updater := &stubNodeUpdater{answers: make(chan bool, 1)}
		a := liveApp(t, &stubAuthService{}, nil)
		a.updater = updater

		resp := a.Call(AppMessage{MessageId: "1", Path: event.PRIVATE_GET_UPDATE, Body: []byte("{}")})
		var info domain.UpdateInfo
		require.NoError(t, json.Unmarshal(resp.Body, &info))
		require.Empty(t, info.NewVersion, "nothing is waiting before a release is found")

		updater.pending = domain.UpdateInfo{CurrentVersion: "0.7.1", NewVersion: "0.7.2"}
		resp = a.Call(AppMessage{MessageId: "2", Path: event.PRIVATE_GET_UPDATE, Body: []byte("{}")})
		require.NoError(t, json.Unmarshal(resp.Body, &info))
		require.Equal(t, "0.7.2", info.NewVersion)
		require.Equal(t, "0.7.1", info.CurrentVersion)

		resp = a.Call(AppMessage{
			MessageId: "3", Path: event.PRIVATE_POST_UPDATE,
			Body: mustJSON(t, event.UpdateEvent{IsAllowed: true}),
		})
		require.JSONEq(t, `["update_answered"]`, string(resp.Body))
		require.True(t, <-updater.answers)
	})

	t.Run("update routes survive a disabled self-update", func(t *testing.T) {
		a := liveApp(t, &stubAuthService{}, nil)

		resp := a.Call(AppMessage{MessageId: "1", Path: event.PRIVATE_GET_UPDATE, Body: []byte("{}")})
		var info domain.UpdateInfo
		require.NoError(t, json.Unmarshal(resp.Body, &info))
		require.Empty(t, info.NewVersion)

		resp = a.Call(AppMessage{
			MessageId: "2", Path: event.PRIVATE_POST_UPDATE,
			Body: mustJSON(t, event.UpdateEvent{IsAllowed: true}),
		})
		require.JSONEq(t, `["update_answered"]`, string(resp.Body))
	})

	t.Run("a malformed update answer is reported", func(t *testing.T) {
		a := liveApp(t, &stubAuthService{}, nil)
		resp := a.Call(AppMessage{MessageId: "1", Path: event.PRIVATE_POST_UPDATE, Body: []byte("not json")})
		require.NotEmpty(t, errBody(t, resp.Body))
	})

	t.Run("an empty response body is reported", func(t *testing.T) {
		node := &stubNodeServer{streamFn: func(stream.WarpRoute, any) ([]byte, error) {
			return nil, nil
		}}
		a := liveApp(t, &stubAuthService{privateKey: testKey(t)}, node)

		resp := a.Call(AppMessage{MessageId: "1", Path: "/private/get/info", Body: []byte("{}")})
		require.Contains(t, errBody(t, resp.Body), "response body is empty")
	})
}

func TestAppClose(t *testing.T) {
	authSvc := &stubAuthService{}
	node := &stubNodeServer{}
	a := liveApp(t, authSvc, node)

	a.close(context.Background())
	require.True(t, node.stopped)
	require.True(t, authSvc.loggedOut)

	// a second close panics on the already-closed channel and is recovered
	require.NotPanics(t, func() { a.close(context.Background()) })
}

func TestServeLoginsStopsWithContext(t *testing.T) {
	a := liveApp(t, &stubAuthService{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	a.ctx = ctx
	cancel()

	// The auth handshake never lands, so serveLogins must return on the context
	// rather than dialling a node.
	done := make(chan struct{})
	go func() {
		a.serveLogins("testnet", nil)
		close(done)
	}()
	<-done
}

func TestSetLinuxDesktopIconSkipsSnap(t *testing.T) {
	t.Setenv("SNAP", "/snap/warpnet/current")
	require.NotPanics(t, func() { setLinuxDesktopIcon([]byte("not-really-a-png")) })
}

func mustJSON(t *testing.T, v any) stdjson.RawMessage {
	t.Helper()
	bt, err := json.Marshal(v)
	require.NoError(t, err)
	return bt
}

// fakeAuthStore stands in for the encrypted store behind AuthService:
// Authenticate opens it and Logout closes it, as the badger-backed repo does.
type fakeAuthStore struct {
	key   ed25519.PrivateKey
	owner domain.Owner
	open  bool
}

func (f *fakeAuthStore) Authenticate(_, _ string) error { f.open = true; return nil }
func (f *fakeAuthStore) SessionToken() string           { return "session-token" }
func (f *fakeAuthStore) GetOwner() domain.Owner         { return f.owner }
func (f *fakeAuthStore) PrivateKey() ed25519.PrivateKey { return f.key }
func (f *fakeAuthStore) Logout()                        { f.open = false }

func (f *fakeAuthStore) SetOwner(o domain.Owner) (domain.Owner, error) {
	f.owner = o
	return o, nil
}

type fakeUserStore struct{}

func (fakeUserStore) Create(u domain.User) (domain.User, error)           { return u, nil }
func (fakeUserStore) Update(_ string, u domain.User) (domain.User, error) { return u, nil }

func TestAppSignsInAgainAfterLogout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key := testKey(t)
	id, err := warpnet.IDFromPublicKey(key.Public().(ed25519.PublicKey))
	require.NoError(t, err)

	store := &fakeAuthStore{key: key}
	a := NewApp()
	a.ctx = ctx
	a.mx = new(sync.RWMutex)
	// unbuffered: the handshake uses one channel in both directions, so a
	// request parked in a buffer before serveLogins reaches its receive would
	// be read straight back by the login that sent it
	a.readyChan = make(chan domain.AuthNodeInfo)
	a.auth = auth.NewAuthService(ctx, store, fakeUserStore{}, a.readyChan)

	var raised []*stubNodeServer
	go a.serveLogins("testnet", func() (NodeServer, error) {
		node := &stubNodeServer{info: warpnet.NodeInfo{ID: id}}
		raised = append(raised, node)
		return node, nil
	})

	signIn := func() event.LoginResponse {
		t.Helper()
		resp := callWithin(t, a, AppMessage{
			MessageId: "login", Path: event.PRIVATE_POST_LOGIN,
			Body: mustJSON(t, event.LoginEvent{Username: "alice", Password: "Owner1234$"}),
		})
		var out event.LoginResponse
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		return out
	}

	require.Equal(t, id.String(), signIn().ID, "the first login raises a node")

	resp := callWithin(t, a, AppMessage{MessageId: "logout", Path: event.PRIVATE_POST_LOGOUT, Body: []byte("{}")})
	require.JSONEq(t, `["logged_out"]`, string(resp.Body))
	require.False(t, store.open, "logout closes the database")

	require.Equal(t, id.String(), signIn().ID, "signing in again must not need an app restart")
	require.True(t, store.open, "the second login reopens the database")
	require.Len(t, raised, 2, "the stopped node must be replaced, not reused")
	require.True(t, raised[0].stopped)
	require.False(t, raised[1].stopped)
}

func TestServeLoginsStopsWhenTheReadyChannelCloses(t *testing.T) {
	a := liveApp(t, &stubAuthService{}, nil)
	a.ctx = context.Background()

	done := make(chan struct{})
	go func() {
		a.serveLogins("testnet", nil)
		close(done)
	}()

	close(a.readyChan) // what App.close does on shutdown
	<-done
}

// callWithin fails instead of hanging: a login whose handshake goes unanswered
// blocks inside AuthLogin forever.
func callWithin(t *testing.T, a *App, msg AppMessage) AppMessage {
	t.Helper()

	done := make(chan AppMessage, 1)
	go func() { done <- a.Call(msg) }()

	select {
	case resp := <-done:
		return resp
	case <-time.After(10 * time.Second):
		t.Fatalf("app call %q hung", msg.Path)
		return AppMessage{}
	}
}

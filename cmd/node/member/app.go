package main

import (
	"context"
	"crypto/ed25519"
	stdjson "encoding/json"
	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/metrics"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/member/auth"
	member "github.com/Warp-net/warpnet/cmd/node/member/node"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	jsoniter "github.com/json-iterator/go"
	log "github.com/sirupsen/logrus"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type AppStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
	NewReadTxn() (local_store.WarpTransactioner, error)
	Get(key local_store.DatabaseKey) ([]byte, error)
	GetExpiration(key local_store.DatabaseKey) (uint64, error)
	GetSize(key local_store.DatabaseKey) (int64, error)
	Sync() error
	IsClosed() bool
	InnerDB() *local_store.WarpDB
	SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error
	Set(key local_store.DatabaseKey, value []byte) error
	Delete(key local_store.DatabaseKey) error
	Path() string
	Stats() map[string]string
	IsFirstRun() bool
	Close()
}

type AppAuthServicer interface {
	AuthLogin(message event.LoginEvent, psk security.PSK) (authInfo event.LoginResponse, err error)
	AuthLogout()
	PrivateKey() ed25519.PrivateKey
	Storage() auth.AuthPersistencyLayer
}

type NodeServer interface {
	SelfStream(path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
	Stop()
	Start() error
}

type App struct {
	ctx            context.Context
	auth           AppAuthServicer
	node           NodeServer
	db             AppStorer
	psk            security.PSK
	readyChan      chan domain.AuthNodeInfo
	mx             *sync.RWMutex
	appPath, dbDir string
	version        *semver.Version
	// deepLink: latest pending warpnet:// payload for the frontend. Guarded by mx.
	deepLink string
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

func (a *App) IsFirstRun(network string) bool {
	if a == nil || a.mx == nil {
		return false
	}
	if strings.TrimSpace(network) == "" {
		network = config.Config().Node.Network
	}
	a.mx.RLock()
	db := a.db
	a.mx.RUnlock()
	if db != nil {
		return db.IsFirstRun()
	}
	// Pre-login the DB is not open yet; read the network-scoped lock file directly.
	dbPath := filepath.Join(a.appPath, strings.TrimSpace(network), strings.TrimSpace(a.dbDir))
	return local_store.IsFirstRun(dbPath)
}

// SetPendingDeepLink stashes a warpnet:// payload for the frontend. Pre-startup safe (a.mx may be nil).
func (a *App) SetPendingDeepLink(raw string) {
	if a == nil {
		return
	}
	if a.mx == nil {
		a.deepLink = raw
		return
	}
	a.mx.Lock()
	a.deepLink = raw
	a.mx.Unlock()
}

// ConsumePendingDeepLink returns the pending warpnet:// URL and clears it.
func (a *App) ConsumePendingDeepLink() string {
	if a == nil || a.mx == nil {
		return ""
	}
	a.mx.Lock()
	defer a.mx.Unlock()
	raw := a.deepLink
	a.deepLink = ""
	return raw
}

// NotifyDeepLink stashes the URL and, if the Wails runtime is ready,
// unminimises + shows the window and emits "deeplink:open" so the
// frontend pulls ConsumePendingDeepLink without waiting for a navigation.
// Called from SingleInstanceLock.OnSecondInstanceLaunch (Linux/Windows)
// and mac.Options.OnUrlOpen — both arrive while the app is already up.
func (a *App) NotifyDeepLink(raw string) {
	if a == nil || raw == "" {
		return
	}
	a.SetPendingDeepLink(raw)
	if a.ctx == nil {
		return
	}
	wailsruntime.WindowUnminimise(a.ctx)
	wailsruntime.WindowShow(a.ctx)
	wailsruntime.EventsEmit(a.ctx, "deeplink:open")
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("app: startup panic: %v", r)
		}
	}()
	a.ctx = ctx
	a.mx = new(sync.RWMutex)
	a.version = config.Config().Version
	a.dbDir = config.Config().Database.Dir
	a.appPath = local_store.GetAppPath()
}

// setupAuth opens the network-scoped database and wires the auth service on the
// first login (the DB path and PSK depend on the login-supplied network). It is
// idempotent so a rejected login can be retried without reopening the DB, which
// would fail on badger's single-process directory lock.
func (a *App) setupAuth(network string) error {
	if strings.TrimSpace(network) == "" {
		network = config.Config().Node.Network
	}
	if a.auth != nil {
		return nil
	}
	psk, err := security.GeneratePSK(network, a.version)
	if err != nil {
		return err
	}
	dbPath := filepath.Join(a.appPath, strings.TrimSpace(network), strings.TrimSpace(a.dbDir))
	db, err := local_store.New(dbPath, local_store.DefaultOptions())
	if err != nil {
		return err
	}
	authRepo := database.NewAuthRepo(db, network)
	userRepo := database.NewUserRepo(db)
	a.mx.Lock()
	a.db = db
	a.mx.Unlock()
	a.psk = psk
	a.readyChan = make(chan domain.AuthNodeInfo, 1)
	a.auth = auth.NewAuthService(a.ctx, authRepo, userRepo, a.readyChan)
	go a.runNode(network)
	return nil
}

func (a *App) runNode(network string) {
	var (
		err                error
		serverNodeAuthInfo domain.AuthNodeInfo
	)

	// wait DB auth
	select {
	case <-a.ctx.Done():
		log.Infoln("interrupted...")
		return
	case serverNodeAuthInfo = <-a.readyChan:
		log.Infoln("database authentication passed")
	}

	ownNodeId, err := warpnet.IDFromPublicKey(a.auth.PrivateKey().Public().(ed25519.PublicKey))
	if err != nil {
		log.Fatalf("failed to get current node ID: %v", err)
	}

	m := metrics.NewMetricsClient(
		config.Config().Node.Metrics.Gateway,
		ownNodeId.String(),
		network,
	)

	infos, err := config.Config().Node.AddrInfos()
	if err != nil {
		log.Fatalf("failed to get bootstrap nodes infos: %v", err)
	}

	a.mx.Lock()
	a.node, err = member.NewMemberNode(
		a.ctx,
		a.auth.PrivateKey(),
		a.psk,
		ownNodeId,
		a.auth.Storage(),
		a.db,
		network,
		infos,
		m,
	)
	if err != nil {
		a.mx.Unlock()
		log.Errorf("failed to init node: %v \n", err)
		return
	}
	a.mx.Unlock()

	err = a.node.Start()
	if err != nil {
		log.Errorf("failed to start member node: %v \n", err)
		return
	}

	// report to auth handler - Node set up and running
	serverNodeAuthInfo.ID = ownNodeId.String()
	serverNodeAuthInfo.Network = network
	serverNodeAuthInfo.Addresses = a.node.NodeInfo().Addresses
	serverNodeAuthInfo.BootstrapPeers = config.Config().Node.Bootstrap
	a.readyChan <- serverNodeAuthInfo
}

type AppMessage struct {
	Body      stdjson.RawMessage `json:"body"`
	MessageId string             `json:"message_id"`
	NodeId    string             `json:"node_id"`
	Path      string             `json:"path"`
	Timestamp string             `json:"timestamp,omitempty"`
	Version   string             `json:"version"`
	Signature string             `json:"signature"`
}

// Call calls a JS/Go mapped method
func (a *App) Call(request AppMessage) (response AppMessage) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("app: call panic: %v", r)
		}
	}()
	// auth is created lazily on the first login (the DB path depends on the
	// login-supplied network), so the guard checks startup ran (mx set), not auth.
	if a == nil || a.mx == nil {
		log.Errorln("app not initialized")
		response.Body = newErrorResp("internal app not ready")
		return response
	}

	response.MessageId = request.MessageId
	response.Path = request.Path
	response.Timestamp = time.Now().String()
	response.Version = "0.0.0"

	if request.MessageId == "" {
		log.Errorln("message id is empty")
		response.Body = newErrorResp("message id is empty")
		return response
	}
	if request.Body == nil {
		log.Errorln("message body is empty")
		response.Body = newErrorResp("message body is empty")
		return response
	}

	switch request.Path {
	case event.PRIVATE_POST_LOGIN:
		var ev event.LoginEvent
		err := json.Unmarshal(request.Body, &ev)
		if err != nil {
			log.Errorf("message body as login event: %v %s \n", err, request.Body)
			response.Body = newErrorResp(err.Error())
			return response
		}

		if err = a.setupAuth(ev.Network); err != nil {
			log.Errorf("auth setup: %v \n", err)
			response.Body = newErrorResp(err.Error())
			return response
		}

		var loginResp event.LoginResponse
		loginResp, err = a.auth.AuthLogin(ev, a.psk)
		if err != nil {
			log.Errorf("auth: %v \n", err)
			response.Body = newErrorResp(err.Error())
			return response
		}

		bt, err := json.Marshal(loginResp)
		if err != nil {
			log.Errorf("login resp marshal: %v \n", err)
			response.Body = newErrorResp(err.Error())
			return response
		}
		response.Body = bt
	case event.PRIVATE_POST_LOGOUT:
		a.mx.RLock()
		n := a.node
		a.mx.RUnlock()
		if n != nil {
			n.Stop() // close node first
		}
		if a.auth != nil {
			a.auth.AuthLogout()
		}
		response.Body = []byte(`["logged_out"]`)
		return response
	default:
		a.mx.RLock()
		if a.node == nil {
			a.mx.RUnlock()
			log.Errorf("app: node is not attached, event: %s %s", request.Path, string(request.Body))
			response.Body = newErrorResp("not attached server node")
			return response
		}
		a.mx.RUnlock()

		if request.Path == "" {
			log.Errorln("message destination is empty")
			response.Body = newErrorResp("response destination is empty")
			return response
		}

		nodeId := a.node.NodeInfo().ID.String()
		response.NodeId = nodeId
		ts, _ := time.Parse(time.RFC3339, request.Timestamp)
		body := jsoniter.RawMessage(request.Body)
		signature := security.Sign(a.auth.PrivateKey(), body)

		respData, err := a.node.SelfStream(
			stream.WarpRoute(request.Path),
			event.Message{
				Body:        body,
				MessageId:   request.MessageId,
				NodeId:      request.NodeId,
				Destination: request.Path,
				Timestamp:   ts,
				Version:     request.Version,
				Signature:   signature,
			},
		)
		if err != nil {
			log.Errorf("send stream: %v \n", err)
			response.Body = newErrorResp(err.Error())
			return response
		}
		response.Body = respData
	}
	if response.Body == nil {
		log.Errorln("response body is empty")
		response.Body = newErrorResp("response body is empty")
		return response
	}
	return response
}

func newErrorResp(msg string) stdjson.RawMessage {
	errResp := event.ResponseError{
		Code:    http.StatusInternalServerError,
		Message: msg,
	}

	bt, _ := json.Marshal(errResp)
	return bt
}

func (a *App) close(_ context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("app: close panic: %v", r)
		}
	}()

	log.Infoln("app: closing...")

	a.mx.RLock()
	n := a.node
	a.mx.RUnlock()
	if n != nil {
		n.Stop() // close node first
	}

	if a.auth != nil {
		a.auth.AuthLogout()
	}

	if a.readyChan != nil {
		close(a.readyChan)
	}
}

// setLinuxDesktopIcon writes the PNG referenced by Icon=warpnet (the .desktop file is owned by deeplink.Register).
func setLinuxDesktopIcon(iconData []byte) {
	if runtime.GOOS != "linux" {
		return
	}
	if os.Getenv("SNAP") != "" { // snap package
		return
	}

	currentUser, err := user.Current()
	if err != nil {
		panic(err)
	}
	homeDir := currentUser.HomeDir

	iconDir := filepath.Join(homeDir, ".local", "share", "icons", "hicolor", "512x512", "apps")

	//#nosec
	_ = os.MkdirAll(iconDir, 0755)

	iconPath := filepath.Join(iconDir, "warpnet.png")
	//#nosec
	if err := os.WriteFile(iconPath, iconData, 0644); err != nil {
		log.Fatalf("setting icon: write icon file fail: %v", err)
	}
}

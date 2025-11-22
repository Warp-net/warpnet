package main

import (
	"context"
	"crypto/ed25519"
	stdjson "encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/cmd/node/member/auth"
	member "github.com/Warp-net/warpnet/cmd/node/member/node"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/database/local"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/json-iterator/go"
	log "github.com/sirupsen/logrus"
)

type AppStorer interface {
	NewTxn() (local.WarpTransactioner, error)
	Get(key local.DatabaseKey) ([]byte, error)
	GetExpiration(key local.DatabaseKey) (uint64, error)
	GetSize(key local.DatabaseKey) (int64, error)
	Sync() error
	IsClosed() bool
	InnerDB() *local.WarpDB
	SetWithTTL(key local.DatabaseKey, value []byte, ttl time.Duration) error
	Set(key local.DatabaseKey, value []byte) error
	Delete(key local.DatabaseKey) error
	Path() string
	Stats() map[string]string
	IsFirstRun() bool
	Close()
}

type AppAuthServicer interface {
	AuthLogin(message event.LoginEvent) (authInfo event.LoginResponse, err error)
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
	ctx         context.Context
	auth        AppAuthServicer
	node        NodeServer
	db          AppStorer
	codeHashHex string
	readyChan   chan domain.AuthNodeInfo
	mx          *sync.RWMutex
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
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

	codeHashHex, err := security.GetCodebaseHashHex(root.GetCodeBase())
	if err != nil {
		log.Errorf("failed to get codebase hash: %v \n", err)
		return
	}
	a.codeHashHex = codeHashHex

	db, err := local.New(config.Config().Database.Path, local.DefaultOptions())
	if err != nil {
		log.Errorf("failed to init db: %v \n", err)
		return
	}

	authRepo := database.NewAuthRepo(db)
	userRepo := database.NewUserRepo(db)
	a.db = db
	a.readyChan = make(chan domain.AuthNodeInfo, 1)
	a.auth = auth.NewAuthService(ctx, authRepo, userRepo, a.readyChan)

	version := config.Config().Version
	network := config.Config().Node.Network
	psk, err := security.GeneratePSK(network, version)
	if err != nil {
		log.Errorf("failed to generate PSK: %v", err)
		return
	}

	go a.runNode(psk)
}

func (a *App) runNode(psk security.PSK) {
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

	a.mx.Lock()
	a.node, err = member.NewMemberNode(
		a.ctx,
		a.auth.PrivateKey(),
		psk,
		a.codeHashHex,
		config.Config().Version,
		a.auth.Storage(),
		a.db,
	)
	if err != nil {
		a.mx.Unlock()
		log.Errorf("failed to init node: %v \n", err)
		return
	}
	a.mx.Unlock()

	if err != nil {
		log.Errorf("failed to init node: %v \n", err)
		return
	}

	err = a.node.Start()
	if err != nil {
		log.Errorf("failed to start member node: %v \n", err)
		return
	}

	// report to auth handler - Node set up and running
	serverNodeAuthInfo.Identity.Owner.NodeId = a.node.NodeInfo().ID.String()
	serverNodeAuthInfo.NodeInfo = a.node.NodeInfo()
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
	if a == nil || a.auth == nil {
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

		var loginResp event.LoginResponse
		loginResp, err = a.auth.AuthLogin(ev)
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
		a.close(a.ctx)
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

	a.node.Stop() // close node first

	a.db.Close() // db is a second

	close(a.readyChan)
}

const linuxDesktopTemplate = `
	[Desktop Entry]
	Name=warpnet
	Exec=%s
	Icon=warpnet
	Type=Application
	Categories=Network;Social;
`

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

	desktopDir := filepath.Join(homeDir, ".local", "share", "applications")
	iconDir := filepath.Join(homeDir, ".local", "share", "icons", "hicolor", "512x512", "apps")

	//#nosec
	_ = os.MkdirAll(desktopDir, 0755) //nolint:mnd
	//#nosec
	_ = os.MkdirAll(iconDir, 0755) //nolint:mnd

	execPath, err := os.Executable()
	if err != nil {
		log.Fatalf("setting icon: unable to determine executable path: %v", err)
	}

	desktopFile := filepath.Join(desktopDir, "warpnet.desktop")
	content := fmt.Sprintf(linuxDesktopTemplate, execPath)
	//#nosec
	if err := os.WriteFile(desktopFile, []byte(content), 0644); err != nil { //nolint:mnd
		log.Fatalf("setting icon: write .desktop file fail: %v", err)
	}

	iconPath := filepath.Join(iconDir, "warpnet.png")
	//#nosec
	if err := os.WriteFile(iconPath, iconData, 0644); err != nil { //nolint:mnd
		log.Fatalf("setting icon: write icon file fail: %v", err)
	}
}

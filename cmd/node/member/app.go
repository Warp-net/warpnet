package main

import (
	"context"
	"crypto/ed25519"
	stdjson "encoding/json"
	"fmt"
	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/auth"
	"github.com/Warp-net/warpnet/core/node/member"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/database/local"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"net/http"
	"os"
	"sync"
	"time"
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
	a.ctx = ctx
	a.mx = new(sync.RWMutex)

	codeHashHex, err := security.GetCodebaseHashHex(root.GetCodeBase())
	if err != nil {
		fmt.Printf("failed to get codebase hash: %v", err)
		os.Exit(1)
		return
	}
	a.codeHashHex = codeHashHex

	db, err := local.New(config.Config().Database.Path, false)
	if err != nil {
		fmt.Printf("failed to init db: %v", err)
		os.Exit(1)
		return
	}

	authRepo := database.NewAuthRepo(db)
	userRepo := database.NewUserRepo(db)
	a.db = db
	a.readyChan = make(chan domain.AuthNodeInfo, 1)
	a.auth = auth.NewAuthService(authRepo, userRepo, a.readyChan)

	version := config.Config().Version
	network := config.Config().Node.Network
	psk, err := security.GeneratePSK(network, version)
	if err != nil {
		fmt.Println("failed:", err)
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
		fmt.Println("interrupted...")
		return
	case serverNodeAuthInfo = <-a.readyChan:
		fmt.Println("database authentication passes")
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
		fmt.Printf("failed to init node: %v", err)
		return
	}
	a.mx.Unlock()

	if err != nil {
		fmt.Printf("failed to init node: %v", err)
		return
	}

	err = a.node.Start()
	if err != nil {
		fmt.Printf("failed to start member node: %v", err)
		return
	}

	// report to auth handler - Node set up and running
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
			fmt.Printf("method Call crashed: %v", r)
		}
	}()
	if a == nil || a.auth == nil {
		fmt.Printf("app not initialized")
		response.Body = newErrorResp("internal app not ready")
		return response
	}

	response.MessageId = request.MessageId
	response.Path = request.Path
	response.Timestamp = time.Now().String()
	response.Version = "0.0.0"

	if request.MessageId == "" {
		fmt.Printf("message id is empty")
		response.Body = newErrorResp("message id is empty")
		return response
	}
	if request.Body == nil {
		fmt.Printf("message body is empty")
		response.Body = newErrorResp("message body is empty")
		return response

	}

	switch request.Path {
	case event.PRIVATE_POST_LOGIN:
		var ev event.LoginEvent
		err := json.Unmarshal(request.Body, &ev)
		if err != nil {
			fmt.Printf("message body as login event: %v %s", err, request.Body)
			response.Body = newErrorResp(err.Error())
			return response
		}

		var loginResp event.LoginResponse
		loginResp, err = a.auth.AuthLogin(ev)
		if err != nil {
			fmt.Printf("auth: %v", err)
			response.Body = newErrorResp(err.Error())
			return response
		}

		bt, err := json.Marshal(loginResp)
		if err != nil {
			fmt.Printf("login resp marshal: %v", err)
			response.Body = newErrorResp(err.Error())
			return response
		}
		response.Body = bt
	case event.PRIVATE_POST_LOGOUT:
		a.close()
		response.Body = []byte(`["logged_out"]`)
		return response
	default:
		a.mx.RLock()
		if a.node == nil {
			a.mx.RUnlock()
			err := fmt.Errorf("not attached to server node")
			fmt.Println(err)
			response.Body = newErrorResp(err.Error())
			return response
		}
		a.mx.RUnlock()

		if request.Path == "" {
			fmt.Printf("message destination is empty")
			response.Body = newErrorResp("response destination is empty")
			return response
		}

		nodeId := a.node.NodeInfo().ID.String()
		response.NodeId = nodeId

		respData, err := a.node.SelfStream(stream.WarpRoute(request.Path), request.Body)
		if err != nil {
			fmt.Printf("send stream: %v", err)
			response.Body = newErrorResp(err.Error())
			return response
		}
		response.Body = respData
	}
	if response.Body == nil {
		fmt.Printf("response body is empty")
		response.Body = newErrorResp("response body is empty")
		return response
	}
	return response
}

func newErrorResp(msg string) stdjson.RawMessage {
	errResp := event.ErrorResponse{
		Code:    http.StatusInternalServerError,
		Message: msg,
	}

	bt, _ := json.Marshal(errResp)
	return bt
}

func (a *App) close() {
	defer func() { recover() }()

	a.db.Close()
	a.node.Stop()

	close(a.readyChan)
}

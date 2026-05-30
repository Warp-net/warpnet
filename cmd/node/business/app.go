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

package main

import (
	"context"
	"crypto/ed25519"
	stdjson "encoding/json"
	"io/fs"
	"net/http"
	"sync"
	"time"

	root "github.com/Warp-net/warpnet"
	business "github.com/Warp-net/warpnet/cmd/node/business/node"
	"github.com/Warp-net/warpnet/cmd/node/member/auth"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/metrics"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
)

// App is the headless analogue of the member node's Wails App: it owns the
// database, the auth service, and the business node, and exposes the same
// SelfStream-backed Call surface — only the transport is an HTTP/WS server
// (see server.go) instead of the Wails runtime.
type App struct {
	ctx         context.Context
	auth        *auth.AuthService
	node        *business.BusinessNode
	db          *local_store.DB
	codeHashHex string
	psk         security.PSK
	readyChan   chan domain.AuthNodeInfo
	mx          *sync.RWMutex

	sessions *sessionStore
}

func NewApp() *App {
	return &App{sessions: newSessionStore()}
}

func (a *App) IsFirstRun() bool {
	if a == nil || a.mx == nil {
		return false
	}
	a.mx.RLock()
	defer a.mx.RUnlock()
	if a.db == nil {
		return false
	}
	return a.db.IsFirstRun()
}

// Startup initialises the database, auth service and PSK, then kicks off
// runNode which waits for an interactive login before building the node.
func (a *App) Startup(ctx context.Context) error {
	a.ctx = ctx
	a.mx = new(sync.RWMutex)

	version := config.Config().Version
	network := config.Config().Node.Network

	codeHashHex, err := security.GetCodebaseHashHex(root.GetCodeBase())
	if err != nil {
		return err
	}
	a.codeHashHex = codeHashHex

	db, err := local_store.New(config.Config().Database.Path, local_store.DefaultOptions())
	if err != nil {
		return err
	}

	authRepo := database.NewAuthRepo(db, network)
	userRepo := database.NewUserRepo(db)
	a.db = db
	a.readyChan = make(chan domain.AuthNodeInfo, 1)
	a.auth = auth.NewAuthService(ctx, authRepo, userRepo, a.readyChan)

	psk, err := security.GeneratePSK(network, version)
	if err != nil {
		return err
	}
	a.psk = psk

	go a.runNode(network, psk)
	return nil
}

func (a *App) runNode(network string, psk security.PSK) {
	var serverNodeAuthInfo domain.AuthNodeInfo

	select {
	case <-a.ctx.Done():
		log.Infoln("business: interrupted before login...")
		return
	case serverNodeAuthInfo = <-a.readyChan:
		log.Infoln("business: database authentication passed")
	}

	ownNodeId, err := warpnet.IDFromPublicKey(a.auth.PrivateKey().Public().(ed25519.PublicKey))
	if err != nil {
		log.Fatalf("business: failed to get current node ID: %v", err)
	}

	m := metrics.NewMetricsClient(
		config.Config().Node.Metrics.Gateway,
		ownNodeId.String(),
		network,
	)

	infos, err := config.Config().Node.AddrInfos()
	if err != nil {
		log.Fatalf("business: failed to get bootstrap nodes infos: %v", err)
	}

	a.mx.Lock()
	a.node, err = business.NewBusinessNode(
		a.ctx,
		a.auth.PrivateKey(),
		psk,
		ownNodeId,
		a.codeHashHex,
		config.Config().Version,
		a.auth.Storage(),
		a.db,
		infos,
		m,
	)
	if err != nil {
		a.mx.Unlock()
		log.Errorf("business: failed to init node: %v", err)
		return
	}
	a.mx.Unlock()

	if err = a.node.Start(); err != nil {
		log.Errorf("business: failed to start node: %v", err)
		return
	}

	// Obligations: public IP (panic if behind NAT once AutoNAT settles) and
	// the moderator engine + report subscription. Relay is already on — every
	// WarpNode runs the circuit-relay service.
	go a.node.AssertPublicReachability()
	if err := a.node.StartModerator(a.ctx); err != nil {
		log.Errorf("business: failed to start moderator: %v", err)
	}

	serverNodeAuthInfo.ID = ownNodeId.String()
	serverNodeAuthInfo.Network = network
	serverNodeAuthInfo.Addresses = a.node.NodeInfo().Addresses
	serverNodeAuthInfo.BootstrapPeers = config.Config().Node.Bootstrap
	a.readyChan <- serverNodeAuthInfo
}

// AppMessage is the Wails JSON envelope the frontend speaks, unchanged so the
// existing Vue client works against the business node without a DTO change.
type AppMessage struct {
	Body      stdjson.RawMessage `json:"body"`
	MessageId string             `json:"message_id"`
	NodeId    string             `json:"node_id"`
	Path      string             `json:"path"`
	Timestamp string             `json:"timestamp,omitempty"`
	Version   string             `json:"version"`
	Signature string             `json:"signature"`
}

// Call mirrors the member App.Call: login and logout are handled inline, every
// other path is signed and fed to the node's SelfStream — the same middleware
// and handler stack the Wails client drives. password, when non-empty, is the
// freshly supplied login password (used to derive the WS session key).
func (a *App) Call(request AppMessage) (response AppMessage, password string) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("business: call panic: %v", r)
		}
	}()
	response.MessageId = request.MessageId
	response.Path = request.Path
	response.Timestamp = time.Now().String()
	response.Version = "0.0.0"

	if a == nil || a.auth == nil {
		response.Body = newErrorResp("internal app not ready")
		return response, ""
	}
	if request.MessageId == "" {
		response.Body = newErrorResp("message id is empty")
		return response, ""
	}
	if request.Body == nil {
		response.Body = newErrorResp("message body is empty")
		return response, ""
	}

	switch request.Path {
	case event.PRIVATE_POST_LOGIN:
		var ev event.LoginEvent
		if err := json.Unmarshal(request.Body, &ev); err != nil {
			response.Body = newErrorResp(err.Error())
			return response, ""
		}
		loginResp, err := a.auth.AuthLogin(ev, a.psk)
		if err != nil {
			log.Errorf("business: auth: %v", err)
			response.Body = newErrorResp(err.Error())
			return response, ""
		}
		bt, err := json.Marshal(loginResp)
		if err != nil {
			response.Body = newErrorResp(err.Error())
			return response, ""
		}
		response.Body = bt
		return response, ev.Password
	case event.PRIVATE_POST_LOGOUT:
		a.mx.RLock()
		n := a.node
		a.mx.RUnlock()
		if n != nil {
			n.Stop()
		}
		a.auth.AuthLogout()
		response.Body = []byte(`["logged_out"]`)
		return response, ""
	default:
		a.mx.RLock()
		n := a.node
		a.mx.RUnlock()
		if n == nil {
			response.Body = newErrorResp("not attached server node")
			return response, ""
		}
		if request.Path == "" {
			response.Body = newErrorResp("response destination is empty")
			return response, ""
		}

		response.NodeId = n.NodeInfo().ID.String()
		ts, _ := time.Parse(time.RFC3339, request.Timestamp)
		body := json.RawMessage(request.Body)
		signature := security.Sign(a.auth.PrivateKey(), body)

		respData, err := n.SelfStream(
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
			response.Body = newErrorResp(err.Error())
			return response, ""
		}
		response.Body = respData
	}

	if response.Body == nil {
		response.Body = newErrorResp("response body is empty")
	}
	return response, ""
}

func newErrorResp(msg string) stdjson.RawMessage {
	bt, _ := json.Marshal(event.ResponseError{
		Code:    http.StatusInternalServerError,
		Message: msg,
	})
	return bt
}

func (a *App) Close() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("business: close panic: %v", r)
		}
	}()
	log.Infoln("business: closing...")
	a.mx.RLock()
	n := a.node
	a.mx.RUnlock()
	if n != nil {
		n.Stop()
	}
	if a.auth != nil {
		a.auth.AuthLogout()
	}
	if a.db != nil {
		a.db.Close()
	}
}

// staticHandler serves the embedded Vue build (frontend/dist) with an SPA
// fallback to index.html so client-side routes resolve.
func staticHandler() (http.Handler, error) {
	sub, err := fs.Sub(root.GetStaticEmbedded(), "frontend/dist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(sub, trimLeadingSlash(r.URL.Path)); err != nil && r.URL.Path != "/" {
			r2 := new(http.Request)
			*r2 = *r
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}

func trimLeadingSlash(p string) string {
	if len(p) > 0 && p[0] == '/' {
		p = p[1:]
	}
	if p == "" {
		return "."
	}
	return p
}

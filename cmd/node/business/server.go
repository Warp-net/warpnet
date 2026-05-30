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
	"errors"
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

// Server is the business node's process: it owns the database, the auth
// service and the node, and serves the dashboard over HTTP/WS. There is no
// Wails App here — the node's identity is still derived from an interactive
// login (the auth service's readyChan handshake), so the node is built by
// runNode once the first login arrives.
type Server struct {
	ctx         context.Context
	auth        *auth.AuthService
	node        *business.BusinessNode
	db          *local_store.DB
	codeHashHex string
	psk         security.PSK
	readyChan   chan domain.AuthNodeInfo
	mx          *sync.RWMutex
}

func NewServer(ctx context.Context) (*Server, error) {
	network := config.Config().Node.Network
	version := config.Config().Version

	codeHashHex, err := security.GetCodebaseHashHex(root.GetCodeBase())
	if err != nil {
		return nil, err
	}

	db, err := local_store.New(config.Config().Database.Path, local_store.DefaultOptions())
	if err != nil {
		return nil, err
	}

	psk, err := security.GeneratePSK(network, version)
	if err != nil {
		db.Close()
		return nil, err
	}

	readyChan := make(chan domain.AuthNodeInfo, 1)
	authRepo := database.NewAuthRepo(db, network)
	userRepo := database.NewUserRepo(db)

	return &Server{
		ctx:         ctx,
		auth:        auth.NewAuthService(ctx, authRepo, userRepo, readyChan),
		db:          db,
		codeHashHex: codeHashHex,
		psk:         psk,
		readyChan:   readyChan,
		mx:          new(sync.RWMutex),
	}, nil
}

// Run starts the node-build goroutine and serves the dashboard, blocking until
// ctx is cancelled.
func (s *Server) Run(addr string) error {
	go s.runNode()

	staticH, err := staticHandler()
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/ws", s.handleWS)
	mux.Handle("/", staticH)

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-s.ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Infof("business: dashboard listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// runNode waits for the first successful login (the auth readyChan handshake),
// builds and starts the node, and wires the business obligations: a public-IP
// assertion driven from the outside through the node's public API, and the
// moderator engine. Relay is already on — every WarpNode runs circuit-relay.
func (s *Server) runNode() {
	var info domain.AuthNodeInfo
	select {
	case <-s.ctx.Done():
		log.Infoln("business: interrupted before login...")
		return
	case info = <-s.readyChan:
		log.Infoln("business: database authentication passed")
	}

	network := config.Config().Node.Network
	ownNodeId, err := warpnet.IDFromPublicKey(s.auth.PrivateKey().Public().(ed25519.PublicKey))
	if err != nil {
		log.Errorf("business: node ID: %v", err)
		return
	}

	infos, err := config.Config().Node.AddrInfos()
	if err != nil {
		log.Errorf("business: bootstrap infos: %v", err)
		return
	}

	m := metrics.NewMetricsClient(config.Config().Node.Metrics.Gateway, ownNodeId.String(), network)

	node, err := business.NewBusinessNode(
		s.ctx, s.auth.PrivateKey(), s.psk, ownNodeId, s.codeHashHex,
		config.Config().Version, s.auth.Storage(), s.db, infos, m,
	)
	if err != nil {
		log.Errorf("business: init node: %v", err)
		return
	}

	s.mx.Lock()
	s.node = node
	s.mx.Unlock()

	if err := node.Start(); err != nil {
		log.Errorf("business: start node: %v", err)
		return
	}

	// Stamp the role onto the owner's own user record so PUBLIC_GET_USER (and
	// the owner's own profile) reports it. Other nodes learn the role through
	// discovery; this commits before the auth handshake completes its final
	// user update, which preserves any non-empty role.
	if err := s.markOwnUserBusiness(info.UserId); err != nil {
		log.Warnf("business: mark own user role: %v", err)
	}

	go assertPublicReachability(s.ctx, node)
	if err := node.StartModerator(s.ctx); err != nil {
		log.Errorf("business: start moderator: %v", err)
	}

	info.ID = ownNodeId.String()
	info.Network = network
	info.Addresses = node.NodeInfo().Addresses
	info.BootstrapPeers = config.Config().Node.Bootstrap
	s.readyChan <- info
}

// AppMessage is the Wails JSON envelope the frontend speaks, kept identical so
// the existing Vue client works against the business node without a DTO change.
type AppMessage struct {
	Body      stdjson.RawMessage `json:"body"`
	MessageId string             `json:"message_id"`
	NodeId    string             `json:"node_id"`
	Path      string             `json:"path"`
	Timestamp string             `json:"timestamp,omitempty"`
	Version   string             `json:"version"`
	Signature string             `json:"signature"`
}

// dispatch handles one request: login and logout inline, every other path
// signed and fed to the node's SelfStream (the same middleware and handler
// stack the desktop client drives). password is non-empty only on a successful
// login, and is used to derive the WS session key.
func (s *Server) dispatch(req AppMessage) (resp AppMessage, password string) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("business: dispatch panic: %v", r)
		}
	}()
	resp.MessageId = req.MessageId
	resp.Path = req.Path
	resp.Timestamp = time.Now().String()
	resp.Version = "0.0.0"

	if req.MessageId == "" {
		resp.Body = newErrorResp("message id is empty")
		return resp, ""
	}
	if req.Body == nil {
		resp.Body = newErrorResp("message body is empty")
		return resp, ""
	}

	switch req.Path {
	case event.PRIVATE_POST_LOGIN:
		var ev event.LoginEvent
		if err := json.Unmarshal(req.Body, &ev); err != nil {
			resp.Body = newErrorResp(err.Error())
			return resp, ""
		}
		loginResp, err := s.auth.AuthLogin(ev, s.psk)
		if err != nil {
			log.Errorf("business: auth: %v", err)
			resp.Body = newErrorResp(err.Error())
			return resp, ""
		}
		bt, err := json.Marshal(loginResp)
		if err != nil {
			resp.Body = newErrorResp(err.Error())
			return resp, ""
		}
		resp.Body = bt
		return resp, ev.Password
	case event.PRIVATE_POST_LOGOUT:
		s.mx.RLock()
		n := s.node
		s.mx.RUnlock()
		if n != nil {
			n.Stop()
		}
		s.auth.AuthLogout()
		resp.Body = []byte(`["logged_out"]`)
		return resp, ""
	default:
		s.mx.RLock()
		n := s.node
		s.mx.RUnlock()
		if n == nil {
			resp.Body = newErrorResp("not attached server node")
			return resp, ""
		}

		resp.NodeId = n.NodeInfo().ID.String()
		ts, _ := time.Parse(time.RFC3339, req.Timestamp)
		body := json.RawMessage(req.Body)
		respData, err := n.SelfStream(
			stream.WarpRoute(req.Path),
			event.Message{
				Body:        body,
				MessageId:   req.MessageId,
				NodeId:      req.NodeId,
				Destination: req.Path,
				Timestamp:   ts,
				Version:     req.Version,
				Signature:   security.Sign(s.auth.PrivateKey(), body),
			},
		)
		if err != nil {
			resp.Body = newErrorResp(err.Error())
			return resp, ""
		}
		resp.Body = respData
	}

	if resp.Body == nil {
		resp.Body = newErrorResp("response body is empty")
	}
	return resp, ""
}

func (s *Server) isFirstRun() bool {
	s.mx.RLock()
	defer s.mx.RUnlock()
	if s.db == nil {
		return false
	}
	return s.db.IsFirstRun()
}

func (s *Server) Close() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("business: close panic: %v", r)
		}
	}()
	log.Infoln("business: closing...")
	s.mx.RLock()
	n := s.node
	s.mx.RUnlock()
	if n != nil {
		n.Stop()
	}
	if s.auth != nil {
		s.auth.AuthLogout()
	}
	if s.db != nil {
		s.db.Close()
	}
}

// markOwnUserBusiness stamps Role="business" on the owner's own user record so
// PUBLIC_GET_USER reports the role for the owner's own profile too. Every other
// node learns the role through discovery; this covers the self / cold-fetch
// case. It runs before the auth handshake's final user update, and Update only
// ever sets a non-empty role, so the stamp is not clobbered.
func (s *Server) markOwnUserBusiness(ownerId string) error {
	if ownerId == "" {
		return nil
	}
	userRepo := database.NewUserRepo(s.db)
	u, err := userRepo.Get(ownerId)
	if err != nil {
		return err
	}
	if u.Role == warpnet.BusinessRole {
		return nil
	}
	u.Role = warpnet.BusinessRole
	_, err = userRepo.Update(u.Id, u)
	return err
}

func newErrorResp(msg string) stdjson.RawMessage {
	bt, _ := json.Marshal(event.ResponseError{Code: http.StatusInternalServerError, Message: msg})
	return bt
}

// reachabilityProbe is the node surface the public-IP assertion reads. The
// check runs from the outside, against the node's exported API, rather than
// from any internal node state.
type reachabilityProbe interface {
	NodeInfo() warpnet.NodeInfo
	PublicAddrs() []warpnet.WarpAddress
}

// assertPublicReachability enforces the business node's public-IP obligation
// from the outside: it reads the node's AutoNAT verdict (NodeInfo().Reachability)
// and public addresses through the node's public methods. It waits out a grace
// window (AutoNAT v2 reports Unknown/Private transiently at boot), returns as
// soon as the node looks public, and panics — crashing the process, which is
// the assertion — only after several consecutive private readings.
func assertPublicReachability(ctx context.Context, node reachabilityProbe) {
	const (
		grace         = 90 * time.Second
		sampleEvery   = 5 * time.Second
		privateStreak = 3
		maxWait       = 5 * time.Minute
	)

	select {
	case <-ctx.Done():
		return
	case <-time.After(grace):
	}

	deadline := time.Now().Add(maxWait)
	ticker := time.NewTicker(sampleEvery)
	defer ticker.Stop()

	streak := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			switch node.NodeInfo().Reachability {
			case warpnet.ReachabilityPublic:
				log.Infoln("business: reachability confirmed public")
				return
			case warpnet.ReachabilityPrivate:
				streak++
				log.Warnf("business: reachability reported private (%d/%d)", streak, privateStreak)
				if streak >= privateStreak && len(node.PublicAddrs()) == 0 {
					panic("business: node is privately reachable (behind NAT) — a business node must have a publicly addressable IP")
				}
			default:
				streak = 0
			}
			if time.Now().After(deadline) {
				log.Warnln("business: reachability still unknown after max wait; continuing without public confirmation")
				return
			}
		}
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
		if _, statErr := fs.Stat(sub, trimLeadingSlash(r.URL.Path)); statErr != nil && r.URL.Path != "/" {
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

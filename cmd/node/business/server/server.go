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

// Package server runs the business dashboard: it owns the node lifecycle and
// the HTTP/WS surface. It receives its dependencies ready-made from main (the
// db, auth service, psk, ...) — it does not build them — and exposes Dispatch
// to the handlers, which are the only path-routing point.
package server

import (
	"context"
	"crypto/ed25519"
	stdjson "encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	node "github.com/Warp-net/warpnet/cmd/node/business/node"
	"github.com/Warp-net/warpnet/cmd/node/business/server/handlers"
	"github.com/Warp-net/warpnet/cmd/node/member/auth"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	localstore "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/metrics"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
)

// Deps are the ready-made dependencies main injects. The server builds nothing
// itself except the node, which can only be built once the owner logs in.
type Deps struct {
	DB             *localstore.DB
	Auth           *auth.AuthService
	PSK            security.PSK
	CodeHashHex    string
	Bootstrap      []warpnet.WarpAddrInfo
	Network        string
	Version        *semver.Version
	MetricsGateway string
	ReadyChan      chan domain.AuthNodeInfo
	WSKey          []byte // sha256(launch password); empty => plaintext channel
}

type Server struct {
	ctx context.Context
	d   Deps

	mx   *sync.RWMutex
	node *node.BusinessNode
}

func New(ctx context.Context, d Deps) *Server {
	s := &Server{ctx: ctx, d: d, mx: new(sync.RWMutex)}
	go s.runNode()
	return s
}

// Run serves the dashboard, blocking until the context is cancelled.
func (s *Server) Run(addr string) error {
	staticH, err := handlers.Static()
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/api/ws", handlers.WS(s, s.d.WSKey))
	mux.HandleFunc("/healthz", handlers.Healthz())
	mux.HandleFunc("/readyz", handlers.Readyz(s))
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
// builds and starts the node, and wires the business obligations: the role on
// the owner's record, the public-IP tracker, and the moderator engine. Relay is
// already on — every WarpNode runs circuit-relay.
func (s *Server) runNode() {
	var info domain.AuthNodeInfo
	select {
	case <-s.ctx.Done():
		log.Infoln("business: interrupted before login...")
		return
	case info = <-s.d.ReadyChan:
		log.Infoln("business: database authentication passed")
	}

	ownNodeId, err := warpnet.IDFromPublicKey(s.d.Auth.PrivateKey().Public().(ed25519.PublicKey))
	if err != nil {
		log.Errorf("business: node ID: %v", err)
		return
	}

	m := metrics.NewMetricsClient(s.d.MetricsGateway, ownNodeId.String(), s.d.Network)

	bn, err := node.NewBusinessNode(
		s.ctx, s.d.Auth.PrivateKey(), s.d.PSK, ownNodeId, s.d.CodeHashHex,
		s.d.Version, s.d.Auth.Storage(), s.d.DB, s.d.Bootstrap, m,
	)
	if err != nil {
		log.Errorf("business: init node: %v", err)
		return
	}

	s.mx.Lock()
	s.node = bn
	s.mx.Unlock()

	if err := bn.Start(); err != nil {
		log.Errorf("business: start node: %v", err)
		return
	}

	// Stamp the role onto the owner's record so it travels to peers via
	// PUBLIC_GET_USER + discovery. Any node could do the same with its own role.
	if err := database.NewUserRepo(s.d.DB).SetRole(info.UserId, warpnet.BusinessRole); err != nil {
		log.Warnf("business: set owner role: %v", err)
	}

	go trackPublicReachability(s.ctx, bn)
	if err := bn.StartModerator(s.ctx); err != nil {
		log.Errorf("business: start moderator: %v", err)
	}

	info.ID = ownNodeId.String()
	info.Network = s.d.Network
	info.Addresses = bn.NodeInfo().Addresses
	s.d.ReadyChan <- info
}

// Dispatch is the single routing point: is-first-run and login/logout inline,
// every other path signed and run through the node's SelfStream — the same
// middleware and handler stack the desktop client drives.
func (s *Server) Dispatch(req handlers.AppMessage) (resp handlers.AppMessage) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("business: dispatch panic: %v", r)
		}
	}()
	resp.MessageId = req.MessageId
	resp.Path = req.Path
	resp.Timestamp = time.Now().String()
	resp.Version = "0.0.0"

	switch req.Path {
	case handlers.PathIsFirstRun:
		body, _ := json.Marshal(s.isFirstRun())
		resp.Body = body
	case event.PRIVATE_POST_LOGIN:
		resp.Body = s.login(req.Body)
	case event.PRIVATE_POST_LOGOUT:
		s.withNode(func(n *node.BusinessNode) { n.Stop() })
		s.d.Auth.AuthLogout()
		resp.Body = []byte(`["logged_out"]`)
	default:
		resp.Body = s.nodeCall(req)
	}

	if resp.Body == nil {
		resp.Body = newErrorResp("response body is empty")
	}
	return resp
}

func (s *Server) login(body []byte) stdjson.RawMessage {
	var ev event.LoginEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return newErrorResp(err.Error())
	}
	loginResp, err := s.d.Auth.AuthLogin(ev, s.d.PSK)
	if err != nil {
		log.Errorf("business: auth: %v", err)
		return newErrorResp(err.Error())
	}
	bt, err := json.Marshal(loginResp)
	if err != nil {
		return newErrorResp(err.Error())
	}
	return bt
}

func (s *Server) nodeCall(req handlers.AppMessage) stdjson.RawMessage {
	s.mx.RLock()
	n := s.node
	s.mx.RUnlock()
	if n == nil {
		return newErrorResp("not attached server node")
	}

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
			Signature:   security.Sign(s.d.Auth.PrivateKey(), body),
		},
	)
	if err != nil {
		return newErrorResp(err.Error())
	}
	return respData
}

// NodeReady reports whether the node has been built and started (post-login).
func (s *Server) NodeReady() bool {
	s.mx.RLock()
	defer s.mx.RUnlock()
	return s.node != nil
}

func (s *Server) isFirstRun() bool {
	return s.d.DB != nil && s.d.DB.IsFirstRun()
}

func (s *Server) withNode(f func(n *node.BusinessNode)) {
	s.mx.RLock()
	n := s.node
	s.mx.RUnlock()
	if n != nil {
		f(n)
	}
}

func (s *Server) Close() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("business: close panic: %v", r)
		}
	}()
	log.Infoln("business: closing...")
	s.withNode(func(n *node.BusinessNode) { n.Stop() })
	if s.d.Auth != nil {
		s.d.Auth.AuthLogout()
	}
	if s.d.DB != nil {
		s.d.DB.Close()
	}
}

func newErrorResp(msg string) stdjson.RawMessage {
	bt, _ := json.Marshal(event.ResponseError{Code: http.StatusInternalServerError, Message: msg})
	return bt
}

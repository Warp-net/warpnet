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

// Package server is the business dashboard: an HTTP/WS front end that serves the
// embedded SPA and bridges dashboard requests to the node. It does NOT own the
// node — main builds and starts the node and attaches it here through the Node
// interface; the server only dispatches to it.
package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/business/server/handlers"
	"github.com/Warp-net/warpnet/cmd/node/member/auth"
	"github.com/Warp-net/warpnet/core/stream"
	localstore "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
)

// pathIsFirstRun is the control path the frontend sends (over the WS) before
// login to choose between the login and sign-up screens.
const pathIsFirstRun = "is-first-run"

// Node is all the dashboard needs from the running node: feed it a request.
type Node interface {
	SelfStream(path stream.WarpRoute, data any) ([]byte, error)
}

type Server struct {
	auth    *auth.AuthService
	psk     security.PSK
	wsKey   []byte
	db      *localstore.DB
	httpSrv *http.Server

	mx   *sync.RWMutex
	node Node
}

func New(authSvc *auth.AuthService, psk security.PSK, wsKey []byte, db *localstore.DB) (*Server, error) {
	s := &Server{auth: authSvc, psk: psk, wsKey: wsKey, db: db, mx: new(sync.RWMutex)}

	staticH, err := handlers.Static()
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/ws", handlers.WS(s, aesCodec{key: wsKey}))
	mux.HandleFunc("/healthz", handlers.Healthz())
	mux.HandleFunc("/readyz", handlers.Readyz(s))
	mux.Handle("/", staticH)
	s.httpSrv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	return s, nil
}

// Attach hands the started node to the dashboard. main calls it once the node
// is up; until then node calls report "not ready".
func (s *Server) Attach(n Node) {
	s.mx.Lock()
	s.node = n
	s.mx.Unlock()
}

// Run serves until Shutdown (or a listen error).
func (s *Server) Run(addr string) error {
	s.httpSrv.Addr = addr
	log.Infof("business: dashboard listening on %s", addr)
	return s.httpSrv.ListenAndServe()
}

func (s *Server) Shutdown() error {
	return s.httpSrv.Shutdown(context.Background())
}

// Dispatch is the single routing point: is-first-run and login/logout inline,
// every other path signed and run through the node's SelfStream — the same
// middleware and handler stack the desktop client drives.
func (s *Server) Dispatch(req event.Message) event.Message {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("business: dispatch panic: %v", r)
		}
	}()
	resp := event.Message{MessageId: req.MessageId, Destination: req.Destination, Timestamp: time.Now(), Version: "0.0.0"}

	switch req.Destination {
	case pathIsFirstRun:
		body, _ := json.Marshal(s.db.IsFirstRun())
		resp.Body = body
	case event.PRIVATE_POST_LOGIN:
		resp.Body = s.login(req.Body)
	case event.PRIVATE_POST_LOGOUT:
		s.auth.AuthLogout()
		resp.Body = json.RawMessage(`["logged_out"]`)
	default:
		resp.Body = s.nodeCall(req)
	}

	if resp.Body == nil {
		resp.Body = newErrorResp("response body is empty")
	}
	return resp
}

func (s *Server) login(body json.RawMessage) json.RawMessage {
	var ev event.LoginEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return newErrorResp(err.Error())
	}
	loginResp, err := s.auth.AuthLogin(ev, s.psk)
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

func (s *Server) nodeCall(req event.Message) json.RawMessage {
	s.mx.RLock()
	n := s.node
	s.mx.RUnlock()
	if n == nil {
		return newErrorResp("not attached server node")
	}

	req.Signature = security.Sign(s.auth.PrivateKey(), req.Body)
	respData, err := n.SelfStream(stream.WarpRoute(req.Destination), req)
	if err != nil {
		return newErrorResp(err.Error())
	}
	return respData
}

// NodeReady reports whether main has attached a started node (the readiness
// probe gates traffic on it).
func (s *Server) NodeReady() bool {
	s.mx.RLock()
	defer s.mx.RUnlock()
	return s.node != nil
}

func newErrorResp(msg string) json.RawMessage {
	bt, _ := json.Marshal(event.ResponseError{Code: http.StatusInternalServerError, Message: msg})
	return bt
}

// aesCodec is the dashboard channel's wire form: AES-256-GCM with the preshared
// key (sha256 of the launch password) when one is set, plaintext otherwise. The
// is-first-run probe precedes the key and arrives in cleartext, so Decode reports
// per frame whether it was encrypted and Encode mirrors that on the reply.
type aesCodec struct{ key []byte }

var _ handlers.Codec = aesCodec{}

func (c aesCodec) Decode(frame []byte) (plain []byte, encrypted bool) {
	if len(c.key) == 0 {
		return frame, false
	}
	if p, err := security.AESGCMDecrypt(c.key, frame); err == nil {
		return p, true
	}
	return frame, false
}

func (c aesCodec) Encode(reply []byte, encrypted bool) ([]byte, error) {
	if !encrypted || len(c.key) == 0 {
		return reply, nil
	}
	return security.AESGCMEncrypt(c.key, reply)
}

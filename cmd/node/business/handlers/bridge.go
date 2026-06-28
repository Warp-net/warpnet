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

package handlers

import (
	"crypto/ed25519"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/security"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

const pathIsFirstRun = "is-first-run"

// maxInflightDispatches bounds the per-connection goroutines a slow client (or a
// burst of dashboard calls) can spawn, so one connection can't exhaust memory.
const maxInflightDispatches = 32

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     sameOrigin, // reject cross-site WebSocket hijacking
}

// sameOrigin permits only the dashboard served from this node: a request whose
// Origin host matches the Host it connects to. A missing Origin (non-browser
// clients, e.g. health probes) is allowed; any other origin is rejected so a
// malicious page can't open a /ws connection to this node.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == r.Host
}

type Codec interface {
	Decode(frame []byte) (plain []byte, encrypted bool)
	Encode(reply []byte, encrypted bool) ([]byte, error)
}

type Node interface {
	SelfStream(path stream.WarpRoute, data any) ([]byte, error)
}

// Authenticator is the slice of the auth service the dispatcher uses: log the
// owner in and out, and sign self-stream requests with their key.
type Authenticator interface {
	AuthLogin(message event.LoginEvent, psk security.PSK) (event.LoginResponse, error)
	AuthLogout()
	Reset()
	PrivateKey() ed25519.PrivateKey
}

// SessionInitializer brings up the network-scoped state (database, auth service
// and PSK) for the network the user picked on the login page, returning the
// authenticator and PSK to drive the login. It is idempotent: the first login
// creates the session and later logins reuse it.
type SessionInitializer func(network string) (Authenticator, security.PSK, error)

type BridgeHandler struct {
	codec       Codec
	initSession SessionInitializer
	firstRun    func(network string) bool

	mx   sync.RWMutex
	auth Authenticator
	node Node
}

func NewBridgeHandler(
	codec Codec,
	initSession SessionInitializer,
	firstRun func(network string) bool,
) *BridgeHandler {
	return &BridgeHandler{
		codec:       codec,
		initSession: initSession,
		firstRun:    firstRun,
	}
}

func (b *BridgeHandler) Handle() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Errorf("business: ws upgrade: %v", err)
			return
		}

		defer func() { _ = conn.Close() }()

		// Dispatch each message in its own goroutine so a slow libp2p self-stream
		// can't head-of-line block every other dashboard call on the connection.
		// writeMx serializes WriteMessage (gorilla allows a single writer); sem
		// bounds in-flight work; the frontend matches replies by message_id, so
		// out-of-order responses are fine.
		var writeMx sync.Mutex
		var inflight sync.WaitGroup
		sem := make(chan struct{}, maxInflightDispatches)

		respond := func(req event.Message, encrypted bool) {
			out, err := json.Marshal(b.dispatch(req))
			if err != nil {
				log.Errorf("business: ws marshal: %v", err)
				return
			}
			if out, err = b.codec.Encode(out, encrypted); err != nil {
				log.Errorf("business: ws encode: %v", err)
				return
			}
			writeMx.Lock()
			defer writeMx.Unlock()
			if err := conn.WriteMessage(websocket.TextMessage, out); err != nil {
				_ = conn.Close() // unblock ReadMessage so the loop exits
			}
		}

		for {
			_, frame, err := conn.ReadMessage()
			if err != nil {
				inflight.Wait()
				return
			}

			plain, encrypted := b.codec.Decode(frame)
			var req event.Message
			if err := json.Unmarshal(plain, &req); err != nil {
				log.Warnf("business: ws envelope: %v", err)
				continue
			}

			// Login/logout are connection-wide state transitions (they open/close
			// the DB and drive a shared auth handshake), so they must not overlap
			// any in-flight call: drain first, then run synchronously as a barrier.
			if req.Destination == event.PRIVATE_POST_LOGIN || req.Destination == event.PRIVATE_POST_LOGOUT {
				inflight.Wait()
				respond(req, encrypted)
				continue
			}

			sem <- struct{}{}
			inflight.Go(func() {
				defer func() { <-sem }()
				respond(req, encrypted)
			})
		}
	}
}

func (b *BridgeHandler) AttachNode(n Node) {
	b.mx.Lock()
	b.node = n
	b.mx.Unlock()
}

func (b *BridgeHandler) dispatch(req event.Message) event.Message {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("business: dispatch panic: %v", r)
		}
	}()
	resp := event.Message{
		MessageId:   req.MessageId,
		Destination: req.Destination,
		Timestamp:   time.Now(),
		Version:     "0.0.0",
	}

	switch req.Destination {
	case pathIsFirstRun:
		var ev event.LoginEvent
		_ = json.Unmarshal(req.Body, &ev) // body carries only the selected network
		body, _ := json.Marshal(b.firstRun(ev.Network))
		resp.Body = body
	case event.PRIVATE_POST_LOGIN:
		resp.Body = b.login(req.Body)
	case event.PRIVATE_POST_LOGOUT:
		b.mx.RLock()
		authn := b.auth
		b.mx.RUnlock()
		if authn != nil {
			authn.AuthLogout() // closes the database; the node keeps running
			authn.Reset()      // clear the auth guard so the next login can re-authenticate
		}
		resp.Body = json.RawMessage(`["logged_out"]`)
	default:
		resp.Body = b.call(req)
	}

	if resp.Body == nil {
		resp.Body = newErrorResp("response body is empty")
	}
	return resp
}

func (b *BridgeHandler) login(body json.RawMessage) json.RawMessage {
	var ev event.LoginEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return newErrorResp(err.Error())
	}

	// The network comes from the login form, so the session (database, auth
	// service, PSK) is brought up here on the first login rather than at boot.
	authn, psk, err := b.initSession(ev.Network)
	if err != nil {
		log.Errorf("business: init session: %v", err)
		return newErrorResp(err.Error())
	}
	b.mx.Lock()
	b.auth = authn
	b.mx.Unlock()

	loginResp, err := authn.AuthLogin(ev, psk)
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

func (b *BridgeHandler) call(req event.Message) json.RawMessage {
	b.mx.RLock()
	n := b.node
	authn := b.auth
	b.mx.RUnlock()
	if n == nil || authn == nil {
		return newErrorResp("not attached server node")
	}
	req.Signature = security.Sign(authn.PrivateKey(), req.Body)
	respData, err := n.SelfStream(stream.WarpRoute(req.Destination), req)
	if err != nil {
		return newErrorResp(err.Error())
	}
	return respData
}

func newErrorResp(msg string) json.RawMessage {
	bt, _ := json.Marshal(event.ResponseError{Code: http.StatusInternalServerError, Message: msg})
	return bt
}

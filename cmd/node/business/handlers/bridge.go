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

// logoutGracePeriod is how long the node waits after the last dashboard
// connection drops before logging the owner out. A page reload reconnects well
// within this window (cancelling the logout); closing the browser tab never
// reconnects, so the logout fires and the database is sealed.
const logoutGracePeriod = 5 * time.Second

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(_ *http.Request) bool { return true }, // same-origin dashboard
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
	IsAuthenticated() bool
	PrivateKey() ed25519.PrivateKey
}

type BridgeHandler struct {
	codec    Codec
	auth     Authenticator
	firstRun func() bool
	psk      security.PSK

	mx   sync.RWMutex
	node Node

	connMx      sync.Mutex
	activeConns int
	logoutTimer *time.Timer
}

func NewBridgeHandler(
	codec Codec,
	auth Authenticator,
	psk security.PSK,
	firstRun func() bool,
) *BridgeHandler {
	return &BridgeHandler{
		codec:    codec,
		auth:     auth,
		psk:      psk,
		firstRun: firstRun,
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

		// Track the dashboard connection so closing the last browser tab logs
		// the owner out (a reload reconnects within the grace period).
		b.connConnected()
		defer b.connDisconnected()

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

// connConnected records a new dashboard connection and cancels any pending
// auto-logout (e.g. a page reload reconnecting within the grace period).
func (b *BridgeHandler) connConnected() {
	b.connMx.Lock()
	defer b.connMx.Unlock()
	b.activeConns++
	if b.logoutTimer != nil {
		b.logoutTimer.Stop()
		b.logoutTimer = nil
	}
}

// connDisconnected records a dropped dashboard connection. When the last one
// goes away it arms a grace timer that logs the owner out unless a new
// connection arrives first, so closing the browser tab seals the node while a
// reload keeps the session.
func (b *BridgeHandler) connDisconnected() {
	b.connMx.Lock()
	defer b.connMx.Unlock()
	if b.activeConns > 0 {
		b.activeConns--
	}
	if b.activeConns > 0 {
		return
	}
	if b.logoutTimer != nil {
		b.logoutTimer.Stop()
	}
	b.logoutTimer = time.AfterFunc(logoutGracePeriod, b.autoLogout)
}

// autoLogout fires when the dashboard has been gone for the whole grace period.
// It bails if a tab reconnected meanwhile or no one is logged in.
func (b *BridgeHandler) autoLogout() {
	b.connMx.Lock()
	b.logoutTimer = nil
	reconnected := b.activeConns > 0
	b.connMx.Unlock()
	if reconnected || !b.auth.IsAuthenticated() {
		return
	}
	log.Infoln("business: dashboard closed, logging out")
	b.auth.AuthLogout() // closes the database; the node keeps running
	b.auth.Reset()      // clear the auth guard so the next login can re-authenticate
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
		body, _ := json.Marshal(b.firstRun())
		resp.Body = body
	case event.PRIVATE_POST_LOGIN:
		resp.Body = b.login(req.Body)
	case event.PRIVATE_POST_LOGOUT:
		b.auth.AuthLogout() // closes the database; the node keeps running
		b.auth.Reset()      // clear the auth guard so the next login can re-authenticate
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
	loginResp, err := b.auth.AuthLogin(ev, b.psk)
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
	b.mx.RUnlock()
	if n == nil {
		return newErrorResp("not attached server node")
	}
	req.Signature = security.Sign(b.auth.PrivateKey(), req.Body)
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

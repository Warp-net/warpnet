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
	PrivateKey() ed25519.PrivateKey
}

type BridgeHandler struct {
	codec    Codec
	auth     Authenticator
	firstRun func() bool
	psk      security.PSK

	mx   sync.RWMutex
	node Node
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

		for {
			_, frame, err := conn.ReadMessage()
			if err != nil {
				return
			}

			plain, encrypted := b.codec.Decode(frame)
			var req event.Message
			if err := json.Unmarshal(plain, &req); err != nil {
				log.Warnf("business: ws envelope: %v", err)
				continue
			}

			out, err := json.Marshal(b.dispatch(req))
			if err != nil {
				log.Errorf("business: ws marshal: %v", err)
				continue
			}
			if out, err = b.codec.Encode(out, encrypted); err != nil {
				log.Errorf("business: ws encode: %v", err)
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, out); err != nil {
				return
			}
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

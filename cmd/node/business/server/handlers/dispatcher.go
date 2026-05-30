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
	"net/http"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
)

// pathIsFirstRun is the control path the frontend sends (over the WS) before
// login to choose between the login and sign-up screens.
const pathIsFirstRun = "is-first-run"

// Node is all the dispatcher needs from the running node: feed it a request.
type Node interface {
	SelfStream(path stream.WarpRoute, data any) ([]byte, error)
}

// Authenticator is the slice of the auth service the dispatcher uses: log the
// owner in and out, and sign self-stream requests with their key.
type Authenticator interface {
	AuthLogin(message event.LoginEvent, psk security.PSK) (event.LoginResponse, error)
	AuthLogout()
	PrivateKey() ed25519.PrivateKey
}

// FirstRunner reports whether this is a first run (no owner created yet).
type FirstRunner interface {
	IsFirstRun() bool
}

// Dispatcher routes one request envelope: is-first-run and login/logout inline,
// every other path signed and run through the node's SelfStream (the same
// middleware and handler stack the desktop client drives). The node is attached
// to the dispatcher once it is built — the dispatcher, not the HTTP server, is
// what holds it.
type Dispatcher struct {
	auth     Authenticator
	firstRun FirstRunner
	psk      security.PSK

	mx   sync.RWMutex
	node Node
}

func NewDispatcher(auth Authenticator, firstRun FirstRunner, psk security.PSK) *Dispatcher {
	return &Dispatcher{auth: auth, firstRun: firstRun, psk: psk}
}

// Attach hands the started node to the dispatcher. main calls it once the node
// is up; until then node calls report "not ready".
func (d *Dispatcher) Attach(n Node) {
	d.mx.Lock()
	d.node = n
	d.mx.Unlock()
}

// NodeReady reports whether a started node has been attached (the readiness
// probe gates on it).
func (d *Dispatcher) NodeReady() bool {
	d.mx.RLock()
	defer d.mx.RUnlock()
	return d.node != nil
}

func (d *Dispatcher) Dispatch(req event.Message) event.Message {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("business: dispatch panic: %v", r)
		}
	}()
	resp := event.Message{MessageId: req.MessageId, Destination: req.Destination, Timestamp: time.Now(), Version: "0.0.0"}

	switch req.Destination {
	case pathIsFirstRun:
		body, _ := json.Marshal(d.firstRun.IsFirstRun())
		resp.Body = body
	case event.PRIVATE_POST_LOGIN:
		resp.Body = d.login(req.Body)
	case event.PRIVATE_POST_LOGOUT:
		d.auth.AuthLogout()
		resp.Body = json.RawMessage(`["logged_out"]`)
	default:
		resp.Body = d.nodeCall(req)
	}

	if resp.Body == nil {
		resp.Body = newErrorResp("response body is empty")
	}
	return resp
}

func (d *Dispatcher) login(body json.RawMessage) json.RawMessage {
	var ev event.LoginEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return newErrorResp(err.Error())
	}
	loginResp, err := d.auth.AuthLogin(ev, d.psk)
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

func (d *Dispatcher) nodeCall(req event.Message) json.RawMessage {
	d.mx.RLock()
	n := d.node
	d.mx.RUnlock()
	if n == nil {
		return newErrorResp("not attached server node")
	}

	req.Signature = security.Sign(d.auth.PrivateKey(), req.Body)
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

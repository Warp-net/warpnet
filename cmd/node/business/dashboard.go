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
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"sync"

	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

// wsPathIsFirstRun is a control path (not a node route) the frontend sends
// before logging in to decide between the login and sign-up screens.
const wsPathIsFirstRun = "is-first-run"

// errShortWSFrame is a package-level sentinel so the dynamic-error linter is
// satisfied (and callers could errors.Is it if they ever needed to).
var errShortWSFrame = errors.New("business: ws frame too short")

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// The dashboard is served from the same origin as the socket.
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// handleWS is the single transport: every request the dashboard makes — login,
// logout, is-first-run, and all node calls — rides this one connection.
//
// The connection carries its own auth: a successful login authenticates the
// socket (no cookies). is-first-run and the login exchange travel in cleartext
// (there is no key yet, and the login response only carries the network PSK,
// which is shared by every node on the network anyway); every frame after a
// successful login is AES-256-GCM sealed with sha256(password).
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("business: ws upgrade: %v", err)
		return
	}
	(&wsConn{conn: conn, srv: s}).serve()
}

type wsConn struct {
	conn    *websocket.Conn
	srv     *Server
	writeMu sync.Mutex

	stateMu sync.RWMutex
	authed  bool
	key     []byte
}

func (c *wsConn) serve() {
	defer func() { _ = c.conn.Close() }()
	for {
		_, frame, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		go c.handleFrame(frame)
	}
}

func (c *wsConn) handleFrame(frame []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("business: ws frame panic: %v", r)
		}
	}()

	c.stateMu.RLock()
	authed, key := c.authed, c.key
	c.stateMu.RUnlock()

	raw := frame
	if authed {
		plain, err := aesOpen(key, frame)
		if err != nil {
			log.Warnf("business: ws decrypt: %v", err)
			return
		}
		raw = plain
	}

	var msg AppMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		log.Warnf("business: ws envelope: %v", err)
		return
	}

	// Pre-login control path: report first-run state in cleartext.
	if !authed && msg.Path == wsPathIsFirstRun {
		body, _ := json.Marshal(c.srv.isFirstRun())
		c.send(false, AppMessage{MessageId: msg.MessageId, Path: msg.Path, Body: body})
		return
	}

	// Before login only the login frame is accepted.
	if !authed && msg.Path != event.PRIVATE_POST_LOGIN {
		c.send(false, AppMessage{
			MessageId: msg.MessageId,
			Path:      msg.Path,
			Body:      newErrorResp("unauthorized"),
		})
		return
	}

	resp, password := c.srv.dispatch(msg)

	if msg.Path == event.PRIVATE_POST_LOGIN && password != "" {
		// Login succeeded: the reply predates keying so it goes in cleartext;
		// from here on the socket is encrypted.
		c.send(false, resp)
		c.stateMu.Lock()
		c.authed = true
		c.key = deriveKey(password)
		c.stateMu.Unlock()
		return
	}

	c.send(authed, resp)

	if authed && msg.Path == event.PRIVATE_POST_LOGOUT {
		c.stateMu.Lock()
		c.authed = false
		c.key = nil
		c.stateMu.Unlock()
	}
}

// send marshals resp, seals it when the connection is encrypted, and writes one
// frame. Writes are serialised because a gossip/relay frame may be in flight on
// another goroutine for the same connection.
func (c *wsConn) send(encrypted bool, resp AppMessage) {
	out, err := json.Marshal(resp)
	if err != nil {
		log.Errorf("business: ws marshal: %v", err)
		return
	}
	if encrypted {
		c.stateMu.RLock()
		key := c.key
		c.stateMu.RUnlock()
		out, err = aesSeal(key, out)
		if err != nil {
			log.Errorf("business: ws seal: %v", err)
			return
		}
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.WriteMessage(websocket.TextMessage, out); err != nil {
		log.Debugf("business: ws write: %v", err)
	}
}

func deriveKey(password string) []byte {
	sum := sha256.Sum256([]byte(password))
	return sum[:]
}

// aesSeal returns base64(nonce || ciphertext+tag) — the frame layout the
// browser's WebCrypto AES-GCM produces and consumes.
func aesSeal(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nonce, nonce, plaintext, nil)
	enc := make([]byte, base64.StdEncoding.EncodedLen(len(ct)))
	base64.StdEncoding.Encode(enc, ct)
	return enc, nil
}

func aesOpen(key, b64 []byte) ([]byte, error) {
	data := make([]byte, base64.StdEncoding.DecodedLen(len(b64)))
	n, err := base64.StdEncoding.Decode(data, b64)
	if err != nil {
		return nil, err
	}
	data = data[:n]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errShortWSFrame
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

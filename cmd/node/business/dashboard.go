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

	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

// wsPathIsFirstRun is a control path (not a node route) the frontend sends
// before logging in to choose between the login and sign-up screens.
const wsPathIsFirstRun = "is-first-run"

var errShortWSFrame = errors.New("business: ws frame too short")

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(_ *http.Request) bool { return true }, // same-origin dashboard
}

// handleWS is the whole dashboard transport: a bridge between WebSocket frames
// and the node's libp2p stream handlers. Each frame is one request envelope; it
// is fed to dispatch (which signs it and runs it through SelfStream, exactly
// like the desktop client) and the response is written back.
//
// The connection carries its own auth: until a login succeeds, key is nil and
// only is-first-run and login are accepted, in cleartext (the login response
// only carries the network-wide PSK). A successful login derives the AES key
// from the password, and from then on every frame is AES-256-GCM sealed. Frames
// are handled in order — one dashboard, one user, no need for more.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("business: ws upgrade: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()

	var key []byte // nil => cleartext (pre-login); set => AES-encrypted

	for {
		_, frame, err := conn.ReadMessage()
		if err != nil {
			return
		}

		req, ok := decodeFrame(key, frame)
		if !ok {
			continue
		}

		// Pre-login: only the first-run probe and the login itself are allowed.
		if key == nil {
			switch req.Path {
			case wsPathIsFirstRun:
				body, _ := json.Marshal(s.isFirstRun())
				writeFrame(conn, nil, AppMessage{MessageId: req.MessageId, Path: req.Path, Body: body})
				continue
			case event.PRIVATE_POST_LOGIN:
				// handled below
			default:
				writeFrame(conn, nil, AppMessage{
					MessageId: req.MessageId, Path: req.Path, Body: newErrorResp("unauthorized"),
				})
				continue
			}
		}

		resp, password := s.dispatch(req)

		if req.Path == event.PRIVATE_POST_LOGIN && password != "" {
			// The login reply predates the key, so it goes in cleartext; every
			// frame after it is encrypted.
			writeFrame(conn, nil, resp)
			key = deriveKey(password)
			continue
		}

		writeFrame(conn, key, resp)
		if key != nil && req.Path == event.PRIVATE_POST_LOGOUT {
			key = nil
		}
	}
}

// decodeFrame decrypts (when keyed) and unmarshals one inbound frame.
func decodeFrame(key, frame []byte) (AppMessage, bool) {
	raw := frame
	if key != nil {
		plain, err := aesOpen(key, frame)
		if err != nil {
			log.Warnf("business: ws decrypt: %v", err)
			return AppMessage{}, false
		}
		raw = plain
	}
	var req AppMessage
	if err := json.Unmarshal(raw, &req); err != nil {
		log.Warnf("business: ws envelope: %v", err)
		return AppMessage{}, false
	}
	return req, true
}

// writeFrame marshals resp, seals it when keyed, and writes one frame.
func writeFrame(conn *websocket.Conn, key []byte, resp AppMessage) {
	out, err := json.Marshal(resp)
	if err != nil {
		log.Errorf("business: ws marshal: %v", err)
		return
	}
	if key != nil {
		if out, err = aesSeal(key, out); err != nil {
			log.Errorf("business: ws seal: %v", err)
			return
		}
	}
	if err := conn.WriteMessage(websocket.TextMessage, out); err != nil {
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
	gcm, err := newGCM(key)
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

	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errShortWSFrame
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

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
	"net/http"

	"github.com/Warp-net/warpnet/json"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(_ *http.Request) bool { return true }, // same-origin dashboard
}

// WS bridges WebSocket frames to the node's libp2p stream handlers: each frame
// is one request envelope, dispatched and answered. That is the whole handler —
// the routing lives in the Dispatcher, not here.
//
// key is the preshared AES-256 secret (sha256 of the launch password). When set,
// frames are AES-256-GCM sealed; the handler simply mirrors the inbound frame's
// encryption on the reply, so the cleartext probe (is-first-run) stays cleartext
// while everything sent with the key stays encrypted. When key is empty the
// channel is plaintext (rely on TLS / a trusted host).
func WS(d Dispatcher, key []byte) http.HandlerFunc {
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

			plain, encrypted := decode(key, frame)
			var req AppMessage
			if err := json.Unmarshal(plain, &req); err != nil {
				log.Warnf("business: ws envelope: %v", err)
				continue
			}

			out, err := json.Marshal(d.Dispatch(req))
			if err != nil {
				log.Errorf("business: ws marshal: %v", err)
				continue
			}
			if encrypted {
				if out, err = seal(key, out); err != nil {
					log.Errorf("business: ws seal: %v", err)
					continue
				}
			}
			if err := conn.WriteMessage(websocket.TextMessage, out); err != nil {
				return
			}
		}
	}
}

// decode reports whether the frame was encrypted (and returns its plaintext).
// With no key the frame is plaintext; with a key it is encrypted unless it
// fails to open (the cleartext is-first-run probe), in which case it is used
// as-is.
func decode(key, frame []byte) (plain []byte, encrypted bool) {
	if len(key) == 0 {
		return frame, false
	}
	if p, err := open(key, frame); err == nil {
		return p, true
	}
	return frame, false
}

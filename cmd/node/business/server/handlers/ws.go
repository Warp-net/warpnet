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
// is one request envelope, dispatched and answered. Framing — and any channel
// encryption — is the codec's job; the routing is the Dispatcher's. The handler
// itself is just the read → dispatch → write loop.
func WS(d Dispatcher, codec Codec) http.HandlerFunc {
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

			plain, encrypted := codec.Decode(frame)
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
			if out, err = codec.Encode(out, encrypted); err != nil {
				log.Errorf("business: ws encode: %v", err)
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, out); err != nil {
				return
			}
		}
	}
}

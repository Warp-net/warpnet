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

// Package handlers holds the dashboard's HTTP/WS handlers — one file per kind
// (ws, static, health). They depend only on a Dispatcher, so the server wires
// them without the handlers importing the server back.
package handlers

import stdjson "encoding/json"

// PathIsFirstRun is a control path (not a node route) the frontend sends before
// logging in to choose between the login and sign-up screens.
const PathIsFirstRun = "is-first-run"

// AppMessage is the Wails JSON envelope the frontend speaks, kept identical so
// the existing Vue client works against the business node unchanged.
type AppMessage struct {
	Body      stdjson.RawMessage `json:"body"`
	MessageId string             `json:"message_id"`
	NodeId    string             `json:"node_id"`
	Path      string             `json:"path"`
	Timestamp string             `json:"timestamp,omitempty"`
	Version   string             `json:"version"`
	Signature string             `json:"signature"`
}

// Dispatcher is the only thing the handlers need from the server: route one
// request envelope through the node, and report node readiness for health.
type Dispatcher interface {
	Dispatch(req AppMessage) AppMessage
	NodeReady() bool
}

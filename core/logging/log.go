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

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package logging

import golog "github.com/ipfs/go-log/v2"

var subsystems = []string{
	"autonat",
	"autonatv2",
	"autorelay",
	"basichost",
	"blankhost",
	"canonical-log",
	"connmgr",
	"dht",
	"dht.pb",
	"dht/RtDiversityFilter",
	"dht/RtRefreshManager",
	"dht/netsize",
	"discovery-backoff",
	"diversityFilter",
	"eventbus",
	"eventlog",
	"internal/nat",
	"ipns",
	"mdns",
	"nat",
	"net/identify",
	"p2p-circuit",
	"p2p-config",
	"p2p-holepunch",
	"peerstore",
	"peerstore/ds",
	"ping",
	"providers",
	"pstoremanager",
	"pubsub",
	"quic-transport",
	"quic-utils",
	"rcmgr",
	"relay",
	"reuseport-transport",
	"routedhost",
	"swarm2",
	"table",
	"tcp-demultiplex",
	"tcp-tpt",
	"test-logger",
	"upgrader",
	"websocket-transport",
	"webtransport",
	"webrtc-transport",
	"webrtc-transport-pion",
	"webrtc-udpmux",
}

func init() {
	level := "error"
	_ = golog.SetLogLevel("raftlib", level)
	_ = golog.SetLogLevel("raft", level)
	_ = golog.SetLogLevel("libp2p-raft", level)

	_ = golog.SetLogLevel("autonatv2", "debug")
	_ = golog.SetLogLevel("autonat", "debug")
	_ = golog.SetLogLevel("p2p-holepunch", "debug")
	_ = golog.SetLogLevel("relay", "debug")
	_ = golog.SetLogLevel("nat", "debug")
	_ = golog.SetLogLevel("p2p-circuit", "debug")
	_ = golog.SetLogLevel("basichost", level)
	_ = golog.SetLogLevel("swarm2", level)
	_ = golog.SetLogLevel("autorelay", "debug")
	_ = golog.SetLogLevel("net/identify", level)
	_ = golog.SetLogLevel("tcp-tpt", level)
}

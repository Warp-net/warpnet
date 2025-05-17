/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

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

	level := "debug"
	_ = golog.SetLogLevel("raftlib", "debug")
	_ = golog.SetLogLevel("raft", "debug")
	_ = golog.SetLogLevel("libp2p-raft", "debug")

	_ = golog.SetLogLevel("autonatv2", level)
	_ = golog.SetLogLevel("autonat", level)
	_ = golog.SetLogLevel("p2p-holepunch", "debug")
	_ = golog.SetLogLevel("relay", level)
	_ = golog.SetLogLevel("nat", level)
	_ = golog.SetLogLevel("p2p-circuit", level)
	_ = golog.SetLogLevel("basichost", "error")
	_ = golog.SetLogLevel("swarm2", "error")
	_ = golog.SetLogLevel("autorelay", level)
	_ = golog.SetLogLevel("net/identify", "error")
	_ = golog.SetLogLevel("tcp-tpt", "error")
}

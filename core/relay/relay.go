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

package relay

import (
	"github.com/Warp-net/warpnet/core/warpnet"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"time"
)

/*
  The libp2p Relay v2 library is an improved version of the connection relay mechanism in libp2p,
  designed for scenarios where nodes cannot establish direct connections due to NAT, firewalls,
  or other network restrictions.

  In a standard P2P network, nodes are expected to connect directly to each other. However, if one
  or both nodes are behind NAT or a firewall, direct connections may be impossible. In such cases,
  Relay v2 enables traffic to be relayed through intermediary nodes (relay nodes), allowing communication
  even in restricted network environments.

  ### **Key Features of libp2p Relay v2:**
  - **Automatic Relay Discovery and Usage**
    - If a node cannot establish a direct connection, it automatically finds a relay node.
  - **Operating Modes:**
    - **Active relay:** A node can act as a relay for others.
    - **Passive relay:** A node uses relay services without providing its own resources.
  - **Hole Punching**
    - Uses NAT traversal techniques (e.g., **DCUtR – Direct Connection Upgrade through Relay**)
      to attempt a direct connection before falling back to a relay.
  - **More Efficient Routing**
    - Relay v2 selects low-latency routes instead of simply forwarding all traffic through a single node.
  - **Bandwidth Optimization**

  ### **When is Relay v2 Needed?**
  - Nodes do not have a public IP address and are behind NAT.
  - A connection is required between network participants who cannot connect directly.
  - Reducing relay server load is important (compared to Relay v1).
  - **DCUtR is used** to attempt NAT traversal before falling back to a relay.
*/

var DefaultResources = relayv2.Resources{
	Limit: &relayv2.RelayLimit{
		Duration: DefaultRelayDurationLimit,
		Data:     DefaultRelayDataLimit,
	},

	ReservationTTL: time.Hour,

	MaxReservations: 128,
	MaxCircuits:     16,
	BufferSize:      4096,

	MaxReservationsPerIP:  8,
	MaxReservationsPerASN: 32,
}

const (
	DefaultRelayDataLimit     = 32 << 20 // 32 MiB
	DefaultRelayDurationLimit = 5 * time.Minute
)

func NewRelay(node warpnet.P2PNode) (*relayv2.Relay, error) {
	relay, err := relayv2.New(
		node,
		relayv2.WithResources(DefaultResources),
	)
	return relay, err
}

func WithDefaultResources() relayv2.Option {
	return relayv2.WithResources(DefaultResources)
}

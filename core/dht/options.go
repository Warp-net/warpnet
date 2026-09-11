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

package dht

import "github.com/Warp-net/warpnet/core/warpnet"

type dhtConfig struct {
	store                         RoutingStorer
	limits                        PeerAdmissionProvider
	addCallbacks, removeCallbacks []func(info warpnet.WarpPeerID)
	bootstrapNodes                []warpnet.WarpAddrInfo
	network                       string
}
type Option func(*dhtConfig)

// PeerAdmissionProvider is what the routing table asks of a peer before
// it holds it.
type PeerAdmissionProvider interface {
	PeerLimits(peerID warpnet.WarpPeerID) warpnet.PeerLimits
}

// PeerLimits is where the routing table reads whether it may hold a peer.
// Without it every peer is welcome.
func PeerLimits(limits PeerAdmissionProvider) Option {
	return func(c *dhtConfig) {
		c.limits = limits
	}
}

func RoutingStore(store RoutingStorer) Option {
	return func(c *dhtConfig) {
		c.store = store
	}
}

func RemovePeerCallbacks(cbs ...func(warpnet.WarpPeerID)) Option {
	return func(c *dhtConfig) {
		c.removeCallbacks = cbs
	}
}

func AddPeerCallbacks(cbs ...func(id warpnet.WarpPeerID)) Option {
	return func(c *dhtConfig) {
		c.addCallbacks = cbs
	}
}

func BootstrapNodes(nodes ...warpnet.WarpAddrInfo) Option {
	return func(c *dhtConfig) {
		c.bootstrapNodes = nodes
	}
}

func Network(network string) Option {
	return func(c *dhtConfig) {
		c.network = network
	}
}

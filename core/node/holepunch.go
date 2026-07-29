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

package node

import (
	"strings"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	log "github.com/sirupsen/logrus"
)

// NAT traversal is the one subsystem whose failures are invisible from the
// outside: a node that never punches keeps working, just permanently through a
// relay. These two hooks report DCUtR progress and the relay/direct state of
// every connection, so that traversal can be diagnosed without turning on
// libp2p's global debug logging.

// holePunchTracer reports every step of the DCUtR exchange. Outcomes are logged
// at info, individual attempts at debug: a punch normally takes up to three
// attempts, and only the outcome is interesting once it works.
type holePunchTracer struct{}

func (holePunchTracer) Trace(evt *holepunch.Event) {
	if evt == nil {
		return
	}
	peer := evt.Remote.ShortString()

	switch e := evt.Evt.(type) {
	case *holepunch.StartHolePunchEvt:
		log.Infof(
			"holepunch: start: peer %s, rtt %s, their addresses %s",
			peer, e.RTT, strings.Join(e.RemoteAddrs, " "),
		)
	case *holepunch.HolePunchAttemptEvt:
		log.Debugf("holepunch: attempt %d: peer %s", e.Attempt, peer)
	case *holepunch.EndHolePunchEvt:
		if e.Success {
			log.Infof("holepunch: SUCCESS: peer %s, took %s", peer, e.EllapsedTime)
			return
		}
		log.Warnf("holepunch: failed: peer %s, took %s, reason: %s", peer, e.EllapsedTime, e.Error)
	case *holepunch.DirectDialEvt:
		// A successful direct dial means the peer was reachable without any
		// punching, so DCUtR stops there.
		if e.Success {
			log.Infof("holepunch: SUCCESS - direct dial succeeded, no punch needed: peer %s, took %s", peer, e.EllapsedTime)
			return
		}
		log.Debugf("holepunch: direct dial failed, punching: peer %s, took %s", peer, e.EllapsedTime)
	case *holepunch.ProtocolErrorEvt:
		log.Warnf("holepunch: protocol error: peer %s, reason: %s", peer, e.Error)
	}
}

// connTracer logs how we are connected to a peer rather than merely that we are.
// Only relay usage and relay-to-direct upgrades reach info level; the rest is
// debug, because a busy node opens connections constantly.
type connTracer struct{}

func (connTracer) Listen(network.Network, warpnet.WarpAddress)      {}
func (connTracer) ListenClose(network.Network, warpnet.WarpAddress) {}

func (connTracer) Connected(n network.Network, c network.Conn) {
	if c == nil {
		return
	}
	peer := c.RemotePeer().ShortString()
	addr := c.RemoteMultiaddr()

	if isRelayed(c) {
		log.Infof("holepunch: connected through a relay: peer %s, address %s", peer, addr)
		return
	}

	// A direct connection that appears while a relayed one is still open is a
	// completed traversal: this is the line to grep for when asking whether
	// hole punching works in the wild.
	for _, other := range n.ConnsToPeer(c.RemotePeer()) {
		if other != c && isRelayed(other) {
			log.Infof("holepunch: SUCCESS - upgraded relay to direct: peer %s, address %s", peer, addr)
			return
		}
	}
	log.Debugf("holepunch: connected directly: peer %s, address %s", peer, addr)
}

func (connTracer) Disconnected(n network.Network, c network.Conn) {
	if c == nil {
		return
	}
	if !isRelayed(c) {
		log.Debugf("holepunch: direct connection closed: peer %s", c.RemotePeer().ShortString())
		return
	}
	// libp2p drops the relayed connection once a punch succeeds. Seeing the
	// direct one outlive it is the confirmation that the upgrade stuck. The
	// opposite case is not reported: at shutdown every connection closes, so an
	// empty connection set says nothing about traversal.
	for _, other := range n.ConnsToPeer(c.RemotePeer()) {
		if other != c && !isRelayed(other) {
			log.Infof("holepunch: relayed connection closed, staying direct: peer %s", c.RemotePeer().ShortString())
			return
		}
	}
	log.Debugf("holepunch: relayed connection closed: peer %s", c.RemotePeer().ShortString())
}

func isRelayed(c network.Conn) bool {
	return c.Stat().Limited || warpnet.IsRelayMultiaddress(c.RemoteMultiaddr())
}

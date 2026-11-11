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
	"context"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	log "github.com/sirupsen/logrus"
)

type Blocklister interface {
	IsBlocklisted(peerId string) bool
}

const blocklistSweepInterval = 10 * time.Second

func NewBlocklistOption(blocklist Blocklister) warpnet.WarpOption {
	return newBlocklistOption(blocklist, blocklistSweepInterval)
}

func newBlocklistOption(blocklist Blocklister, sweepInterval time.Duration) warpnet.WarpOption {
	g := &blocklistGater{blocklist: blocklist}
	return warpnet.NewChainOption(
		warpnet.NewConnectionGaterOption(g),
		warpnet.NewEventBusInvokeOption(func(bus warpnet.WarpEventBus) {
			emitter, err := bus.Emitter(new(warpnet.EvtReservationRefused))
			if err != nil {
				log.Errorf("node: blocklist: reservation refusal emitter: %v", err)
				return
			}
			g.emitter = emitter
		}),
		warpnet.NewSwarmInvokeOption(func(lc warpnet.WarpLifecycle, s *warpnet.Swarm) {
			s.Notify(&warpnet.WarpNotifyBundle{ConnectedF: g.closeBlocked})
			stop := make(chan struct{})
			lc.Append(warpnet.WarpHook{
				OnStart: func(context.Context) error {
					go g.sweepBlocked(s, sweepInterval, stop)
					return nil
				},
				OnStop: func(context.Context) error {
					close(stop)
					return nil
				},
			})
		}),
	)
}

type blocklistGater struct {
	blocklist Blocklister
	emitter   warpnet.WarpEmitter
}

func (g *blocklistGater) InterceptPeerDial(id warpnet.WarpPeerID) bool {
	return !g.blocklist.IsBlocklisted(id.String())
}

func (g *blocklistGater) InterceptAddrDial(id warpnet.WarpPeerID, _ warpnet.WarpAddress) bool {
	return g.InterceptPeerDial(id)
}

func (g *blocklistGater) InterceptAccept(warpnet.WarpConnMultiaddrs) bool {
	return true
}

func (g *blocklistGater) InterceptSecured(dir warpnet.WarpDirection, id warpnet.WarpPeerID, addrs warpnet.WarpConnMultiaddrs) bool {
	if g.InterceptPeerDial(id) {
		return true
	}
	if dir != warpnet.DirInbound || g.emitter == nil {
		return false
	}
	relayNodeId, ok := findRelayNodeId(addrs.RemoteMultiaddr())
	if !ok {
		return false
	}
	go func() {
		_ = g.emitter.Emit(warpnet.EvtReservationRefused{RelayNodeId: relayNodeId, PeerId: id})
	}()
	return false
}

func (g *blocklistGater) InterceptUpgraded(c warpnet.WarpConn) (bool, warpnet.WarpDisconnectReason) {
	return g.InterceptPeerDial(c.RemotePeer()), 0
}

func (g *blocklistGater) closeBlocked(_ warpnet.WarpNetwork, c warpnet.WarpConn) {
	if g.InterceptPeerDial(c.RemotePeer()) {
		return
	}
	go func() { _ = c.Close() }()
}

func (g *blocklistGater) sweepBlocked(s *warpnet.Swarm, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			for _, id := range s.Peers() {
				if g.InterceptPeerDial(id) {
					continue
				}
				_ = s.ClosePeer(id)
				s.Peerstore().RemovePeer(id)
				s.Peerstore().ClearAddrs(id)
			}
		}
	}
}

func findRelayNodeId(addr warpnet.WarpAddress) (warpnet.WarpPeerID, bool) {
	var relayNodeId warpnet.WarpPeerID
	for _, c := range addr {
		switch c.Code() {
		case warpnet.P_P2P:
			relayNodeId = warpnet.FromBytesToPeerID(c.RawValue())
		case warpnet.P_CIRCUIT:
			return relayNodeId, relayNodeId != ""
		}
	}
	return "", false
}

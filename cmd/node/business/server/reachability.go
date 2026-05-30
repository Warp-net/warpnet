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

package server

import (
	"context"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	log "github.com/sirupsen/logrus"
)

// reachabilityProbe is the node surface the public-IP tracker reads. The check
// runs from the outside, against the node's exported API, not internal state.
type reachabilityProbe interface {
	NodeInfo() warpnet.NodeInfo
	PublicAddrs() []warpnet.WarpAddress
}

// trackPublicReachability enforces the business node's public-IP obligation
// from the outside: it watches the node's AutoNAT verdict (NodeInfo().Reachability)
// and public addresses through the node's public API. It waits out a grace
// window (AutoNAT v2 reports Unknown/Private transiently at boot), returns as
// soon as the node looks public, and panics — crashing the process, which is
// the assertion — only after several consecutive private readings.
func trackPublicReachability(ctx context.Context, node reachabilityProbe) {
	const (
		grace         = 90 * time.Second
		sampleEvery   = 5 * time.Second
		privateStreak = 3
		maxWait       = 5 * time.Minute
	)

	select {
	case <-ctx.Done():
		return
	case <-time.After(grace):
	}

	deadline := time.Now().Add(maxWait)
	ticker := time.NewTicker(sampleEvery)
	defer ticker.Stop()

	streak := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			switch node.NodeInfo().Reachability {
			case warpnet.ReachabilityPublic:
				log.Infoln("business: reachability confirmed public")
				return
			case warpnet.ReachabilityPrivate:
				streak++
				log.Warnf("business: reachability reported private (%d/%d)", streak, privateStreak)
				if streak >= privateStreak && len(node.PublicAddrs()) == 0 {
					panic("business: node is privately reachable (behind NAT) — a business node must have a publicly addressable IP")
				}
			default:
				streak = 0
			}
			if time.Now().After(deadline) {
				log.Warnln("business: reachability still unknown after max wait; continuing without public confirmation")
				return
			}
		}
	}
}

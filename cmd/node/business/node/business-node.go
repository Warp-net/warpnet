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
	"crypto/ed25519"
	"github.com/Warp-net/warpnet/core/stream"
	"time"

	member "github.com/Warp-net/warpnet/cmd/node/member/node"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
)

type BusinessNode struct {
	mn *member.MemberNode
}

func NewBusinessNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	ownNodeId warpnet.WarpPeerID,
	authRepo member.AuthProvider,
	db member.Storer,
	network string,
	bootstrapNodes []warpnet.WarpAddrInfo,
	metrics member.MetricsOnlinePusher,
) (*BusinessNode, error) {
	mn, err := member.NewMemberNode(
		ctx, privKey, psk, ownNodeId,
		authRepo, db, network, bootstrapNodes, metrics,
	)
	if err != nil {
		return nil, err
	}
	bn := &BusinessNode{mn: mn}
	go bn.trackPublicReachability(ctx)
	return bn, nil
}

func (b *BusinessNode) Start() error {
	return b.mn.Start()
}

func (b *BusinessNode) NodeInfo() warpnet.NodeInfo {
	info := b.mn.NodeInfo()
	info.Type = warpnet.BusinessNode
	return info
}

func (b *BusinessNode) SelfStream(path stream.WarpRoute, data any) ([]byte, error) {
	return b.mn.SelfStream(path, data)
}

const (
	gracePeriod   = 90 * time.Second
	sampleEvery   = 10 * time.Second
	privateStreak = 5
)

func (b *BusinessNode) trackPublicReachability(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(gracePeriod):
	}

	ticker := time.NewTicker(sampleEvery)
	defer ticker.Stop()

	streak := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			switch b.NodeInfo().Reachability {
			case warpnet.ReachabilityPrivate:
				streak++
				log.Warnf("business: reachability reported private (%d/%d)", streak, privateStreak)
				if streak >= privateStreak {
					// Used to panic here; relay-only periods happen, so just escalate.
					log.Errorf("business: node is privately reachable (behind NAT) — a business node must have a publicly addressable IP")
				}
			default:
				streak = 0
			}
		}
	}
}

func (b *BusinessNode) Stop() {
	b.mn.Stop()
}

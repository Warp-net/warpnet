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

// Package node hosts the business node. A business node IS a member node — same
// discovery, DHT, MDNS, pubsub, relay and the full handler set, so its profile
// and posts are queryable like any user — so it embeds *member.MemberNode
// rather than re-declaring any of that. It adds only what makes it a business
// node: Role="business" on the wire, access to the node's gossip for the
// moderator (see moderation.go), and tearing that moderator down on Stop.
package node

import (
	"context"
	"crypto/ed25519"

	"github.com/Masterminds/semver/v3"
	member "github.com/Warp-net/warpnet/cmd/node/member/node"
	"github.com/Warp-net/warpnet/core/handler"
	corePubsub "github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/security"
)

type BusinessNode struct {
	*member.MemberNode

	// moder is the moderator engine wrapper set by StartModerator. Held as a
	// closer so the node needn't depend on the moderator package's concrete type.
	moder interface{ Close() }
}

func NewBusinessNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	ownNodeId warpnet.WarpPeerID,
	selfHashHex string,
	version *semver.Version,
	authRepo member.AuthProvider,
	db member.Storer,
	bootstrapNodes []warpnet.WarpAddrInfo,
	metrics member.MetricsOnlinePusher,
) (*BusinessNode, error) {
	mn, err := member.NewMemberNode(
		ctx, privKey, psk, ownNodeId, selfHashHex, version,
		authRepo, db, bootstrapNodes, metrics,
	)
	if err != nil {
		return nil, err
	}
	return &BusinessNode{MemberNode: mn}, nil
}

// Start brings up the embedded member node, then rebinds PUBLIC_GET_INFO so the
// response advertises Role="business". The member-registered handler closes
// over the MemberNode, whose NodeInfo has no role, so the rebind is what lets
// peers learn the kind via requestNodeInfo.
func (b *BusinessNode) Start() error {
	if err := b.MemberNode.Start(); err != nil {
		return err
	}
	b.SetStreamHandlers(warpnet.WarpStreamHandler{
		Path:    event.PUBLIC_GET_INFO,
		Handler: handler.StreamGetInfoHandler(b, nil),
	})
	return nil
}

// NodeInfo is the member node info plus Role. OwnerId stays the real user id,
// so the node is still cached and queried like any member.
func (b *BusinessNode) NodeInfo() warpnet.NodeInfo {
	ni := b.MemberNode.NodeInfo()
	ni.Role = warpnet.BusinessRole
	return ni
}

// ID satisfies the moderator's ModeratorNode interface.
func (b *BusinessNode) ID() warpnet.WarpPeerID {
	return b.Node().ID()
}

// Gossip is the node's single gossip instance, exposed for the moderator wiring.
func (b *BusinessNode) Gossip() *corePubsub.Gossip {
	ps := b.PubSub()
	if ps == nil {
		return nil
	}
	return ps.Gossip()
}

func (b *BusinessNode) Stop() {
	if b == nil {
		return
	}
	if b.moder != nil {
		b.moder.Close()
	}
	b.MemberNode.Stop()
}

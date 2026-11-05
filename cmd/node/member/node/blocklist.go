package node

import (
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

type memberBlocklist struct {
	*database.NodeRepo
	onBlock func(peer.ID)
}

func (b *memberBlocklist) BlocklistPermanent(id string) error {
	if err := b.NodeRepo.BlocklistPermanent(id); err != nil {
		return err
	}
	b.disconnect(id)
	return nil
}

func (b *memberBlocklist) BlocklistExponential(id string) error {
	if err := b.NodeRepo.BlocklistExponential(id); err != nil {
		return err
	}
	b.disconnect(id)
	return nil
}

func (b *memberBlocklist) disconnect(id string) {
	if b.onBlock != nil {
		b.onBlock(warpnet.FromStringToPeerID(id))
	}
}

var _ connmgr.ConnectionGater = (*memberBlocklist)(nil)

func (b *memberBlocklist) InterceptPeerDial(id peer.ID) bool {
	return !b.IsBlocklisted(id.String())
}

func (b *memberBlocklist) InterceptAddrDial(id peer.ID, _ ma.Multiaddr) bool {
	return b.InterceptPeerDial(id)
}

func (b *memberBlocklist) InterceptAccept(network.ConnMultiaddrs) bool {
	// The remote peer's identity is not authenticated until InterceptSecured.
	return true
}

func (b *memberBlocklist) InterceptSecured(_ network.Direction, id peer.ID, _ network.ConnMultiaddrs) bool {
	return b.InterceptPeerDial(id)
}

func (b *memberBlocklist) InterceptUpgraded(conn network.Conn) (bool, control.DisconnectReason) {
	return b.InterceptPeerDial(conn.RemotePeer()), 0
}

type memberRoutingRatings struct {
	PeersRatings
	blocklist *memberBlocklist
}

func (r memberRoutingRatings) IsAllowedInDHT(id warpnet.WarpPeerID) bool {
	return r.blocklist.InterceptPeerDial(id) && r.PeersRatings.IsAllowedInDHT(id)
}

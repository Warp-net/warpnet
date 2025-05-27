package encrypted

import (
	"crypto/ed25519"
	"net"

	"github.com/Arceliar/phony"

	"github.com/Arceliar/ironwood/network"
	"github.com/Arceliar/ironwood/types"
)

type PacketConn struct {
	actor phony.Inbox
	*network.PacketConn
	secretEd  edPriv
	secretBox boxPriv
	sessions  sessionManager
	network   netManager
	Debug     Debug
}

// NewPacketConn returns a *PacketConn struct which implements the types.PacketConn interface.
func NewPacketConn(secret ed25519.PrivateKey, options ...network.Option) (*PacketConn, error) {
	npc, err := network.NewPacketConn(secret, options...)
	if err != nil {
		return nil, err
	}
	pc := &PacketConn{PacketConn: npc}
	copy(pc.secretEd[:], secret[:])
	pc.secretBox = *pc.secretEd.toBox()
	pc.sessions.init(pc)
	pc.network.init(pc)
	pc.Debug.init(pc)
	return pc, nil
}

func (pc *PacketConn) ReadFrom(p []byte) (n int, from net.Addr, err error) {
	pc.network.read()
	info := <-pc.network.readCh
	if info.err != nil {
		err = info.err
		return
	}
	n, from = len(info.data), types.Addr(info.from.asKey())
	if n > len(p) {
		n = len(p)
	}
	copy(p, info.data[:n])
	freeBytes(info.data)
	return
}

func (pc *PacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	select {
	case <-pc.network.closed:
		return 0, types.ErrClosed
	default:
	}
	destKey, ok := addr.(types.Addr)
	if !ok || len(destKey) != edPubSize {
		return 0, types.ErrBadAddress
	}
	if uint64(len(p)) > pc.MTU() {
		return 0, types.ErrOversizedMessage
	}
	n = len(p)
	var dest edPub
	copy(dest[:], destKey)
	pc.sessions.writeTo(dest, append(allocBytes(0), p...))
	return
}

// MTU returns the maximum transmission unit of the PacketConn, i.e. maximum safe message size to send over the network.
func (pc *PacketConn) MTU() uint64 {
	return pc.PacketConn.MTU() - sessionTrafficOverhead
}

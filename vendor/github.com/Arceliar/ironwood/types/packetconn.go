package types

import (
	"crypto/ed25519"
	"net"
)

type PacketConn interface {
	net.PacketConn
	// HandleConn expects a peer's public key as its first argument, and a net.Conn with TCP-like semantics (reliable ordered delivery) as its second argument.
	// This function blocks while the net.Conn is in use, and returns an error if any occurs.
	// This function returns (almost) immediately if PacketConn.Close() is called.
	// In all cases, the net.Conn is closed before returning.
	HandleConn(key ed25519.PublicKey, conn net.Conn, prio uint8) error

	// IsClosed returns true if and only if the connection is closed.
	// This is to check if the PacketConn is closed without potentially being stuck on a blocking operation (e.g. a read or write).
	IsClosed() bool

	// PrivateKey returns the ed25519.PrivateKey used to initialize the PacketConn.
	PrivateKey() ed25519.PrivateKey

	// MTU returns the maximum transmission unit of the PacketConn, i.e. maximum safe message size to send over the network.
	MTU() uint64

	// SendLookup sends a lookup for a given (possibly partial) key.
	SendLookup(target ed25519.PublicKey)
}

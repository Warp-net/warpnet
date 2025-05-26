package types

import (
	"crypto/ed25519"
	"encoding/hex"
)

// Addr implements the `net.Addr` interface for `ed25519.PublicKey` values.
type Addr ed25519.PublicKey

// Network returns "ed25519.PublicKey" as a string, but is otherwise unused.
func (a Addr) Network() string {
	return "ed25519.PublicKey"
}

// String returns the ed25519.PublicKey as a hexidecimal string, but is otherwise unused.
func (a Addr) String() string {
	return hex.EncodeToString(a)
}

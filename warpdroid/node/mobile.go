//go:build mobile

package node

import (
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
)

// Mobile-friendly wrapper types for gomobile compatibility
// gomobile bind has limitations on complex types

// The Kotlin side runs requests concurrently and may shut the node down
// from another thread while they are in flight, so the instance is only
// ever read into a local and swapped atomically.
var clientInstance atomic.Pointer[clientNode]

// Initialize method creates a new WarpNet client with optional PSK
// Returns error message or empty string on success
func Initialize(privKeyHex, warpNetwork, pskHex, bootstrapNodes string) string {
	var (
		psk, privKey []byte
		err          error
	)

	if clientInstance.Load() != nil {
		return "already initialized"
	}

	if pskHex != "" {
		psk, err = hex.DecodeString(pskHex)
		if err != nil {
			return fmt.Sprintf("invalid PSK: %v", err)
		}
	}
	if privKeyHex != "" {
		privKey, err = hex.DecodeString(privKeyHex)
		if err != nil {
			return fmt.Sprintf("invalid PK: %v", err)
		}
	}

	client, err := newClient(privKey, psk, warpNetwork, strings.Split(bootstrapNodes, ","))
	if err != nil {
		return fmt.Sprintf("failed to create client: %v", err)
	}

	if !clientInstance.CompareAndSwap(nil, client) {
		_ = client.close()
		return "already initialized"
	}
	return ""
}

func Connect(addrInfo string) string {
	c := clientInstance.Load()
	if c == nil {
		return "client not initialized"
	}

	err := c.connect(addrInfo)
	if err != nil {
		return fmt.Sprintf("connection failed: %v", err)
	}

	return ""
}

func Stream(protocolID string, data string) string {
	c := clientInstance.Load()
	if c == nil {
		return "client not initialized"
	}

	response, err := c.stream(protocolID, []byte(data))
	if err != nil {
		return err.Error()
	}

	return string(response)
}

// Sign returns the base64-encoded Ed25519 signature of the given signing input
// (body followed by the timestamp as decimal Unix nanoseconds), computed with
// the libp2p identity key from Initialize. Returns "" if uninitialized, or an
// "error: "-prefixed string on failure, to keep the gomobile signature simple.
func Sign(body string) string {
	c := clientInstance.Load()
	if c == nil {
		return ""
	}
	sig, err := c.sign([]byte(body))
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return sig
}

func PeerID() string {
	c := clientInstance.Load()
	if c == nil {
		return ""
	}
	return c.getPeerID()
}

func IsConnected() string {
	c := clientInstance.Load()
	if c == nil {
		return "false"
	}
	if c.isConnected() {
		return "true"
	}
	return "false"
}

// Connectedness returns the current libp2p connectedness to the paired
// desktop peer as a stringly-typed snapshot. Returned values mirror
// network.Connectedness#String — "Connected", "Limited", "NotConnected",
// "CanConnect", "CannotConnect" — plus "Uninitialised" when no client
// instance exists. The Kotlin ConnectionMonitor polls this every couple
// of seconds and drives reconnect / UI state from the result; Go owns
// only the snapshot, never the lifecycle.
func Connectedness() string {
	c := clientInstance.Load()
	if c == nil {
		return "Uninitialised"
	}
	return c.connectedness()
}

func Disconnect() string {
	c := clientInstance.Load()
	if c == nil {
		return ""
	}

	err := c.disconnect()
	if err != nil {
		return fmt.Sprintf("disconnect failed: %v", err)
	}

	return ""
}

// Pause background transition
func Pause() {
	c := clientInstance.Load()
	if c == nil {
		return
	}
	c.pause()
}

// Resume foreground transition
func Resume() {
	c := clientInstance.Load()
	if c == nil {
		return
	}
	c.resume()
}

// RefreshPeerAddrs merges the supplied newline-separated multiaddrs into
// the libp2p peerstore for the paired desktop peer. Called by the Kotlin
// side after parsing a /private/post/pair response, which now carries
// the fat node's current public addresses on every successful pair.
func RefreshPeerAddrs(addrs string) string {
	c := clientInstance.Load()
	if c == nil {
		return "client not initialized"
	}
	if err := c.refreshPeerAddrs(addrs); err != nil {
		return err.Error()
	}
	return ""
}

func Shutdown() string {
	c := clientInstance.Swap(nil)
	if c == nil {
		return ""
	}

	if err := c.close(); err != nil {
		return fmt.Sprintf("shutdown failed: %v", err)
	}

	return ""
}

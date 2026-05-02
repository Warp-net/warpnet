package node

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
)

// Mobile-friendly wrapper types for gomobile compatibility
// gomobile bind has limitations on complex types

var clientInstance *clientNode

// Initialize method creates a new WarpNet client with optional PSK
// Returns error message or empty string on success
func Initialize(privKeyHex, warpNetwork, pskHex, bootstrapNodes string) string {
	var (
		psk, privKey []byte
		err          error
	)

	if clientInstance != nil {
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

	clientInstance = client
	return ""
}

func Connect(addrInfo string) string {
	if clientInstance == nil {
		return "client not initialized"
	}

	err := clientInstance.connect(addrInfo)
	if err != nil {
		return fmt.Sprintf("connection failed: %v", err)
	}

	return ""
}

func Stream(protocolID string, data string) string {
	if clientInstance == nil {
		return "client not initialized"
	}

	response, err := clientInstance.stream(protocolID, []byte(data))
	if err != nil {
		return err.Error()
	}

	return string(response)
}

// Sign returns the base64-encoded Ed25519 signature of body computed with the
// libp2p identity key passed to Initialize. The Kotlin envelope signer uses
// this so the desktop's auth middleware verifies the signature against the
// same peer ID it sees on the libp2p stream. Returns an empty string if the
// client is not initialized; signing errors are returned with an "error: "
// prefix to keep the gomobile signature simple.
func Sign(body string) string {
	if clientInstance == nil {
		return ""
	}
	sig, err := clientInstance.sign([]byte(body))
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return sig
}

func PeerID() string {
	if clientInstance == nil {
		return ""
	}
	return clientInstance.getPeerID()
}

func IsConnected() string {
	if clientInstance == nil {
		return "false"
	}
	if clientInstance.isConnected() {
		return "true"
	}
	return "false"
}

func Disconnect() string {
	if clientInstance == nil {
		return ""
	}

	err := clientInstance.disconnect()
	if err != nil {
		return fmt.Sprintf("disconnect failed: %v", err)
	}

	return ""
}

// Pause background transition
func Pause() {
	if clientInstance == nil || clientInstance.host == nil {
		return
	}
	for _, c := range clientInstance.host.Network().Conns() {
		_ = c.Close()
	}
}

// Resume foreground transition
func Resume() {
	if clientInstance == nil || clientInstance.host == nil {
		return
	}
	for _, id := range clientInstance.host.Peerstore().PeersWithAddrs() {
		info := clientInstance.host.Peerstore().PeerInfo(id)
		_ = clientInstance.host.Connect(context.Background(), info)
	}
}

func Shutdown() string {
	if clientInstance == nil {
		return ""
	}

	err := clientInstance.close()
	if err != nil {
		return fmt.Sprintf("shutdown failed: %v", err)
	}

	clientInstance = nil
	return ""
}

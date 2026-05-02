package node

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
)

func newKeyHex(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	return hex.EncodeToString(priv)
}

func newPSKHex() string {
	psk := make([]byte, 32)
	for i := range psk {
		psk[i] = byte(i)
	}
	return hex.EncodeToString(psk)
}

func TestInitializeRejectsInvalidPrivKey(t *testing.T) {
	clientInstance = nil

	result := Initialize("not-hex!", "testnet", newPSKHex(), "addr")
	if result == "" || !strings.Contains(result, "invalid PK") {
		t.Fatalf("expected invalid PK error, got: %q", result)
	}
}

func TestInitializeRejectsInvalidPSK(t *testing.T) {
	clientInstance = nil

	result := Initialize(newKeyHex(t), "testnet", "not-hex!", "addr")
	if result == "" || !strings.Contains(result, "invalid PSK") {
		t.Fatalf("expected invalid PSK error, got: %q", result)
	}
}

func TestInitializeRejectsWhenAlreadyInitialized(t *testing.T) {
	// Simulate an already-initialized client so we hit the guard without
	// needing a live network.
	clientInstance = &clientNode{}
	defer func() { clientInstance = nil }()

	result := Initialize(newKeyHex(t), "testnet", newPSKHex(), "addr")
	if result != "already initialized" {
		t.Fatalf("expected already-initialized error, got: %q", result)
	}
}

func TestConnectFailsWhenNotInitialized(t *testing.T) {
	clientInstance = nil

	result := Connect("/ip4/127.0.0.1/tcp/4011/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j")
	if result != "client not initialized" {
		t.Fatalf("expected client-not-initialized error, got: %q", result)
	}
}

func TestStreamFailsWhenNotInitialized(t *testing.T) {
	clientInstance = nil

	result := Stream("/test/protocol", "{}")
	if result != "client not initialized" {
		t.Fatalf("expected client-not-initialized error, got: %q", result)
	}
}

func TestPeerIDEmptyWhenNotInitialized(t *testing.T) {
	clientInstance = nil

	if got := PeerID(); got != "" {
		t.Fatalf("expected empty peer id, got: %q", got)
	}
}

func TestIsConnectedFalseWhenNotInitialized(t *testing.T) {
	clientInstance = nil

	if got := IsConnected(); got != "false" {
		t.Fatalf("expected \"false\", got: %q", got)
	}
}

func TestDisconnectNoopWhenNotInitialized(t *testing.T) {
	clientInstance = nil

	if got := Disconnect(); got != "" {
		t.Fatalf("expected empty disconnect result, got: %q", got)
	}
}

func TestShutdownNoopWhenNotInitialized(t *testing.T) {
	clientInstance = nil

	if got := Shutdown(); got != "" {
		t.Fatalf("expected empty shutdown result, got: %q", got)
	}
}

func TestPauseResumeNoopWhenNotInitialized(t *testing.T) {
	clientInstance = nil

	// Must not panic when there's nothing to pause/resume.
	Pause()
	Resume()
}

package node

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

// freshKey returns a 64-byte libp2p Ed25519 private key (seed || pub).
func freshKey(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	return priv
}

// freshPSK returns a 32-byte PSK; valid length for libp2p PrivateNetwork.
func freshPSK(t *testing.T) []byte {
	t.Helper()
	psk := make([]byte, 32)
	for i := range psk {
		psk[i] = byte(i)
	}
	return psk
}

// newClient requires live bootstrap reachability; these tests exercise the
// preflight validation branches so they don't need a network.

func TestNewClientRejectsMissingPSK(t *testing.T) {
	_, err := newClient(freshKey(t), nil, "testnet", []string{"addr"})
	if err == nil {
		t.Fatal("expected psk required error")
	}
}

func TestNewClientRejectsMissingBootstrap(t *testing.T) {
	_, err := newClient(freshKey(t), freshPSK(t), "testnet", nil)
	if err == nil {
		t.Fatal("expected bootstrap required error")
	}
}

func TestNewClientRejectsInvalidPrivKey(t *testing.T) {
	_, err := newClient([]byte{0x00}, freshPSK(t), "testnet", []string{"addr"})
	if err == nil {
		t.Fatal("expected invalid priv key error")
	}
}

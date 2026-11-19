package security

import (
	"bytes"
	"testing"
)

func TestNostrKeyIsStablePerNodeKey(t *testing.T) {
	a, err := GenerateKeyFromSeed([]byte("node-a"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := GenerateKeyFromSeed([]byte("node-b"))
	ka, err := NostrKey(a)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := NostrKey(a)
	kb, _ := NostrKey(b)
	if ka != again {
		t.Fatal("same node key must give the same Nostr key")
	}
	if ka == kb {
		t.Fatal("different node keys must give different Nostr keys")
	}
	if bytes.Compare(ka[:], secp256k1Order) >= 0 {
		t.Fatal("key must be below the curve order")
	}
	if _, err := NostrKey(nil); err == nil {
		t.Fatal("an empty node key must be rejected")
	}
}

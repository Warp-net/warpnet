package security

import (
	"bytes"
	"testing"
)

func TestBridgeKeyIsStablePerNodeKey(t *testing.T) {
	a, err := GenerateKeyFromSeed([]byte("node-a"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := GenerateKeyFromSeed([]byte("node-b"))
	ka, err := BridgeKey(a)
	if err != nil || len(ka) != 32 {
		t.Fatalf("key %x, %v", ka, err)
	}
	again, _ := BridgeKey(a)
	kb, _ := BridgeKey(b)
	if !bytes.Equal(ka, again) || bytes.Equal(ka, kb) {
		t.Fatal("one stable key per node key")
	}
	if bytes.Contains(a, ka) {
		t.Fatal("the bridge key must not expose the node key")
	}
	if _, err := BridgeKey(nil); err == nil {
		t.Fatal("an empty node key must be rejected")
	}
}

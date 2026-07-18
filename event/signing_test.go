// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package event_test

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
)

// TestMessage_SigningBytes_WireRoundTrip guards the cross-language contract:
// the bytes a Go sender signs must equal what a receiver reconstructs from the
// raw wire body + timestamp string, or signatures would not verify.
func TestMessage_SigningBytes_WireRoundTrip(t *testing.T) {
	msg := event.Message{
		Body:      json.RawMessage(`{"hello":"world"}`),
		Timestamp: time.Now().UTC(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var wire struct {
		Body      json.RawMessage `json:"body"`
		Timestamp string          `json:"timestamp"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	reconstructed := append(append([]byte{}, wire.Body...), wire.Timestamp...)
	if string(reconstructed) != string(msg.SigningBytes()) {
		t.Fatalf("signing bytes mismatch:\n sender = %q\n wire   = %q", msg.SigningBytes(), reconstructed)
	}
}

// TestMessage_SignatureBindsTimestamp proves the timestamp is covered by the
// signature: refreshing a captured message's timestamp (the replay move) must
// invalidate it.
func TestMessage_SignatureBindsTimestamp(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	msg := event.Message{
		Body:      json.RawMessage(`{"amount":100}`),
		Timestamp: time.Now().UTC(),
	}
	msg.Signature = security.Sign(priv, msg.SigningBytes())

	if err := security.VerifySignature(pub, msg.SigningBytes(), msg.Signature); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}

	tampered := msg
	tampered.Timestamp = msg.Timestamp.Add(time.Hour)
	if err := security.VerifySignature(pub, tampered.SigningBytes(), msg.Signature); err == nil {
		t.Fatal("signature verified after timestamp tampering; timestamp not bound")
	}
}

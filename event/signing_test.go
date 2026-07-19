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

// SigningBytes must survive a JSON round-trip unchanged.
func TestMessage_SigningBytes_JSONRoundTrip(t *testing.T) {
	msg := event.Message{
		Body:      json.RawMessage(`{"hello":"world"}`),
		Timestamp: time.Now().UTC(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got event.Message
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if string(got.SigningBytes()) != string(msg.SigningBytes()) {
		t.Fatalf("signing bytes changed across JSON round-trip:\n before = %q\n after  = %q", msg.SigningBytes(), got.SigningBytes())
	}
}

// Tampering with the timestamp must invalidate the signature.
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

//nolint:all
package handler

import (
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

func TestStreamNodesPairingHandler(t *testing.T) {
	serverToken := "secret-token"
	serverAuth := domain.AuthNodeInfo{
		Identity: domain.Identity{Token: serverToken},
	}

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamNodesPairingHandler(serverAuth)
		_, err := h([]byte("{invalid"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty token", func(t *testing.T) {
		h := StreamNodesPairingHandler(serverAuth)
		// Use raw JSON to avoid peer.ID marshaling issues
		_, err := h([]byte(`{"identity":{"token":""}}`), nil)
		if err == nil {
			t.Fatal("expected error for empty token")
		}
		if err.Error() != "empty token" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("token mismatch", func(t *testing.T) {
		h := StreamNodesPairingHandler(serverAuth)
		_, err := h([]byte(`{"identity":{"token":"wrong-token"}}`), nil)
		if err == nil {
			t.Fatal("expected error for token mismatch")
		}
		if err.Error() != "token mismatch" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("success - token match", func(t *testing.T) {
		h := StreamNodesPairingHandler(serverAuth)
		_, err := h([]byte(`{"identity":{"token":"secret-token"}}`), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("success - returns accepted", func(t *testing.T) {
		h := StreamNodesPairingHandler(serverAuth)
		resp, err := h([]byte(`{"identity":{"token":"secret-token"}}`), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
	})
}

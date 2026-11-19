//nolint:all
package handler

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/core/fediverse"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p/core/peer"
)

type stubNostrOwner struct{ id string }

func (s stubNostrOwner) GetOwner() domain.Owner { return domain.Owner{UserId: s.id} }

type stubNostrUsers struct{ user domain.User }

func (s stubNostrUsers) Get(string) (domain.User, error) { return s.user, nil }

func nostrKeyStream(t *testing.T, id string) stubInfoStream {
	t.Helper()
	p, err := peer.Decode(id)
	if err != nil {
		t.Fatal(err)
	}
	return stubInfoStream{conn: stubInfoConn{remotePeerID: p}}
}

func TestStreamGetNostrKeyHandler(t *testing.T) {
	priv, err := security.GenerateKeyFromSeed([]byte("nostr-owner"))
	if err != nil {
		t.Fatal(err)
	}
	allowed := domain.User{Id: "owner", Metadata: map[string]string{"nostr": "allow"}}
	gateway := nostrKeyStream(t, fediverse.GatewayNodeID())
	stranger := nostrKeyStream(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	body := []byte(`{"user_id":"owner"}`)

	h := StreamGetNostrKeyHandler(stubNostrOwner{"owner"}, stubNostrUsers{allowed}, priv)
	resp, err := h(body, gateway)
	if err != nil {
		t.Fatalf("gateway with consent: %v", err)
	}
	want, _ := security.NostrKey(priv)
	if got := resp.(event.GetNostrKeyResponse); got.Key != hex.EncodeToString(want[:]) || got.UserId != "owner" {
		t.Fatalf("response = %+v", got)
	}

	if _, err := h(body, stranger); !errors.Is(err, ErrNostrKeyNotGateway) {
		t.Fatalf("a peer other than the gateway must get nothing: %v", err)
	}
	if _, err := h([]byte(`{"user_id":"someone"}`), gateway); !errors.Is(err, ErrNostrNotOwner) {
		t.Fatalf("only the owner's key is served: %v", err)
	}
	denied := StreamGetNostrKeyHandler(stubNostrOwner{"owner"}, stubNostrUsers{domain.User{Id: "owner"}}, priv)
	if _, err := denied(body, gateway); !errors.Is(err, ErrNostrNotAllowed) {
		t.Fatalf("no consent, no key: %v", err)
	}
}

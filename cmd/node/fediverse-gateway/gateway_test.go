//nolint:all
package main

import (
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Warp-net/warpnet/json"
)

func testGateway(t *testing.T) *gateway {
	t.Helper()
	key, err := loadOrCreateKey(t.TempDir() + "/key.pem")
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	pub, err := publicKeyPEM(key)
	if err != nil {
		t.Fatalf("pub: %v", err)
	}
	return &gateway{
		host:        "gw.example",
		key:         key,
		keyPubPEM:   pub,
		source:      staticSource{user: warpnetUser{ID: "alice", PreferredUsername: "alice", DisplayName: "Alice"}},
		signingUser: "alice",
		client:      http.DefaultClient,
		sem:         make(chan struct{}, 4),
	}
}

func TestWebFinger(t *testing.T) {
	srv := httptest.NewServer(testGateway(t).routes())
	defer srv.Close()

	t.Run("known user", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/.well-known/webfinger?resource=acct:alice@gw.example")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		var jrd webFingerJRD
		if err := json.NewDecoder(resp.Body).Decode(&jrd); err != nil {
			t.Fatal(err)
		}
		if len(jrd.Links) != 1 || jrd.Links[0].Href != "https://gw.example/users/alice" {
			t.Fatalf("unexpected jrd: %+v", jrd)
		}
	})

	t.Run("unknown user", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/.well-known/webfinger?resource=acct:bob@gw.example")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})
}

func TestActorDocument(t *testing.T) {
	srv := httptest.NewServer(testGateway(t).routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/users/alice")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if ct := resp.Header.Get("Content-Type"); ct != contentTypeAP {
		t.Fatalf("content-type = %q", ct)
	}
	var a actor
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		t.Fatal(err)
	}
	if a.ID != "https://gw.example/users/alice" {
		t.Fatalf("id = %q", a.ID)
	}
	if a.Inbox != "https://gw.example/users/alice/inbox" {
		t.Fatalf("inbox = %q", a.Inbox)
	}
	if !strings.Contains(a.PublicKey.PublicKeyPEM, "BEGIN PUBLIC KEY") {
		t.Fatalf("actor is missing a public key PEM")
	}
}

func TestHTTPSignatureRoundTrip(t *testing.T) {
	g := testGateway(t)
	body := []byte(`{"type":"Follow","actor":"https://remote/users/bob"}`)

	req, err := http.NewRequest(http.MethodPost, "https://gw.example/users/alice/inbox", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	if err := signRequest(req, g.keyID("alice"), g.key, body); err != nil {
		t.Fatalf("sign: %v", err)
	}

	pubKey := func(string) (*rsa.PublicKey, error) { return &g.key.PublicKey, nil }

	if err := verifyRequest(req, body, pubKey); err != nil {
		t.Fatalf("verify: %v", err)
	}

	t.Run("tampered body fails", func(t *testing.T) {
		if err := verifyRequest(req, []byte(`{"type":"Undo"}`), pubKey); err == nil {
			t.Fatal("expected digest mismatch, got nil")
		}
	})
}

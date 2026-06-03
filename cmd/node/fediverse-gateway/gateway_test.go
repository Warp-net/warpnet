//nolint:all
package main

import (
	"context"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/domain"
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
	fs, err := newFollowerStore(t.TempDir() + "/followers.json")
	if err != nil {
		t.Fatalf("followers: %v", err)
	}
	return &gateway{
		host:        "gw.example",
		key:         key,
		keyPubPEM:   pub,
		source:      staticSource{user: warpnetUser{ID: "alice", PreferredUsername: "alice", DisplayName: "Alice"}},
		signingUser: "alice",
		client:      http.DefaultClient,
		sem:         make(chan struct{}, 4),
		followers:   fs,
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

func TestFollowerStore(t *testing.T) {
	path := t.TempDir() + "/f.json"
	s, err := newFollowerStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add("alice", follower{Actor: "https://m/users/bob", Inbox: "https://m/inbox"}); err != nil {
		t.Fatal(err)
	}
	// idempotent by actor URL
	if err := s.Add("alice", follower{Actor: "https://m/users/bob", Inbox: "https://m/inbox2"}); err != nil {
		t.Fatal(err)
	}
	if got := s.List("alice"); len(got) != 1 {
		t.Fatalf("want 1 follower, got %d", len(got))
	}
	// reload from disk sees the persisted follower
	s2, err := newFollowerStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.List("alice"); len(got) != 1 || got[0].Actor != "https://m/users/bob" {
		t.Fatalf("reloaded store mismatch: %+v", got)
	}
}

func TestBuildCreateNote(t *testing.T) {
	g := testGateway(t)
	a := g.buildCreateNote("alice", domain.Tweet{Id: "t1", Text: "hello <fedi>", CreatedAt: time.Unix(1700000000, 0)})
	if a.Type != "Create" || a.Actor != "https://gw.example/users/alice" {
		t.Fatalf("bad create: %+v", a)
	}
	n, ok := a.Object.(note)
	if !ok {
		t.Fatalf("object is not a note: %T", a.Object)
	}
	if n.ID != "https://gw.example/users/alice/statuses/t1" {
		t.Fatalf("bad note id: %s", n.ID)
	}
	if !strings.Contains(n.Content, "&lt;fedi&gt;") {
		t.Fatalf("content not html-escaped: %s", n.Content)
	}
	if len(n.To) == 0 || n.To[0] != asPublic {
		t.Fatalf("note not addressed to public: %+v", n.To)
	}
}

func TestPublishNoteFanout(t *testing.T) {
	g := testGateway(t)
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	g.client = srv.Client()

	_ = g.followers.Add("alice", follower{Actor: srv.URL + "/users/bob", Inbox: srv.URL + "/inbox/bob"})
	_ = g.followers.Add("alice", follower{Actor: srv.URL + "/users/carol", Inbox: srv.URL + "/inbox/carol"})

	g.publishNote(context.Background(), "alice", domain.Tweet{Id: "t1", Text: "hello fedi", CreatedAt: time.Now()})

	mu.Lock()
	defer mu.Unlock()
	if hits["/inbox/bob"] != 1 || hits["/inbox/carol"] != 1 {
		t.Fatalf("fanout mismatch: %+v", hits)
	}
}

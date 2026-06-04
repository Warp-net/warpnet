//nolint:all
package main

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
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
	fs, err := newFileFollowerStore(t.TempDir() + "/followers.json")
	if err != nil {
		t.Fatalf("followers: %v", err)
	}
	return &gateway{
		host:                "gw.example",
		key:                 key,
		keyPubPEM:           pub,
		source:              staticSource{user: warpnetUser{ID: "alice", PreferredUsername: "alice", DisplayName: "Alice"}},
		signingUser:         "alice",
		client:              http.DefaultClient,
		sem:                 make(chan struct{}, 4),
		followers:           fs,
		allowPrivateTargets: true,
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

func TestFileFollowerStore(t *testing.T) {
	path := t.TempDir() + "/f.json"
	s, err := newFileFollowerStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add("alice", "https://m/users/bob"); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("alice", "https://m/users/bob"); err != nil { // idempotent
		t.Fatal(err)
	}
	if got, _ := s.List("alice"); len(got) != 1 {
		t.Fatalf("want 1 follower, got %d", len(got))
	}
	// reload from disk sees the persisted follower
	s2, err := newFileFollowerStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s2.List("alice"); len(got) != 1 || got[0] != "https://m/users/bob" {
		t.Fatalf("reloaded store mismatch: %+v", got)
	}
}

func TestBuildCreateNote(t *testing.T) {
	g := testGateway(t)
	a := g.buildCreateNote("alice", domain.Tweet{Id: "t1", Text: "hello <fedi>", CreatedAt: time.Unix(1700000000, 0), UserId: "alice", ImageKeys: []string{"k1"}})
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
	if len(n.Attachment) != 1 || n.Attachment[0].Type != "Document" {
		t.Fatalf("attachment: %+v", n.Attachment)
	}
	if u, k, ok := decodeMediaRef(strings.TrimPrefix(n.Attachment[0].URL, "https://gw.example/media/")); !ok || u != "alice" || k != "k1" {
		t.Fatalf("attachment ref: %s", n.Attachment[0].URL)
	}
}

func TestPublishNoteFanout(t *testing.T) {
	g := testGateway(t)
	var mu sync.Mutex
	hits := map[string]int{}
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if name, ok := strings.CutPrefix(r.URL.Path, "/users/"); ok {
			// actor document carrying this follower's inbox
			writeJSON(w, contentTypeAP, map[string]any{
				"id":    srv.URL + "/users/" + name,
				"inbox": srv.URL + "/inbox/" + name,
			})
			return
		}
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	g.client = srv.Client()

	_ = g.followers.Add("alice", srv.URL+"/users/bob")
	_ = g.followers.Add("alice", srv.URL+"/users/carol")

	g.publishNote(context.Background(), "alice", domain.Tweet{Id: "t1", Text: "hello fedi", CreatedAt: time.Now()})

	mu.Lock()
	defer mu.Unlock()
	if hits["/inbox/bob"] != 1 || hits["/inbox/carol"] != 1 {
		t.Fatalf("fanout mismatch: %+v", hits)
	}
}

type fakeRequester struct {
	lastRoute     stream.WarpRoute
	lastPayload   any
	followersJSON []byte
	imageFile     string
}

func (f *fakeRequester) request(route stream.WarpRoute, payload any) ([]byte, error) {
	f.lastRoute = route
	f.lastPayload = payload
	switch route {
	case event.PUBLIC_GET_FOLLOWERS:
		return f.followersJSON, nil
	case event.PUBLIC_GET_IMAGE:
		bt, _ := json.Marshal(event.GetImageResponse{File: f.imageFile})
		return bt, nil
	}
	return []byte(`["accepted"]`), nil
}

func TestNodeFollowerStore(t *testing.T) {
	const actor = "https://mastodon.social/users/bob"
	fr := &fakeRequester{}
	s := nodeFollowerStore{req: fr}

	if err := s.Add("owner1", actor); err != nil {
		t.Fatal(err)
	}
	ev, ok := fr.lastPayload.(event.NewFollowEvent)
	if !ok {
		t.Fatalf("follow payload type %T", fr.lastPayload)
	}
	if ev.FollowingId != "owner1" {
		t.Fatalf("following id = %q", ev.FollowingId)
	}
	if got, _ := decodeActorID(ev.FollowerId); got != actor {
		t.Fatalf("follower id didn't round-trip: %q -> %q", ev.FollowerId, got)
	}

	// List decodes AP follower ids and skips native Warpnet ids.
	resp := event.FollowersResponse{Followers: []domain.ID{
		encodeActorID(actor),
		"01KSGHBHKG0N77T6A3RZV8WSH5", // native ULID — must be skipped
	}}
	fr.followersJSON, _ = json.Marshal(resp)

	urls, err := s.List("owner1")
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) != 1 || urls[0] != actor {
		t.Fatalf("list mismatch: %+v", urls)
	}
}

func strptr(s string) *string { return &s }

func TestPublishableTweet(t *testing.T) {
	cases := []struct {
		tw   domain.Tweet
		want bool
	}{
		{domain.Tweet{UserId: "alice"}, true},
		{domain.Tweet{UserId: "bob"}, false},                                 // not the owner
		{domain.Tweet{UserId: "alice", RetweetedBy: strptr("alice")}, false}, // retweet
		{domain.Tweet{UserId: "alice", ParentId: strptr("t0")}, false},       // reply
		{domain.Tweet{UserId: "alice", ParentId: strptr("")}, true},          // empty parent = top-level
	}
	for i, c := range cases {
		if got := publishableTweet(c.tw, "alice"); got != c.want {
			t.Errorf("case %d: got %v want %v", i, got, c.want)
		}
	}
}

type fakeTweetsRequester struct {
	tweets []domain.Tweet
}

func (f *fakeTweetsRequester) request(route stream.WarpRoute, _ any) ([]byte, error) {
	if route == event.PUBLIC_GET_TWEETS {
		bt, _ := json.Marshal(event.TweetsResponse{Tweets: f.tweets})
		return bt, nil
	}
	return []byte(`["accepted"]`), nil
}

func TestTweetPollerSeedAndDedup(t *testing.T) {
	fr := &fakeTweetsRequester{tweets: []domain.Tweet{{Id: "t1", UserId: "alice"}}}
	var published []string
	p := newTweetPoller(fr, "alice", func(_ context.Context, _ string, tw domain.Tweet) {
		published = append(published, tw.Id)
	})

	// seed marks existing tweets as seen (no history replay)
	for _, tw := range p.fetch() {
		p.seen[tw.Id] = struct{}{}
	}

	// a new tweet arrives → published once
	fr.tweets = append(fr.tweets, domain.Tweet{Id: "t2", UserId: "alice"})
	p.poll(context.Background())
	if len(published) != 1 || published[0] != "t2" {
		t.Fatalf("expected only t2 published, got %v", published)
	}

	// polling again republishes nothing
	published = nil
	p.poll(context.Background())
	if len(published) != 0 {
		t.Fatalf("expected no republish, got %v", published)
	}
}

func TestTranslateInbound(t *testing.T) {
	g := testGateway(t) // host gw.example
	actor := "https://m/users/bob"
	status := "https://gw.example/users/alice/statuses/t1"

	route, payload, ok := g.translateInbound(map[string]any{"type": "Like", "actor": actor, "object": status})
	if !ok || route != event.PUBLIC_POST_LIKE {
		t.Fatalf("like: route=%q ok=%v", route, ok)
	}
	like := payload.(event.LikeEvent)
	if like.TweetId != "t1" || like.OwnerId != "alice" {
		t.Fatalf("like event: %+v", like)
	}
	if got, _ := decodeActorID(like.UserId); got != actor {
		t.Fatalf("liker id round-trip: %q", like.UserId)
	}

	route, payload, ok = g.translateInbound(map[string]any{
		"type": "Create", "actor": actor,
		"object": map[string]any{"type": "Note", "content": "<p>hi there</p>", "inReplyTo": status},
	})
	if !ok || route != event.PUBLIC_POST_REPLY {
		t.Fatalf("reply: route=%q ok=%v", route, ok)
	}
	reply := payload.(event.NewReplyEvent)
	if reply.RootId != "t1" || reply.ParentId == nil || *reply.ParentId != "t1" || reply.Text != "hi there" {
		t.Fatalf("reply event: %+v", reply)
	}

	if route, _, ok := g.translateInbound(map[string]any{
		"type": "Undo", "actor": actor,
		"object": map[string]any{"type": "Follow", "object": "https://gw.example/users/alice"},
	}); !ok || route != event.PUBLIC_POST_UNFOLLOW {
		t.Fatalf("undo follow: route=%q ok=%v", route, ok)
	}

	if route, _, ok := g.translateInbound(map[string]any{
		"type": "Undo", "actor": actor,
		"object": map[string]any{"type": "Like", "object": status},
	}); !ok || route != event.PUBLIC_POST_UNLIKE {
		t.Fatalf("undo like: route=%q ok=%v", route, ok)
	}

	route, payload, ok = g.translateInbound(map[string]any{"type": "Announce", "actor": actor, "object": status})
	if !ok || route != event.PUBLIC_POST_RETWEET {
		t.Fatalf("announce: route=%q ok=%v", route, ok)
	}
	rt := payload.(event.NewRetweetEvent)
	if rt.Id != "t1" || rt.RetweetedBy == nil {
		t.Fatalf("retweet event: %+v", rt)
	}
	if got, _ := decodeActorID(*rt.RetweetedBy); got != actor {
		t.Fatalf("booster id round-trip: %q", *rt.RetweetedBy)
	}

	// Foreign-host objects and unhandled types are rejected.
	if _, _, ok := g.translateInbound(map[string]any{"type": "Like", "actor": actor, "object": "https://evil/users/x/statuses/9"}); ok {
		t.Fatal("foreign-host like should be unhandled")
	}
	if _, _, ok := g.translateInbound(map[string]any{"type": "Delete", "actor": actor, "object": status}); ok {
		t.Fatal("delete should be unhandled")
	}
}

func TestHandleMedia(t *testing.T) {
	g := testGateway(t)
	raw := []byte{0x89, 'P', 'N', 'G'}
	g.req = &fakeRequester{imageFile: "image/png," + base64.StdEncoding.EncodeToString(raw)}

	srv := httptest.NewServer(g.routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/media/" + encodeMediaRef("alice", "img1"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type = %q", ct)
	}
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, raw) {
		t.Fatalf("body mismatch: %v", got)
	}
}

func TestValidateRemoteURL(t *testing.T) {
	for _, u := range []string{
		"https://mastodon.social/users/x",
		"https://example.com/inbox",
	} {
		if err := validateRemoteURL(u); err != nil {
			t.Errorf("expected ok for %s, got %v", u, err)
		}
	}
	for _, u := range []string{
		"http://example.com/x",  // not https
		"https://127.0.0.1/x",   // loopback
		"https://localhost/x",   // localhost
		"https://10.0.0.1/x",    // private
		"https://169.254.1.1/x", // link-local
		"https://[::1]/x",       // ipv6 loopback
		"https:///x",            // no host
	} {
		if err := validateRemoteURL(u); err == nil {
			t.Errorf("expected error for %s", u)
		}
	}
}

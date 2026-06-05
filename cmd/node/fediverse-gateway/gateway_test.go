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
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
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
	a := g.buildCreateNote("alice", tweet{Id: "t1", Text: "hello <fedi>", CreatedAt: time.Unix(1700000000, 0), UserId: "alice", ImageKeys: []string{"k1"}})
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

	g.publishNote(context.Background(), "alice", tweet{Id: "t1", Text: "hello fedi", CreatedAt: time.Now()})

	mu.Lock()
	defer mu.Unlock()
	if hits["/inbox/bob"] != 1 || hits["/inbox/carol"] != 1 {
		t.Fatalf("fanout mismatch: %+v", hits)
	}
}

type fakeRequester struct {
	lastRoute      string
	lastPayload    any
	followersJSON  []byte
	followingsJSON []byte
	imageFile      string
	tweet          tweet
}

func (f *fakeRequester) request(route string, payload any) ([]byte, error) {
	f.lastRoute = route
	f.lastPayload = payload
	switch route {
	case routeGetFollowers:
		return f.followersJSON, nil
	case routeGetFollowings:
		return f.followingsJSON, nil
	case routeGetImage:
		bt, _ := json.Marshal(getImageResponse{File: f.imageFile})
		return bt, nil
	case routeGetTweet:
		bt, _ := json.Marshal(f.tweet)
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
	ev, ok := fr.lastPayload.(newFollowEvent)
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
	resp := followersResponse{Followers: []string{
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
		tw   tweet
		want bool
	}{
		{tweet{UserId: "alice"}, true},
		{tweet{UserId: "bob"}, false},                                 // not the owner
		{tweet{UserId: "alice", RetweetedBy: strptr("alice")}, false}, // retweet
		{tweet{UserId: "alice", ParentId: strptr("t0")}, false},       // reply
		{tweet{UserId: "alice", ParentId: strptr("")}, true},          // empty parent = top-level
	}
	for i, c := range cases {
		if got := publishableTweet(c.tw, "alice"); got != c.want {
			t.Errorf("case %d: got %v want %v", i, got, c.want)
		}
	}
}

type fakeTweetsRequester struct {
	tweets []tweet
}

func (f *fakeTweetsRequester) request(route string, _ any) ([]byte, error) {
	if route == routeGetTweets {
		bt, _ := json.Marshal(tweetsResponse{Tweets: f.tweets})
		return bt, nil
	}
	return []byte(`["accepted"]`), nil
}

func TestTweetPollerSeedAndDedup(t *testing.T) {
	fr := &fakeTweetsRequester{tweets: []tweet{{Id: "t1", UserId: "alice"}}}
	var published []string
	p := newTweetPoller(fr, "alice", func(_ context.Context, _ string, tw tweet) {
		published = append(published, tw.Id)
	})

	// seed marks existing tweets as seen (no history replay)
	for _, tw := range p.fetch() {
		p.seen[tw.Id] = struct{}{}
	}

	// a new tweet arrives → published once
	fr.tweets = append(fr.tweets, tweet{Id: "t2", UserId: "alice"})
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
	if !ok || route != routePostLike {
		t.Fatalf("like: route=%q ok=%v", route, ok)
	}
	like := payload.(likeEvent)
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
	if !ok || route != routePostReply {
		t.Fatalf("reply: route=%q ok=%v", route, ok)
	}
	reply := payload.(newReplyEvent)
	if reply.RootId != "t1" || reply.ParentId == nil || *reply.ParentId != "t1" || reply.Text != "hi there" {
		t.Fatalf("reply event: %+v", reply)
	}
	if reply.Username != "bob@m" {
		t.Fatalf("reply username = %q, want bob@m", reply.Username)
	}

	if route, _, ok := g.translateInbound(map[string]any{
		"type": "Undo", "actor": actor,
		"object": map[string]any{"type": "Follow", "object": "https://gw.example/users/alice"},
	}); !ok || route != routePostUnfollow {
		t.Fatalf("undo follow: route=%q ok=%v", route, ok)
	}

	if route, _, ok := g.translateInbound(map[string]any{
		"type": "Undo", "actor": actor,
		"object": map[string]any{"type": "Like", "object": status},
	}); !ok || route != routePostUnlike {
		t.Fatalf("undo like: route=%q ok=%v", route, ok)
	}

	if route, payload, ok := g.translateInbound(map[string]any{
		"type": "Undo", "actor": actor,
		"object": map[string]any{"type": "Announce", "object": status},
	}); !ok || route != routePostUnretweet {
		t.Fatalf("undo announce: route=%q ok=%v", route, ok)
	} else if ur := payload.(unretweetEvent); ur.TweetId != "t1" {
		t.Fatalf("unretweet event: %+v", ur)
	}

	route, payload, ok = g.translateInbound(map[string]any{"type": "Announce", "actor": actor, "object": status})
	if !ok || route != routePostRetweet {
		t.Fatalf("announce: route=%q ok=%v", route, ok)
	}
	rt := payload.(tweet)
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
		t.Fatal("delete should not translate to a node route")
	}
}

func TestHandleDeleteStub(t *testing.T) {
	g := testGateway(t)
	w := httptest.NewRecorder()
	g.handleDelete(w, map[string]any{
		"type": "Delete", "actor": "https://m/users/bob",
		"object": "https://m/users/bob/statuses/9",
	})
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
}

func TestHandleFromActorURL(t *testing.T) {
	cases := map[string]string{
		"https://mastodon.social/users/bob": "bob@mastodon.social",
		"https://example.com/@alice":        "alice@example.com",
		"https://example.com/users/carol/":  "carol@example.com",
		"justname":                          "justname",
	}
	for in, want := range cases {
		if got := handleFromActorURL(in); got != want {
			t.Errorf("handleFromActorURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSafeClientRedirect(t *testing.T) {
	c := newSafeClient(time.Second)
	if c.CheckRedirect == nil {
		t.Fatal("expected a CheckRedirect guard")
	}
	priv, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1/x", nil)
	if err := c.CheckRedirect(priv, nil); err == nil {
		t.Fatal("private redirect target should be rejected")
	}
	pub, _ := http.NewRequest(http.MethodGet, "https://example.com/y", nil)
	if err := c.CheckRedirect(pub, nil); err != nil {
		t.Fatalf("public https redirect should pass: %v", err)
	}
	if err := c.CheckRedirect(pub, make([]*http.Request, maxRedirects)); err == nil {
		t.Fatal("overlong redirect chain should be rejected")
	}
}

func TestIsBlockedIP(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "169.254.1.1", "::1", "0.0.0.0"} {
		if !isBlockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}
	for _, s := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if isBlockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
}

func TestSafeClientBlocksPrivateDial(t *testing.T) {
	// The dialer's Control must reject a connection to a loopback IP (DNS
	// rebinding defence) before any TCP connect happens.
	c := newSafeClient(2 * time.Second)
	if _, err := c.Get("http://127.0.0.1:9/"); err == nil {
		t.Fatal("dial to a loopback IP should be blocked")
	}
}

func TestFollowPollerDiff(t *testing.T) {
	fr := &fakeRequester{}
	var followed, unfollowed []string
	p := newFollowPoller(fr, "owner",
		func(a string) { followed = append(followed, a) },
		func(a string) { unfollowed = append(unfollowed, a) },
	)
	enc := func(urls ...string) []byte {
		ids := []string{"warpnet-native-id"} // non-ap following must be ignored
		for _, u := range urls {
			ids = append(ids, encodeActorID(u))
		}
		bt, _ := json.Marshal(followingsResponse{Followings: ids})
		return bt
	}

	fr.followingsJSON = enc("https://m/users/bob")
	if err := p.poll(); err != nil {
		t.Fatal(err)
	}
	if len(followed) != 0 || len(unfollowed) != 0 {
		t.Fatalf("baseline must fire nothing: f=%v u=%v", followed, unfollowed)
	}

	fr.followingsJSON = enc("https://m/users/carol")
	if err := p.poll(); err != nil {
		t.Fatal(err)
	}
	if len(followed) != 1 || followed[0] != "https://m/users/carol" {
		t.Fatalf("followed=%v", followed)
	}
	if len(unfollowed) != 1 || unfollowed[0] != "https://m/users/bob" {
		t.Fatalf("unfollowed=%v", unfollowed)
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

func TestServeStatus(t *testing.T) {
	g := testGateway(t)
	g.req = &fakeRequester{tweet: tweet{
		Id: "t1", UserId: "alice", Text: "hi <there>", CreatedAt: time.Unix(1700000000, 0),
	}}

	srv := httptest.NewServer(g.routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/users/alice/statuses/t1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != contentTypeAP {
		t.Fatalf("content-type = %q", ct)
	}
	var n note
	if err := json.NewDecoder(resp.Body).Decode(&n); err != nil {
		t.Fatal(err)
	}
	if n.ID != "https://gw.example/users/alice/statuses/t1" || n.Type != typeNote {
		t.Fatalf("note id/type: %+v", n)
	}
	if n.Context != asContext || !strings.Contains(n.Content, "&lt;there&gt;") {
		t.Fatalf("note context/content: %+v", n)
	}

	t.Run("missing tweet 404s", func(t *testing.T) {
		g2 := testGateway(t)
		g2.req = &fakeRequester{} // empty tweet (Id == "")
		srv2 := httptest.NewServer(g2.routes())
		defer srv2.Close()
		resp, err := http.Get(srv2.URL + "/users/alice/statuses/nope")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})
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

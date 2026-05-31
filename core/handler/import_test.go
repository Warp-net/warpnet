//nolint:all
package handler

import (
	"archive/zip"
	"bytes"
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

// ---- stubs ----

type stubImportInformer struct{ ownerId string }

func (s stubImportInformer) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{OwnerId: s.ownerId, Type: warpnet.MemberNode}
}

type stubImportUserRepo struct{ user domain.User }

func (s stubImportUserRepo) Get(userId string) (domain.User, error) {
	return s.user, nil
}

type stubImportTweetRepo struct {
	stored map[string]domain.Tweet
}

func newStubImportTweetRepo() *stubImportTweetRepo {
	return &stubImportTweetRepo{stored: map[string]domain.Tweet{}}
}

func (s *stubImportTweetRepo) Get(userID, tweetID string) (domain.Tweet, error) {
	t, ok := s.stored[tweetID]
	if !ok {
		return domain.Tweet{}, database.ErrTweetNotFound
	}
	return t, nil
}

func (s *stubImportTweetRepo) Create(_ string, tweet domain.Tweet) (domain.Tweet, error) {
	s.stored[tweet.Id] = tweet
	return tweet, nil
}

type stubImportMediaRepo struct{ saved int }

func (s *stubImportMediaRepo) GetImage(userId, key string) (database.Base64Image, error) {
	return "", nil
}
func (s *stubImportMediaRepo) SetImage(userId string, img database.Base64Image) (database.ImageKey, error) {
	s.saved++
	return database.ImageKey("imgkey"), nil
}
func (s *stubImportMediaRepo) SetForeignImageWithTTL(userId, key string, img database.Base64Image) error {
	return nil
}

// ---- helpers ----

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return b.Bytes()
}

func writeTempArchive(t *testing.T, files map[string][]byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "archive-*.zip")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return f.Name()
}

func marshalImport(t *testing.T, v any) []byte {
	t.Helper()
	bt, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bt
}

// tweetsJS is a representative window.YTD.tweets.part0 payload with one
// original photo tweet, one retweet, one reply and one animated-gif tweet.
const tweetsJS = `window.YTD.tweets.part0 = [
  {
    "tweet" : {
      "id_str" : "111",
      "created_at" : "Fri May 29 20:52:08 +0000 2026",
      "full_text" : "Hello &amp; welcome",
      "entities" : { "media" : [ { "type" : "photo", "media_url_https" : "https://pbs.twimg.com/media/ABC123.png" } ] },
      "extended_entities" : { "media" : [ { "type" : "photo", "media_url_https" : "https://pbs.twimg.com/media/ABC123.png" } ] }
    }
  },
  {
    "tweet" : {
      "id_str" : "222",
      "created_at" : "Fri May 29 20:53:08 +0000 2026",
      "full_text" : "RT @someone: a shared post"
    }
  },
  {
    "tweet" : {
      "id_str" : "333",
      "created_at" : "Fri May 29 20:54:08 +0000 2026",
      "full_text" : "@bob agreed",
      "in_reply_to_status_id_str" : "999"
    }
  },
  {
    "tweet" : {
      "id_str" : "444",
      "created_at" : "Fri May 29 20:55:08 +0000 2026",
      "full_text" : "look a gif",
      "entities" : { "media" : [ { "type" : "photo", "media_url_https" : "https://pbs.twimg.com/tweet_video_thumb/GIF1.jpg" } ] },
      "extended_entities" : { "media" : [ { "type" : "animated_gif", "media_url_https" : "https://pbs.twimg.com/tweet_video_thumb/GIF1.jpg" } ] }
    }
  }
]`

func newImportHandlerWithArchive(t *testing.T, files map[string][]byte) (warpnet.WarpHandlerFunc, *stubImportTweetRepo, *stubImportMediaRepo, string) {
	t.Helper()
	tweetRepo := newStubImportTweetRepo()
	mediaRepo := &stubImportMediaRepo{}
	informer := stubImportInformer{ownerId: "owner-1"}
	userRepo := stubImportUserRepo{user: domain.User{Id: "owner-1", Username: "alice"}}
	h := StreamImportTwitterArchiveHandler(informer, tweetRepo, mediaRepo, userRepo)
	return h, tweetRepo, mediaRepo, writeTempArchive(t, files)
}

func TestStreamImportTwitterArchiveHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		h, _, _, _ := newImportHandlerWithArchive(t, map[string][]byte{})
		if _, err := h([]byte("{"), nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty archive path", func(t *testing.T) {
		h, _, _, _ := newImportHandlerWithArchive(t, map[string][]byte{})
		_, err := h(marshalImport(t, event.ImportTwitterArchiveEvent{ArchivePath: ""}), nil)
		if err == nil {
			t.Fatal("expected error for empty path")
		}
	})

	t.Run("missing archive file", func(t *testing.T) {
		h, _, _, _ := newImportHandlerWithArchive(t, map[string][]byte{})
		_, err := h(marshalImport(t, event.ImportTwitterArchiveEvent{ArchivePath: "/no/such/archive.zip"}), nil)
		if err == nil {
			t.Fatal("expected error opening missing archive")
		}
	})

	t.Run("no tweets.js in archive", func(t *testing.T) {
		h, _, _, path := newImportHandlerWithArchive(t, map[string][]byte{
			"twitter-x/data/account.js": []byte("window.YTD.account.part0 = []"),
		})
		_, err := h(marshalImport(t, event.ImportTwitterArchiveEvent{ArchivePath: path}), nil)
		if err == nil {
			t.Fatal("expected error when tweets.js absent")
		}
	})

	t.Run("happy path: imports originals, skips retweets/replies, drops gifs", func(t *testing.T) {
		files := map[string][]byte{
			"twitter-x/data/tweets.js":                   []byte(tweetsJS),
			"twitter-x/data/tweets_media/111-ABC123.png": tinyPNG(t),
			"twitter-x/data/tweets_media/444-GIF1.jpg":   tinyPNG(t), // present but type is gif -> ignored
		}
		h, tweetRepo, mediaRepo, path := newImportHandlerWithArchive(t, files)

		out, err := h(marshalImport(t, event.ImportTwitterArchiveEvent{ArchivePath: path}), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resp, ok := out.(event.ImportTwitterArchiveResponse)
		if !ok {
			t.Fatalf("unexpected response type %T", out)
		}
		if resp.ImportedTweets != 2 {
			t.Fatalf("imported tweets = %d, want 2", resp.ImportedTweets)
		}
		if resp.ImportedImages != 1 {
			t.Fatalf("imported images = %d, want 1", resp.ImportedImages)
		}
		if resp.SkippedTweets != 2 {
			t.Fatalf("skipped tweets = %d, want 2", resp.SkippedTweets)
		}
		if mediaRepo.saved != 1 {
			t.Fatalf("media saved = %d, want 1 (gif must not be stored)", mediaRepo.saved)
		}

		// Photo tweet: HTML unescaped, original id + time preserved, image attached.
		tw111, err := tweetRepo.Get("owner-1", "111")
		if err != nil {
			t.Fatalf("tweet 111 not stored: %v", err)
		}
		if tw111.Text != "Hello & welcome" {
			t.Fatalf("tweet 111 text = %q, want %q", tw111.Text, "Hello & welcome")
		}
		if tw111.UserId != "owner-1" || tw111.Username != "alice" {
			t.Fatalf("tweet 111 author = %s/%s, want owner-1/alice", tw111.UserId, tw111.Username)
		}
		if tw111.CreatedAt.Year() != 2026 {
			t.Fatalf("tweet 111 created year = %d, want 2026", tw111.CreatedAt.Year())
		}
		if len(tw111.ImageKeys) != 1 {
			t.Fatalf("tweet 111 image keys = %d, want 1", len(tw111.ImageKeys))
		}

		// GIF tweet: imported as text-only (no images).
		tw444, err := tweetRepo.Get("owner-1", "444")
		if err != nil {
			t.Fatalf("tweet 444 not stored: %v", err)
		}
		if len(tw444.ImageKeys) != 0 {
			t.Fatalf("tweet 444 image keys = %d, want 0 (gif ignored)", len(tw444.ImageKeys))
		}

		// Retweet and reply must not be stored.
		if _, err := tweetRepo.Get("owner-1", "222"); err == nil {
			t.Fatal("retweet 222 should be skipped")
		}
		if _, err := tweetRepo.Get("owner-1", "333"); err == nil {
			t.Fatal("reply 333 should be skipped")
		}
	})

	t.Run("idempotent re-import skips everything", func(t *testing.T) {
		files := map[string][]byte{
			"twitter-x/data/tweets.js":                   []byte(tweetsJS),
			"twitter-x/data/tweets_media/111-ABC123.png": tinyPNG(t),
		}
		h, _, _, path := newImportHandlerWithArchive(t, files)

		if _, err := h(marshalImport(t, event.ImportTwitterArchiveEvent{ArchivePath: path}), nil); err != nil {
			t.Fatalf("first import error: %v", err)
		}
		out, err := h(marshalImport(t, event.ImportTwitterArchiveEvent{ArchivePath: path}), nil)
		if err != nil {
			t.Fatalf("second import error: %v", err)
		}
		resp := out.(event.ImportTwitterArchiveResponse)
		if resp.ImportedTweets != 0 {
			t.Fatalf("re-import imported = %d, want 0", resp.ImportedTweets)
		}
		if resp.SkippedTweets != 4 {
			t.Fatalf("re-import skipped = %d, want 4", resp.SkippedTweets)
		}
	})
}

func TestArchiveTweetClassification(t *testing.T) {
	t.Run("retweet detection", func(t *testing.T) {
		if !(archiveTweet{FullText: "RT @x: hi"}).isRetweet() {
			t.Fatal("expected RT @ to be a retweet")
		}
		if (archiveTweet{FullText: "great post RT @x"}).isRetweet() {
			t.Fatal("RT not at start is not a retweet")
		}
	})

	t.Run("reply detection", func(t *testing.T) {
		if !(archiveTweet{InReplyToStatusIDStr: "1"}).isReply() {
			t.Fatal("expected reply")
		}
		if (archiveTweet{}).isReply() {
			t.Fatal("no in_reply_to is not a reply")
		}
	})

	t.Run("photo media filters gifs and videos", func(t *testing.T) {
		at := archiveTweet{ExtendedEntities: archiveEntities{Media: []archiveMedia{
			{Type: "photo", MediaURLHTTPS: "https://x/media/a.jpg"},
			{Type: "animated_gif", MediaURLHTTPS: "https://x/tweet_video_thumb/b.jpg"},
			{Type: "video", MediaURLHTTPS: "https://x/media/c.jpg"},
		}}}
		photos := at.photoMedia()
		if len(photos) != 1 || photos[0].Type != "photo" {
			t.Fatalf("photoMedia = %+v, want exactly the photo", photos)
		}
	})

	t.Run("extended_entities wins over entities for type", func(t *testing.T) {
		// A gif is labelled "photo" in entities but "animated_gif" in
		// extended_entities; we must trust the latter and drop it.
		at := archiveTweet{
			Entities:         archiveEntities{Media: []archiveMedia{{Type: "photo", MediaURLHTTPS: "https://x/media/a.jpg"}}},
			ExtendedEntities: archiveEntities{Media: []archiveMedia{{Type: "animated_gif", MediaURLHTTPS: "https://x/tweet_video_thumb/a.jpg"}}},
		}
		if len(at.photoMedia()) != 0 {
			t.Fatal("gif disguised as photo in entities must be dropped")
		}
	})
}

func TestExtractTweetsArrayJSON(t *testing.T) {
	t.Run("strips assignment prefix", func(t *testing.T) {
		got, err := extractTweetsArrayJSON([]byte(`window.YTD.tweets.part0 = [ {"a":1} ]`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.HasPrefix(got, []byte("[")) {
			t.Fatalf("got %q, want JSON array", got)
		}
	})

	t.Run("no array", func(t *testing.T) {
		if _, err := extractTweetsArrayJSON([]byte("window.YTD.tweets.part0 = ")); err == nil {
			t.Fatal("expected error when no array present")
		}
	})
}

func TestIsTweetsFile(t *testing.T) {
	cases := map[string]bool{
		"x/data/tweets.js":          true,
		"x/data/tweets-part1.js":    true,
		"x/data/tweet-headers.js":   false,
		"x/data/deleted-tweets.js":  false,
		"x/data/note-tweet.js":      false,
		"x/data/community-tweet.js": false,
	}
	for name, want := range cases {
		if got := isTweetsFile(name); got != want {
			t.Fatalf("isTweetsFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestParseArchiveTime(t *testing.T) {
	got := parseArchiveTime("Fri May 29 20:52:08 +0000 2026")
	if got.IsZero() || got.Year() != 2026 || got.Month() != 5 || got.Day() != 29 {
		t.Fatalf("parseArchiveTime = %v, want 2026-05-29", got)
	}
	if !parseArchiveTime("nonsense").IsZero() {
		t.Fatal("expected zero time on parse failure")
	}
}

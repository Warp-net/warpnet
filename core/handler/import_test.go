//nolint:all
package handler

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
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

func marshalImport(t *testing.T, v any) []byte {
	t.Helper()
	bt, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bt
}

func newImportTweetHandler(t *testing.T) (warpnet.WarpHandlerFunc, *stubImportTweetRepo, *stubImportMediaRepo) {
	t.Helper()
	tweetRepo := newStubImportTweetRepo()
	mediaRepo := &stubImportMediaRepo{}
	informer := stubImportInformer{ownerId: "owner-1"}
	userRepo := stubImportUserRepo{user: domain.User{Id: "owner-1", Username: "alice"}}
	return StreamImportTweetHandler(informer, tweetRepo, mediaRepo, userRepo), tweetRepo, mediaRepo
}

func TestStreamImportTweetHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		h, _, _ := newImportTweetHandler(t)
		if _, err := h([]byte("{"), nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty id", func(t *testing.T) {
		h, _, _ := newImportTweetHandler(t)
		if _, err := h(marshalImport(t, event.ImportTweetEvent{Text: "hi"}), nil); err == nil {
			t.Fatal("expected error for empty id")
		}
	})

	t.Run("happy path: text unescaped, photo attached", func(t *testing.T) {
		h, tweetRepo, mediaRepo := newImportTweetHandler(t)
		photo := base64.StdEncoding.EncodeToString(tinyPNG(t))
		out, err := h(marshalImport(t, event.ImportTweetEvent{
			Id:        "111",
			Text:      "Hello &amp; welcome",
			CreatedAt: "Fri May 29 20:52:08 +0000 2026",
			Images:    []string{photo},
		}), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resp, ok := out.(event.ImportTwitterArchiveResponse)
		if !ok {
			t.Fatalf("unexpected response type %T", out)
		}
		if resp.ImportedTweets != 1 || resp.ImportedImages != 1 {
			t.Fatalf("resp = %+v, want 1 tweet / 1 image", resp)
		}
		tw, err := tweetRepo.Get("owner-1", "111")
		if err != nil {
			t.Fatalf("tweet 111 not stored: %v", err)
		}
		if tw.Text != "Hello & welcome" {
			t.Fatalf("text = %q, want unescaped", tw.Text)
		}
		if tw.UserId != "owner-1" || tw.Username != "alice" {
			t.Fatalf("author = %s/%s, want owner-1/alice", tw.UserId, tw.Username)
		}
		if tw.CreatedAt.Year() != 2026 {
			t.Fatalf("created year = %d, want 2026", tw.CreatedAt.Year())
		}
		if len(tw.ImageKeys) != 1 {
			t.Fatalf("image keys = %d, want 1", len(tw.ImageKeys))
		}
		if mediaRepo.saved != 1 {
			t.Fatalf("media saved = %d, want 1", mediaRepo.saved)
		}
	})

	t.Run("text-only tweet imports", func(t *testing.T) {
		h, tweetRepo, _ := newImportTweetHandler(t)
		out, err := h(marshalImport(t, event.ImportTweetEvent{Id: "555", Text: "just text"}), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.(event.ImportTwitterArchiveResponse).ImportedTweets != 1 {
			t.Fatal("text-only tweet should import")
		}
		if _, err := tweetRepo.Get("owner-1", "555"); err != nil {
			t.Fatalf("tweet 555 not stored: %v", err)
		}
	})

	t.Run("empty text and no images is skipped", func(t *testing.T) {
		h, tweetRepo, _ := newImportTweetHandler(t)
		out, err := h(marshalImport(t, event.ImportTweetEvent{Id: "666"}), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resp := out.(event.ImportTwitterArchiveResponse)
		if resp.ImportedTweets != 0 || resp.SkippedTweets != 1 {
			t.Fatalf("resp = %+v, want 0 imported / 1 skipped", resp)
		}
		if len(tweetRepo.stored) != 0 {
			t.Fatalf("stored %d tweets, want 0", len(tweetRepo.stored))
		}
	})

	t.Run("caps at four photos", func(t *testing.T) {
		h, tweetRepo, mediaRepo := newImportTweetHandler(t)
		photo := base64.StdEncoding.EncodeToString(tinyPNG(t))
		out, err := h(marshalImport(t, event.ImportTweetEvent{
			Id:     "777",
			Text:   "many",
			Images: []string{photo, photo, photo, photo, photo, photo},
		}), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.(event.ImportTwitterArchiveResponse).ImportedImages != 4 {
			t.Fatalf("imported images = %d, want 4 (capped)", out.(event.ImportTwitterArchiveResponse).ImportedImages)
		}
		if mediaRepo.saved != 4 {
			t.Fatalf("media saved = %d, want 4 (capped)", mediaRepo.saved)
		}
		tw, _ := tweetRepo.Get("owner-1", "777")
		if len(tw.ImageKeys) != 4 {
			t.Fatalf("image keys = %d, want 4 (capped)", len(tw.ImageKeys))
		}
	})
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

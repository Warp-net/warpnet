//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/event"
)

type stubMediaMetaRepo struct {
	setFn func(userId, key string, meta database.MediaMeta) error
	getFn func(userId, key string) (database.MediaMeta, error)
}

func (s stubMediaMetaRepo) SetImageMeta(userId, key string, meta database.MediaMeta) error {
	if s.setFn != nil {
		return s.setFn(userId, key, meta)
	}
	return nil
}

func (s stubMediaMetaRepo) GetImageMeta(userId, key string) (database.MediaMeta, error) {
	if s.getFn != nil {
		return s.getFn(userId, key)
	}
	return database.MediaMeta{}, nil
}

func TestStreamUpdateMediaMetaHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamUpdateMediaMetaHandler(stubMediaMetaRepo{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user id", func(t *testing.T) {
		_, err := StreamUpdateMediaMetaHandler(stubMediaMetaRepo{})(marshal(t, event.UpdateMediaMetaEvent{Key: "k"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty key", func(t *testing.T) {
		_, err := StreamUpdateMediaMetaHandler(stubMediaMetaRepo{})(marshal(t, event.UpdateMediaMetaEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("boom")
		_, err := StreamUpdateMediaMetaHandler(stubMediaMetaRepo{setFn: func(_, _ string, _ database.MediaMeta) error {
			return repoErr
		}})(marshal(t, event.UpdateMediaMetaEvent{UserId: "u", Key: "k"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
	t.Run("happy path", func(t *testing.T) {
		var got database.MediaMeta
		resp, err := StreamUpdateMediaMetaHandler(stubMediaMetaRepo{setFn: func(_, _ string, m database.MediaMeta) error {
			got = m
			return nil
		}})(marshal(t, event.UpdateMediaMetaEvent{
			UserId:      "u",
			Key:         "k",
			Description: "alt text",
			FocusX:      0.5,
			FocusY:      -0.25,
		}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
		if got.Description != "alt text" || got.FocusX != 0.5 || got.FocusY != -0.25 {
			t.Fatalf("repo got wrong meta: %+v", got)
		}
	})
}

func TestStreamGetMediaHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamGetMediaHandler(stubMediaMetaRepo{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user id", func(t *testing.T) {
		_, err := StreamGetMediaHandler(stubMediaMetaRepo{})(marshal(t, event.GetMediaEvent{Key: "k"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty key", func(t *testing.T) {
		_, err := StreamGetMediaHandler(stubMediaMetaRepo{})(marshal(t, event.GetMediaEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamGetMediaHandler(stubMediaMetaRepo{getFn: func(_, _ string) (database.MediaMeta, error) {
			return database.MediaMeta{Description: "alt", FocusX: 0.1, FocusY: 0.2}, nil
		}})(marshal(t, event.GetMediaEvent{UserId: "u", Key: "k"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		r := resp.(event.GetMediaResponse)
		if r.Description != "alt" || r.FocusX != 0.1 || r.FocusY != 0.2 || r.Key != "k" {
			t.Fatalf("bad response: %+v", r)
		}
	})
}

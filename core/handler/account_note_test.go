//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/event"
)

type stubAccountNoteRepo struct {
	setFn func(selfId, targetId, note string) error
	getFn func(selfId, targetId string) (string, error)
}

func (s stubAccountNoteRepo) SetNote(selfId, targetId, note string) error {
	if s.setFn != nil {
		return s.setFn(selfId, targetId, note)
	}
	return nil
}

func (s stubAccountNoteRepo) GetNote(selfId, targetId string) (string, error) {
	if s.getFn != nil {
		return s.getFn(selfId, targetId)
	}
	return "", nil
}

func TestStreamUpdateAccountNoteHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamUpdateAccountNoteHandler(stubAccountNoteRepo{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty self id", func(t *testing.T) {
		_, err := StreamUpdateAccountNoteHandler(stubAccountNoteRepo{})(marshal(t, event.UpdateAccountNoteEvent{TargetUserId: "x", Note: "a"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty target id", func(t *testing.T) {
		_, err := StreamUpdateAccountNoteHandler(stubAccountNoteRepo{})(marshal(t, event.UpdateAccountNoteEvent{SelfId: "x", Note: "a"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty note clears", func(t *testing.T) {
		var captured string
		resp, err := StreamUpdateAccountNoteHandler(stubAccountNoteRepo{setFn: func(_, _, note string) error {
			captured = note
			return nil
		}})(marshal(t, event.UpdateAccountNoteEvent{SelfId: "a", TargetUserId: "b", Note: ""}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
		if captured != "" {
			t.Fatalf("expected empty note, got %q", captured)
		}
	})
	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("boom")
		_, err := StreamUpdateAccountNoteHandler(stubAccountNoteRepo{setFn: func(_, _, _ string) error {
			return repoErr
		}})(marshal(t, event.UpdateAccountNoteEvent{SelfId: "a", TargetUserId: "b", Note: "x"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
	t.Run("happy path", func(t *testing.T) {
		var gotSelf, gotTarget, gotNote string
		resp, err := StreamUpdateAccountNoteHandler(stubAccountNoteRepo{setFn: func(s, target, note string) error {
			gotSelf, gotTarget, gotNote = s, target, note
			return nil
		}})(marshal(t, event.UpdateAccountNoteEvent{SelfId: "a", TargetUserId: "b", Note: "knows go"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
		if gotSelf != "a" || gotTarget != "b" || gotNote != "knows go" {
			t.Fatalf("bad repo args: %s/%s/%q", gotSelf, gotTarget, gotNote)
		}
	})
}

func TestStreamGetAccountNoteHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamGetAccountNoteHandler(stubAccountNoteRepo{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty self id", func(t *testing.T) {
		_, err := StreamGetAccountNoteHandler(stubAccountNoteRepo{})(marshal(t, event.GetAccountNoteEvent{TargetUserId: "x"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty target id", func(t *testing.T) {
		_, err := StreamGetAccountNoteHandler(stubAccountNoteRepo{})(marshal(t, event.GetAccountNoteEvent{SelfId: "x"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamGetAccountNoteHandler(stubAccountNoteRepo{getFn: func(_, _ string) (string, error) {
			return "private note", nil
		}})(marshal(t, event.GetAccountNoteEvent{SelfId: "a", TargetUserId: "b"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		r := resp.(event.GetAccountNoteResponse)
		if r.Note != "private note" {
			t.Fatalf("expected note, got %q", r.Note)
		}
	})
}

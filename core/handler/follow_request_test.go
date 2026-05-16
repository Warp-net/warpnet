//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/event"
)

type stubFollowReqRepo struct {
	addFn    func(targetUserId, followerId string) error
	removeFn func(targetUserId, followerId string) error
	listFn   func(targetUserId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubFollowReqRepo) Add(t, f string) error {
	if s.addFn != nil {
		return s.addFn(t, f)
	}
	return nil
}

func (s stubFollowReqRepo) Remove(t, f string) error {
	if s.removeFn != nil {
		return s.removeFn(t, f)
	}
	return nil
}

func (s stubFollowReqRepo) List(t string, l *uint64, c *string) ([]string, string, error) {
	if s.listFn != nil {
		return s.listFn(t, l, c)
	}
	return nil, "end", nil
}

type stubFollowGranter struct {
	followFn func(followerId, followingId string) error
	captured [2]string
}

func (s *stubFollowGranter) Follow(followerId, followingId string) error {
	s.captured = [2]string{followerId, followingId}
	if s.followFn != nil {
		return s.followFn(followerId, followingId)
	}
	return nil
}

func TestStreamGetFollowRequestsHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamGetFollowRequestsHandler(stubFollowReqRepo{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user id", func(t *testing.T) {
		_, err := StreamGetFollowRequestsHandler(stubFollowReqRepo{})(marshal(t, event.GetFollowRequestsEvent{}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamGetFollowRequestsHandler(stubFollowReqRepo{listFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
			return []string{"f1", "f2"}, "end", nil
		}})(marshal(t, event.GetFollowRequestsEvent{UserId: "u"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		r := resp.(event.GetFollowRequestsResponse)
		if len(r.FollowerIds) != 2 {
			t.Fatalf("expected 2 ids, got %d", len(r.FollowerIds))
		}
		if r.Cursor != "end" {
			t.Fatalf("expected end cursor, got %s", r.Cursor)
		}
	})
}

func TestStreamAuthorizeFollowRequestHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamAuthorizeFollowRequestHandler(stubFollowReqRepo{}, &stubFollowGranter{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user id", func(t *testing.T) {
		_, err := StreamAuthorizeFollowRequestHandler(stubFollowReqRepo{}, &stubFollowGranter{})(marshal(t, event.FollowRequestActionEvent{FollowerId: "f"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty follower id", func(t *testing.T) {
		_, err := StreamAuthorizeFollowRequestHandler(stubFollowReqRepo{}, &stubFollowGranter{})(marshal(t, event.FollowRequestActionEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("follow error", func(t *testing.T) {
		followErr := errors.New("boom")
		_, err := StreamAuthorizeFollowRequestHandler(stubFollowReqRepo{}, &stubFollowGranter{followFn: func(_, _ string) error {
			return followErr
		}})(marshal(t, event.FollowRequestActionEvent{UserId: "u", FollowerId: "f"}), nil)
		if !errors.Is(err, followErr) {
			t.Fatalf("expected follow error: %v", err)
		}
	})
	t.Run("happy path", func(t *testing.T) {
		var removed [2]string
		fg := &stubFollowGranter{}
		repo := stubFollowReqRepo{removeFn: func(u, f string) error {
			removed = [2]string{u, f}
			return nil
		}}
		resp, err := StreamAuthorizeFollowRequestHandler(repo, fg)(marshal(t, event.FollowRequestActionEvent{UserId: "u", FollowerId: "f"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
		if fg.captured != [2]string{"f", "u"} {
			t.Fatalf("expected Follow(follower=f, following=u), got %v", fg.captured)
		}
		if removed != [2]string{"u", "f"} {
			t.Fatalf("expected request removed, got %v", removed)
		}
	})
}

func TestStreamRejectFollowRequestHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamRejectFollowRequestHandler(stubFollowReqRepo{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user id", func(t *testing.T) {
		_, err := StreamRejectFollowRequestHandler(stubFollowReqRepo{})(marshal(t, event.FollowRequestActionEvent{FollowerId: "f"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty follower id", func(t *testing.T) {
		_, err := StreamRejectFollowRequestHandler(stubFollowReqRepo{})(marshal(t, event.FollowRequestActionEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		var removed [2]string
		repo := stubFollowReqRepo{removeFn: func(u, f string) error {
			removed = [2]string{u, f}
			return nil
		}}
		resp, err := StreamRejectFollowRequestHandler(repo)(marshal(t, event.FollowRequestActionEvent{UserId: "u", FollowerId: "f"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
		if removed != [2]string{"u", "f"} {
			t.Fatalf("expected request removed, got %v", removed)
		}
	})
}

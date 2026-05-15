//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubBlockUserResolver struct {
	getFn func(userId string) (domain.User, error)
}

func (s stubBlockUserResolver) Get(userId string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return domain.User{Id: userId, NodeId: "node-" + userId}, nil
}

type stubPeerBlocklister struct {
	blocklistFn func(peerId string) error
	captured    []string
}

func (s *stubPeerBlocklister) Blocklist(peerId string) error {
	s.captured = append(s.captured, peerId)
	if s.blocklistFn != nil {
		return s.blocklistFn(peerId)
	}
	return nil
}

type stubUserSetRepo struct {
	addFn    func(ownerId, targetId string) error
	removeFn func(ownerId, targetId string) error
	listFn   func(ownerId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubUserSetRepo) Add(ownerId, targetId string) error {
	if s.addFn != nil {
		return s.addFn(ownerId, targetId)
	}
	return nil
}

func (s stubUserSetRepo) Remove(ownerId, targetId string) error {
	if s.removeFn != nil {
		return s.removeFn(ownerId, targetId)
	}
	return nil
}

func (s stubUserSetRepo) List(ownerId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.listFn != nil {
		return s.listFn(ownerId, limit, cursor)
	}
	return nil, "end", nil
}

type stubConvMuteRepo struct {
	muteFn   func(userId, tweetId string) error
	unmuteFn func(userId, tweetId string) error
}

func (s stubConvMuteRepo) Mute(userId, tweetId string) error {
	if s.muteFn != nil {
		return s.muteFn(userId, tweetId)
	}
	return nil
}

func (s stubConvMuteRepo) Unmute(userId, tweetId string) error {
	if s.unmuteFn != nil {
		return s.unmuteFn(userId, tweetId)
	}
	return nil
}

func TestStreamBlockHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamBlockHandler(stubUserSetRepo{}, stubBlockUserResolver{}, &stubPeerBlocklister{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty blocker", func(t *testing.T) {
		_, err := StreamBlockHandler(stubUserSetRepo{}, stubBlockUserResolver{}, &stubPeerBlocklister{})(marshal(t, event.BlockEvent{BlockeeId: "b"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty blockee", func(t *testing.T) {
		_, err := StreamBlockHandler(stubUserSetRepo{}, stubBlockUserResolver{}, &stubPeerBlocklister{})(marshal(t, event.BlockEvent{BlockerId: "a"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("self block", func(t *testing.T) {
		_, err := StreamBlockHandler(stubUserSetRepo{}, stubBlockUserResolver{}, &stubPeerBlocklister{})(marshal(t, event.BlockEvent{BlockerId: "a", BlockeeId: "a"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("boom")
		_, err := StreamBlockHandler(stubUserSetRepo{addFn: func(_, _ string) error { return repoErr }}, stubBlockUserResolver{}, &stubPeerBlocklister{})(marshal(t, event.BlockEvent{BlockerId: "a", BlockeeId: "b"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
	t.Run("happy path escalates to peer blocklist", func(t *testing.T) {
		var gotOwner, gotTarget string
		userResolver := stubBlockUserResolver{getFn: func(uid string) (domain.User, error) {
			return domain.User{Id: uid, NodeId: "node-b"}, nil
		}}
		peerBl := &stubPeerBlocklister{}
		resp, err := StreamBlockHandler(stubUserSetRepo{addFn: func(o, tg string) error {
			gotOwner = o
			gotTarget = tg
			return nil
		}}, userResolver, peerBl)(marshal(t, event.BlockEvent{BlockerId: "a", BlockeeId: "b"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
		if gotOwner != "a" || gotTarget != "b" {
			t.Fatalf("bad repo args: %s/%s", gotOwner, gotTarget)
		}
		if len(peerBl.captured) != 1 || peerBl.captured[0] != "node-b" {
			t.Fatalf("expected peer block for node-b, got %v", peerBl.captured)
		}
	})
}

func TestStreamUnblockHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamUnblockHandler(stubUserSetRepo{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty blocker", func(t *testing.T) {
		_, err := StreamUnblockHandler(stubUserSetRepo{})(marshal(t, event.BlockEvent{BlockeeId: "b"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamUnblockHandler(stubUserSetRepo{})(marshal(t, event.UnblockEvent{BlockerId: "a", BlockeeId: "b"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
	})
}

func TestStreamGetBlocksHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamGetBlocksHandler(stubUserSetRepo{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user id", func(t *testing.T) {
		_, err := StreamGetBlocksHandler(stubUserSetRepo{})(marshal(t, event.GetBlocksEvent{}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamGetBlocksHandler(stubUserSetRepo{listFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
			return []string{"x", "y"}, "end", nil
		}})(marshal(t, event.GetBlocksEvent{UserId: "a"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		r := resp.(event.GetBlocksResponse)
		if len(r.Ids) != 2 {
			t.Fatalf("expected 2 ids, got %d", len(r.Ids))
		}
		if r.Cursor != "end" {
			t.Fatalf("expected end cursor, got %s", r.Cursor)
		}
	})
}

func TestStreamMuteHandler(t *testing.T) {
	t.Run("empty muter", func(t *testing.T) {
		_, err := StreamMuteHandler(stubUserSetRepo{})(marshal(t, event.MuteEvent{MuteeId: "b"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("self mute", func(t *testing.T) {
		_, err := StreamMuteHandler(stubUserSetRepo{})(marshal(t, event.MuteEvent{MuterId: "a", MuteeId: "a"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamMuteHandler(stubUserSetRepo{})(marshal(t, event.MuteEvent{MuterId: "a", MuteeId: "b"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
	})
}

func TestStreamUnmuteHandler(t *testing.T) {
	t.Run("empty muter", func(t *testing.T) {
		_, err := StreamUnmuteHandler(stubUserSetRepo{})(marshal(t, event.MuteEvent{MuteeId: "b"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamUnmuteHandler(stubUserSetRepo{})(marshal(t, event.UnmuteEvent{MuterId: "a", MuteeId: "b"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
	})
}

func TestStreamMuteConversationHandler(t *testing.T) {
	t.Run("empty user id", func(t *testing.T) {
		_, err := StreamMuteConversationHandler(stubConvMuteRepo{})(marshal(t, event.MuteConversationEvent{TweetId: "t"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty tweet id", func(t *testing.T) {
		_, err := StreamMuteConversationHandler(stubConvMuteRepo{})(marshal(t, event.MuteConversationEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamMuteConversationHandler(stubConvMuteRepo{})(marshal(t, event.MuteConversationEvent{UserId: "u", TweetId: "t"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
	})
}

func TestStreamUnmuteConversationHandler(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamUnmuteConversationHandler(stubConvMuteRepo{})(marshal(t, event.UnmuteConversationEvent{UserId: "u", TweetId: "t"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
	})
}

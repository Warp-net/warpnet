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
	removed     []string
}

func (s *stubPeerBlocklister) BlocklistPermanent(peerId string) error {
	s.captured = append(s.captured, peerId)
	if s.blocklistFn != nil {
		return s.blocklistFn(peerId)
	}
	return nil
}

func (s *stubPeerBlocklister) BlocklistRemove(peerId string) error {
	s.removed = append(s.removed, peerId)
	return nil
}

type stubBlocksRepo struct {
	blockFn   func(blockerId, blockeeId string) error
	unblockFn func(blockerId, blockeeId string) error
	listFn    func(blockerId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubBlocksRepo) Block(b, e string) error {
	if s.blockFn != nil {
		return s.blockFn(b, e)
	}
	return nil
}

func (s stubBlocksRepo) Unblock(b, e string) error {
	if s.unblockFn != nil {
		return s.unblockFn(b, e)
	}
	return nil
}

func (s stubBlocksRepo) List(b string, l *uint64, c *string) ([]string, string, error) {
	if s.listFn != nil {
		return s.listFn(b, l, c)
	}
	return nil, "end", nil
}

type stubMutesRepo struct {
	muteFn   func(muterId, muteeId string) error
	unmuteFn func(muterId, muteeId string) error
	listFn   func(muterId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubMutesRepo) Mute(m, e string) error {
	if s.muteFn != nil {
		return s.muteFn(m, e)
	}
	return nil
}

func (s stubMutesRepo) Unmute(m, e string) error {
	if s.unmuteFn != nil {
		return s.unmuteFn(m, e)
	}
	return nil
}

func (s stubMutesRepo) List(m string, l *uint64, c *string) ([]string, string, error) {
	if s.listFn != nil {
		return s.listFn(m, l, c)
	}
	return nil, "end", nil
}

func TestStreamBlockHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamBlockHandler(stubBlocksRepo{}, stubBlockUserResolver{}, &stubPeerBlocklister{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty blocker", func(t *testing.T) {
		_, err := StreamBlockHandler(stubBlocksRepo{}, stubBlockUserResolver{}, &stubPeerBlocklister{})(marshal(t, event.BlockEvent{BlockeeId: "b"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty blockee", func(t *testing.T) {
		_, err := StreamBlockHandler(stubBlocksRepo{}, stubBlockUserResolver{}, &stubPeerBlocklister{})(marshal(t, event.BlockEvent{BlockerId: "a"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("self block", func(t *testing.T) {
		_, err := StreamBlockHandler(stubBlocksRepo{}, stubBlockUserResolver{}, &stubPeerBlocklister{})(marshal(t, event.BlockEvent{BlockerId: "a", BlockeeId: "a"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("boom")
		_, err := StreamBlockHandler(stubBlocksRepo{blockFn: func(_, _ string) error { return repoErr }}, stubBlockUserResolver{}, &stubPeerBlocklister{})(marshal(t, event.BlockEvent{BlockerId: "a", BlockeeId: "b"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
	t.Run("happy path escalates to peer blocklist", func(t *testing.T) {
		var gotBlocker, gotBlockee string
		userResolver := stubBlockUserResolver{getFn: func(uid string) (domain.User, error) {
			return domain.User{Id: uid, NodeId: "node-b"}, nil
		}}
		peerBl := &stubPeerBlocklister{}
		resp, err := StreamBlockHandler(stubBlocksRepo{blockFn: func(blocker, blockee string) error {
			gotBlocker = blocker
			gotBlockee = blockee
			return nil
		}}, userResolver, peerBl)(marshal(t, event.BlockEvent{BlockerId: "a", BlockeeId: "b"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
		if gotBlocker != "a" || gotBlockee != "b" {
			t.Fatalf("bad repo args: %s/%s", gotBlocker, gotBlockee)
		}
		if len(peerBl.captured) != 1 || peerBl.captured[0] != "node-b" {
			t.Fatalf("expected peer block for node-b, got %v", peerBl.captured)
		}
	})
}

func TestStreamUnblockHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamUnblockHandler(stubBlocksRepo{}, stubBlockUserResolver{}, &stubPeerBlocklister{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty blocker", func(t *testing.T) {
		_, err := StreamUnblockHandler(stubBlocksRepo{}, stubBlockUserResolver{}, &stubPeerBlocklister{})(marshal(t, event.BlockEvent{BlockeeId: "b"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path removes peer blocklist", func(t *testing.T) {
		peerBl := &stubPeerBlocklister{}
		resp, err := StreamUnblockHandler(
			stubBlocksRepo{},
			stubBlockUserResolver{getFn: func(uid string) (domain.User, error) {
				return domain.User{Id: uid, NodeId: "node-b"}, nil
			}},
			peerBl,
		)(marshal(t, event.UnblockEvent{BlockerId: "a", BlockeeId: "b"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
		if len(peerBl.removed) != 1 || peerBl.removed[0] != "node-b" {
			t.Fatalf("expected peer unblock for node-b, got %v", peerBl.removed)
		}
	})
}

func TestStreamGetBlocksHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamGetBlocksHandler(stubBlocksRepo{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user id", func(t *testing.T) {
		_, err := StreamGetBlocksHandler(stubBlocksRepo{})(marshal(t, event.GetBlocksEvent{}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamGetBlocksHandler(stubBlocksRepo{listFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
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
		_, err := StreamMuteHandler(stubMutesRepo{})(marshal(t, event.MuteEvent{MuteeId: "b"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("self mute", func(t *testing.T) {
		_, err := StreamMuteHandler(stubMutesRepo{})(marshal(t, event.MuteEvent{MuterId: "a", MuteeId: "a"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamMuteHandler(stubMutesRepo{})(marshal(t, event.MuteEvent{MuterId: "a", MuteeId: "b"}), nil)
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
		_, err := StreamUnmuteHandler(stubMutesRepo{})(marshal(t, event.MuteEvent{MuteeId: "b"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamUnmuteHandler(stubMutesRepo{})(marshal(t, event.UnmuteEvent{MuterId: "a", MuteeId: "b"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
	})
}

//nolint:all
package handler

import (
	"errors"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

type stubReplyRepo struct {
	getReplyFn       func(rootID, replyID string) (domain.Tweet, error)
	getRepliesTreeFn func(rootID, parentId string, limit *uint64, cursor *string) ([]domain.ReplyNode, string, error)
	addReplyFn       func(reply domain.Tweet) (domain.Tweet, error)
	deleteReplyFn    func(rootID, parentID, replyID string) error
}

func (s stubReplyRepo) GetReply(rootID, replyID string) (domain.Tweet, error) {
	if s.getReplyFn != nil {
		return s.getReplyFn(rootID, replyID)
	}
	return domain.Tweet{Id: replyID, RootId: rootID}, nil
}
func (s stubReplyRepo) GetRepliesTree(rootID, parentId string, limit *uint64, cursor *string) ([]domain.ReplyNode, string, error) {
	if s.getRepliesTreeFn != nil {
		return s.getRepliesTreeFn(rootID, parentId, limit, cursor)
	}
	return nil, "", nil
}
func (s stubReplyRepo) AddReply(reply domain.Tweet) (domain.Tweet, error) {
	if s.addReplyFn != nil {
		return s.addReplyFn(reply)
	}
	if reply.Id == "" {
		reply.Id = "reply-1"
	}
	return reply, nil
}
func (s stubReplyRepo) DeleteReply(rootID, parentID, replyID string) error {
	if s.deleteReplyFn != nil {
		return s.deleteReplyFn(rootID, parentID, replyID)
	}
	return nil
}

type stubReplyUserRepo struct {
	getFn func(userId string) (domain.User, error)
}

func (s stubReplyUserRepo) Get(userId string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return domain.User{Id: userId, NodeId: "node-2"}, nil
}

type stubReplyTweetRepo struct {
	getFn func(userID, tweetID string) (domain.Tweet, error)
}

func (s stubReplyTweetRepo) Get(userID, tweetID string) (domain.Tweet, error) {
	if s.getFn != nil {
		return s.getFn(userID, tweetID)
	}
	return domain.Tweet{Id: tweetID, UserId: userID}, nil
}

func TestStreamNewReplyHandler(t *testing.T) {
	owner := "owner-1"
	parentUser := "parent-user"
	rootId := "root-1"
	parentId := "parent-1"

	makeEvent := func() event.NewReplyEvent {
		return event.NewReplyEvent{
			CreatedAt:    time.Now(),
			Id:           "reply-1",
			ParentId:     &parentId,
			ParentUserId: parentUser,
			RootId:       rootId,
			Text:         "a reply",
			UserId:       owner,
			Username:     "testuser",
		}
	}

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamNewReplyHandler(stubReplyRepo{}, stubReplyUserRepo{}, stubStreamer{}, stubModerationNotifier{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty text", func(t *testing.T) {
		h := StreamNewReplyHandler(stubReplyRepo{}, stubReplyUserRepo{}, stubStreamer{}, stubModerationNotifier{})
		ev := makeEvent()
		ev.Text = ""
		_, err := h(marshal(t, ev), nil)
		if err == nil || err.Error() != "empty reply body" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("nil parent id", func(t *testing.T) {
		h := StreamNewReplyHandler(stubReplyRepo{}, stubReplyUserRepo{}, stubStreamer{}, stubModerationNotifier{})
		ev := makeEvent()
		ev.ParentId = nil
		_, err := h(marshal(t, ev), nil)
		if err == nil || err.Error() != "empty parent ID" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("add reply error", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamNewReplyHandler(stubReplyRepo{addReplyFn: func(reply domain.Tweet) (domain.Tweet, error) {
			return domain.Tweet{}, repoErr
		}}, stubReplyUserRepo{}, stubStreamer{}, stubModerationNotifier{})
		_, err := h(marshal(t, makeEvent()), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("parent user not found", func(t *testing.T) {
		h := StreamNewReplyHandler(stubReplyRepo{}, stubReplyUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}}, stubModerationNotifier{})
		resp, err := h(marshal(t, makeEvent()), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Id == "" {
			t.Fatal("expected reply in response")
		}
	})

	t.Run("reply to own tweet - adds notification", func(t *testing.T) {
		notified := false
		nodeID := warpnet.WarpPeerID("my-node")
		h := StreamNewReplyHandler(stubReplyRepo{}, stubReplyUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{Id: userId, NodeId: nodeID.String()}, nil
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: parentUser, ID: nodeID}}, stubModerationNotifier{addFn: func(not domain.Notification) error {
			notified = true
			if not.Type != domain.NotificationReplyType {
				t.Fatalf("expected reply type, got: %v", not.Type)
			}
			if not.UserId != parentUser {
				t.Fatalf("expected notification for parent user, got: %v", not.UserId)
			}
			return nil
		}})
		resp, err := h(marshal(t, makeEvent()), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Id == "" {
			t.Fatal("expected reply in response")
		}
		if !notified {
			t.Fatal("expected notification to be added")
		}
	})

	t.Run("user repo error", func(t *testing.T) {
		repoErr := errors.New("user repo err")
		h := StreamNewReplyHandler(stubReplyRepo{}, stubReplyUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, repoErr
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}}, stubModerationNotifier{})
		_, err := h(marshal(t, makeEvent()), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected user repo error: %v", err)
		}
	})

	t.Run("successful stream response", func(t *testing.T) {
		h := StreamNewReplyHandler(stubReplyRepo{}, stubReplyUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return []byte("{}"), nil
			},
		}, stubModerationNotifier{})
		resp, err := h(marshal(t, makeEvent()), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Text != "a reply" {
			t.Fatalf("unexpected response: %v", resp)
		}
	})

	t.Run("stream node offline", func(t *testing.T) {
		h := StreamNewReplyHandler(stubReplyRepo{}, stubReplyUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, warpnet.ErrNodeIsOffline
			},
		}, stubModerationNotifier{})
		resp, err := h(marshal(t, makeEvent()), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Id == "" {
			t.Fatal("expected reply in response")
		}
	})

	t.Run("stream error", func(t *testing.T) {
		streamErr := errors.New("broken")
		h := StreamNewReplyHandler(stubReplyRepo{}, stubReplyUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, streamErr
			},
		}, stubModerationNotifier{})
		_, err := h(marshal(t, makeEvent()), nil)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})

	t.Run("remote response with error payload", func(t *testing.T) {
		respErr, _ := json.Marshal(event.ResponseError{Code: 500, Message: "oops"})
		h := StreamNewReplyHandler(stubReplyRepo{}, stubReplyUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return respErr, nil
			},
		}, stubModerationNotifier{})
		resp, err := h(marshal(t, makeEvent()), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Id == "" {
			t.Fatal("expected reply")
		}
	})

	t.Run("strips retweet prefix from rootId and parentId", func(t *testing.T) {
		var capturedRootId string
		h := StreamNewReplyHandler(stubReplyRepo{addReplyFn: func(reply domain.Tweet) (domain.Tweet, error) {
			capturedRootId = reply.RootId
			reply.Id = "reply-1"
			return reply, nil
		}}, stubReplyUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}}, stubModerationNotifier{})
		ev := makeEvent()
		ev.RootId = domain.RetweetPrefix + rootId
		rtParent := domain.RetweetPrefix + parentId
		ev.ParentId = &rtParent
		_, err := h(marshal(t, ev), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if capturedRootId != rootId {
			t.Fatalf("expected stripped rootId %q, got %q", rootId, capturedRootId)
		}
	})
}

func TestStreamGetRepliesHandler(t *testing.T) {
	owner := "owner-1"
	rootId := "root-1"
	parentId := "parent-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetRepliesHandler(stubReplyRepo{}, stubReplyUserRepo{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty parent id", func(t *testing.T) {
		h := StreamGetRepliesHandler(stubReplyRepo{}, stubReplyUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.GetAllRepliesEvent{RootId: rootId}), nil)
		if err == nil || err.Error() != "empty parent id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty root id", func(t *testing.T) {
		h := StreamGetRepliesHandler(stubReplyRepo{}, stubReplyUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.GetAllRepliesEvent{ParentId: parentId}), nil)
		if err == nil || err.Error() != "empty root id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("own tweet replies", func(t *testing.T) {
		replies := []domain.ReplyNode{{Reply: domain.Tweet{Id: "r1", Text: "reply"}}}
		h := StreamGetRepliesHandler(stubReplyRepo{getRepliesTreeFn: func(rootID, parentIdArg string, limit *uint64, cursor *string) ([]domain.ReplyNode, string, error) {
			return replies, "end", nil
		}}, stubReplyUserRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: parentId}})
		resp, err := h(marshal(t, event.GetAllRepliesEvent{RootId: rootId, ParentId: parentId}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.RepliesResponse)
		if len(r.Replies) != 1 {
			t.Fatalf("expected 1 reply, got %d", len(r.Replies))
		}
	})

	t.Run("parent user not found fallback", func(t *testing.T) {
		h := StreamGetRepliesHandler(stubReplyRepo{}, stubReplyUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.GetAllRepliesEvent{RootId: rootId, ParentId: parentId}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		_ = resp.(event.RepliesResponse)
	})

	t.Run("stream node offline fallback", func(t *testing.T) {
		h := StreamGetRepliesHandler(stubReplyRepo{}, stubReplyUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, warpnet.ErrNodeIsOffline
			},
		})
		resp, err := h(marshal(t, event.GetAllRepliesEvent{RootId: rootId, ParentId: parentId}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		_ = resp.(event.RepliesResponse)
	})

	t.Run("stream error", func(t *testing.T) {
		streamErr := errors.New("broken")
		h := StreamGetRepliesHandler(stubReplyRepo{}, stubReplyUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, streamErr
			},
		})
		_, err := h(marshal(t, event.GetAllRepliesEvent{RootId: rootId, ParentId: parentId}), nil)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})

	t.Run("remote successful response", func(t *testing.T) {
		remoteResp, _ := json.Marshal(event.RepliesResponse{
			Cursor:  "end",
			Replies: []domain.ReplyNode{{Reply: domain.Tweet{Id: "r1", Text: "remote reply", RootId: rootId, ParentId: &parentId}}},
		})
		h := StreamGetRepliesHandler(stubReplyRepo{}, stubReplyUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return remoteResp, nil
			},
		})
		resp, err := h(marshal(t, event.GetAllRepliesEvent{RootId: rootId, ParentId: parentId}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.RepliesResponse)
		if len(r.Replies) != 1 {
			t.Fatalf("expected 1 reply: %v", r)
		}
	})
}

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

type stubReplyUserRepo struct {
	getFn func(userId string) (domain.User, error)
}

func (s stubReplyUserRepo) Get(userId string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return domain.User{Id: userId, NodeId: "node-2"}, nil
}

// Replies are created through StreamNewTweetHandler: a tweet with a parent
// (ParentId set) is routed into the thread store instead of the timeline.
func TestStreamNewReplyHandler(t *testing.T) {
	owner := "owner-1"
	parentUser := "parent-user"
	rootId := "root-1"
	parentId := "parent-1"

	makeEvent := func() event.NewTweetEvent {
		pu := parentUser
		return event.NewTweetEvent{
			CreatedAt:    time.Now(),
			Id:           "reply-1",
			ParentId:     &parentId,
			ParentUserId: &pu,
			RootId:       rootId,
			Text:         "a reply",
			UserId:       owner,
			Username:     "testuser",
		}
	}

	build := func(repo stubTweetRepo, userRepo stubReplyUserRepo, notifier stubModerationNotifier, streamer stubStreamer) warpnet.WarpHandlerFunc {
		return StreamNewTweetHandler(
			stubTweetBroadcaster{},
			stubAuth{owner: domain.Owner{UserId: owner}},
			repo,
			stubTimelineRepo{},
			stubFollowChecker{},
			userRepo,
			notifier,
			streamer,
		)
	}

	t.Run("invalid payload", func(t *testing.T) {
		h := build(stubTweetRepo{}, stubReplyUserRepo{}, stubModerationNotifier{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty text", func(t *testing.T) {
		h := build(stubTweetRepo{}, stubReplyUserRepo{}, stubModerationNotifier{}, stubStreamer{})
		ev := makeEvent()
		ev.Text = ""
		_, err := h(marshal(t, ev), nil)
		if err == nil || err.Error() != "empty tweet text" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("add reply error", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := build(stubTweetRepo{addReplyFn: func(reply domain.Tweet) (domain.Tweet, error) {
			return domain.Tweet{}, repoErr
		}}, stubReplyUserRepo{}, stubModerationNotifier{}, stubStreamer{})
		_, err := h(marshal(t, makeEvent()), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("parent user not found", func(t *testing.T) {
		h := build(stubTweetRepo{}, stubReplyUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubModerationNotifier{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, makeEvent()), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Id == "" {
			t.Fatal("expected reply in response")
		}
	})

	t.Run("someone replied to my tweet - adds notification", func(t *testing.T) {
		notified := false
		nodeID := warpnet.WarpPeerID("my-node")
		h := build(stubTweetRepo{}, stubReplyUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{Id: userId, NodeId: nodeID.String()}, nil
		}}, stubModerationNotifier{addFn: func(not domain.Notification) error {
			notified = true
			if not.Type != domain.NotificationReplyType {
				t.Fatalf("expected reply type, got: %v", not.Type)
			}
			if not.UserId != parentUser {
				t.Fatalf("expected notification for parent user, got: %v", not.UserId)
			}
			return nil
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: parentUser, ID: nodeID}})
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
		h := build(stubTweetRepo{}, stubReplyUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, repoErr
		}}, stubModerationNotifier{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		_, err := h(marshal(t, makeEvent()), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected user repo error: %v", err)
		}
	})

	t.Run("successful stream response", func(t *testing.T) {
		h := build(stubTweetRepo{}, stubReplyUserRepo{}, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return []byte("{}"), nil
			},
		})
		resp, err := h(marshal(t, makeEvent()), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Text != "a reply" {
			t.Fatalf("unexpected response: %v", resp)
		}
	})

	t.Run("stream node offline", func(t *testing.T) {
		h := build(stubTweetRepo{}, stubReplyUserRepo{}, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, warpnet.ErrNodeIsOffline
			},
		})
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
		h := build(stubTweetRepo{}, stubReplyUserRepo{}, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, streamErr
			},
		})
		_, err := h(marshal(t, makeEvent()), nil)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})

	t.Run("remote response with error payload", func(t *testing.T) {
		respErr, _ := json.Marshal(event.ResponseError{Code: 500, Message: "oops"})
		h := build(stubTweetRepo{}, stubReplyUserRepo{}, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return respErr, nil
			},
		})
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
		h := build(stubTweetRepo{addReplyFn: func(reply domain.Tweet) (domain.Tweet, error) {
			capturedRootId = reply.RootId
			reply.Id = "reply-1"
			return reply, nil
		}}, stubReplyUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubModerationNotifier{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
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

// Fetching a thread's replies goes through StreamGetTweetsHandler: a RootId
// in the request selects the thread branch, which returns the replies as a
// flat TweetsResponse (replies are tweets).
func TestStreamGetRepliesHandler(t *testing.T) {
	rootId := "root-1"
	parentId := "parent-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetTweetsHandler(stubTweetRepo{}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty parent id defaults to root", func(t *testing.T) {
		// Top-level replies on a thread carry no parent_id from the client —
		// the handler must fall back to root_id so the lookup runs against
		// the first tier of replies hanging off the root tweet.
		var seenRoot, seenParent string
		h := StreamGetTweetsHandler(
			stubTweetRepo{repliesFn: func(rootID, parentIdArg string, _ *uint64, _ *string) ([]domain.Tweet, string, error) {
				seenRoot = rootID
				seenParent = parentIdArg
				return nil, "", nil
			}},
			stubTweetUserRepo{}, stubStreamer{},
		)
		_, err := h(marshal(t, event.GetAllTweetsEvent{RootId: rootId}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if seenRoot != rootId || seenParent != rootId {
			t.Fatalf("expected rootId %q and parentId %q from root fallback, got root=%q parent=%q", rootId, rootId, seenRoot, seenParent)
		}
	})

	t.Run("serves replies from local repo", func(t *testing.T) {
		replies := []domain.Tweet{{Id: "r1", Text: "reply"}}
		h := StreamGetTweetsHandler(stubTweetRepo{repliesFn: func(rootID, parentIdArg string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
			return replies, "end", nil
		}}, stubTweetUserRepo{}, stubStreamer{})
		resp, err := h(marshal(t, event.GetAllTweetsEvent{RootId: rootId, ParentId: parentId}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.TweetsResponse)
		if len(r.Tweets) != 1 {
			t.Fatalf("expected 1 reply, got %d", len(r.Tweets))
		}
		if r.Cursor != "end" {
			t.Fatalf("expected cursor 'end', got %q", r.Cursor)
		}
	})

	t.Run("propagates repo error", func(t *testing.T) {
		boom := errors.New("db down")
		h := StreamGetTweetsHandler(stubTweetRepo{repliesFn: func(string, string, *uint64, *string) ([]domain.Tweet, string, error) {
			return nil, "", boom
		}}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.GetAllTweetsEvent{RootId: rootId, ParentId: parentId}), nil)
		if !errors.Is(err, boom) {
			t.Fatalf("expected db error, got %v", err)
		}
	})

	t.Run("forwards to root author's home node when no local replies", func(t *testing.T) {
		// With an empty local store the handler resolves the root author's home
		// node (foreign, e.g. a bridged post) and returns that node's replies.
		forwarded := event.TweetsResponse{Tweets: []domain.Tweet{{Id: "m1"}}}
		userRepo := stubTweetUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{Id: userId, NodeId: "remote-node"}, nil
		}}
		streamer := stubStreamer{genericStreamFn: func(nodeId string, _ stream.WarpRoute, _ any) ([]byte, error) {
			if nodeId != "remote-node" {
				t.Fatalf("expected forward to remote-node, got %q", nodeId)
			}
			return marshal(t, forwarded), nil
		}}
		h := StreamGetTweetsHandler(stubTweetRepo{}, userRepo, streamer)
		resp, err := h(marshal(t, event.GetAllTweetsEvent{RootId: rootId, RootUserId: "author-1"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.TweetsResponse)
		if len(r.Tweets) != 1 || r.Tweets[0].Id != "m1" {
			t.Fatalf("expected forwarded reply m1, got %+v", r.Tweets)
		}
	})
}

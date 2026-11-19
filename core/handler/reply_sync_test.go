//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

// A reply lives on its parent author's node, which serves the thread; the
// replier's node forwards later edits and deletes of the reply there.

type forwardCall struct {
	nodeId string
	path   stream.WarpRoute
	body   []byte
}

func replyForwardStreamer(owner string, calls *[]forwardCall) stubStreamer {
	return stubStreamer{
		nodeInfo: warpnet.NodeInfo{OwnerId: owner},
		genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			bt, _ := json.Marshal(data)
			*calls = append(*calls, forwardCall{nodeId: nodeId, path: path, body: bt})
			return []byte(`{"code":0,"message":"Accepted"}`), nil
		},
	}
}

func TestOwnReplyChangesReachTheParentAuthor(t *testing.T) {
	const owner, parentAuthor, parentId, replyId = "owner-1", "bob", "parent-1", "reply-1"
	bob := parentAuthor
	users := stubReplyUserRepo{getFn: func(id string) (domain.User, error) {
		return domain.User{Id: id, NodeId: "node-of-" + id}, nil
	}}
	ownReply := func(string, string) (domain.Tweet, error) {
		return domain.Tweet{Id: replyId, UserId: owner, ParentUserId: &bob, RootId: "root-1", Text: "old"}, nil
	}

	t.Run("an edit is forwarded", func(t *testing.T) {
		var calls []forwardCall
		repo := &mockTweetsStorer{GetFunc: ownReply}
		_, err := StreamEditTweetHandler(repo, nil, users, replyForwardStreamer(owner, &calls))(marshal(t, event.EditTweetEvent{
			UserId: owner, TweetId: replyId, ParentId: parentId, Text: "new",
		}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(calls) != 1 || calls[0].path != event.PUBLIC_POST_REPLY_EDIT || calls[0].nodeId != "node-of-bob" {
			t.Fatalf("calls = %+v", calls)
		}
		var sent event.EditTweetEvent
		_ = json.Unmarshal(calls[0].body, &sent)
		if sent.TweetId != replyId || sent.UserId != owner || sent.ParentId != parentId || sent.Text != "new" {
			t.Fatalf("forwarded %+v", sent)
		}
	})

	t.Run("a delete is forwarded", func(t *testing.T) {
		var calls []forwardCall
		repo := stubTweetRepo{getReplyFn: ownReply}
		_, err := StreamDeleteTweetHandler(stubTweetBroadcaster{}, stubAuth{owner: domain.Owner{UserId: owner}},
			repo, stubTimelineRepo{}, stubTweetReactionRepo{}, stubTweetMediaRepo{}, replyForwardStreamer(owner, &calls), users,
		)(marshal(t, event.DeleteTweetEvent{UserId: owner, TweetId: replyId, ParentId: parentId}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(calls) != 1 || calls[0].path != event.PUBLIC_DELETE_REPLY || calls[0].nodeId != "node-of-bob" {
			t.Fatalf("calls = %+v", calls)
		}
		var sent event.DeleteTweetEvent
		_ = json.Unmarshal(calls[0].body, &sent)
		if sent.TweetId != replyId || sent.UserId != owner || sent.ParentId != parentId {
			t.Fatalf("forwarded %+v", sent)
		}
	})

	t.Run("nothing is forwarded when the parent is the owner's own", func(t *testing.T) {
		var calls []forwardCall
		self := owner
		repo := &mockTweetsStorer{GetFunc: func(string, string) (domain.Tweet, error) {
			return domain.Tweet{Id: replyId, UserId: owner, ParentUserId: &self, Text: "old"}, nil
		}}
		if _, err := StreamEditTweetHandler(repo, nil, users, replyForwardStreamer(owner, &calls))(marshal(t, event.EditTweetEvent{
			UserId: owner, TweetId: replyId, ParentId: parentId, Text: "new",
		}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(calls) != 0 {
			t.Fatalf("calls = %+v", calls)
		}
	})

	t.Run("a parent node failure leaves the local change", func(t *testing.T) {
		streamer := stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
				return nil, errors.New("stream broken")
			},
		}
		repo := &mockTweetsStorer{GetFunc: ownReply}
		if _, err := StreamEditTweetHandler(repo, nil, users, streamer)(marshal(t, event.EditTweetEvent{
			UserId: owner, TweetId: replyId, ParentId: parentId, Text: "new",
		}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})
}

func TestStreamEditReplyHandler(t *testing.T) {
	const author, parentId, replyId = "alice", "parent-1", "reply-1"
	nodeId, s := actorNodeStream(t)
	users := stubReplyUserRepo{getFn: func(id string) (domain.User, error) {
		return domain.User{Id: id, NodeId: nodeId.String()}, nil
	}}
	ev := event.EditTweetEvent{UserId: author, TweetId: replyId, ParentId: parentId, Text: "new"}

	t.Run("invalid payload", func(t *testing.T) {
		if _, err := StreamEditReplyHandler(&mockTweetsStorer{}, users, stubStreamer{})([]byte("{"), s); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("required fields", func(t *testing.T) {
		for _, bad := range []event.EditTweetEvent{
			{TweetId: replyId, ParentId: parentId, Text: "new"},
			{UserId: author, ParentId: parentId, Text: "new"},
			{UserId: author, TweetId: replyId, Text: "new"},
			{UserId: author, TweetId: replyId, ParentId: parentId},
		} {
			if _, err := StreamEditReplyHandler(&mockTweetsStorer{}, users, stubStreamer{})(marshal(t, bad), s); err == nil {
				t.Fatalf("expected an error for %+v", bad)
			}
		}
	})

	t.Run("only the author's own node may edit", func(t *testing.T) {
		others := stubReplyUserRepo{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "another-node"}, nil
		}}
		repo := &mockTweetsStorer{UpdateReplyFunc: func(string, domain.Tweet) error {
			t.Fatal("must not update")
			return nil
		}}
		_, err := StreamEditReplyHandler(repo, others, stubStreamer{})(marshal(t, ev), s)
		if !errors.Is(err, warpnet.ErrForeignAuthor) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("someone else's reply is refused", func(t *testing.T) {
		repo := &mockTweetsStorer{GetReplyFunc: func(string, string) (domain.Tweet, error) {
			return domain.Tweet{Id: replyId, UserId: "mallory"}, nil
		}}
		_, err := StreamEditReplyHandler(repo, users, stubStreamer{})(marshal(t, ev), s)
		if !errors.Is(err, warpnet.ErrForeignAuthor) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("the stored copy takes the new text", func(t *testing.T) {
		var updated domain.Tweet
		var under string
		repo := &mockTweetsStorer{
			GetReplyFunc: func(p, id string) (domain.Tweet, error) {
				return domain.Tweet{Id: id, UserId: author, Text: "old"}, nil
			},
			UpdateReplyFunc: func(p string, tw domain.Tweet) error {
				under, updated = p, tw
				return nil
			},
		}
		out, err := StreamEditReplyHandler(repo, users, stubStreamer{})(marshal(t, ev), s)
		if err != nil || out != event.Accepted {
			t.Fatalf("out=%v err=%v", out, err)
		}
		if under != parentId || updated.Text != "new" || updated.Id != replyId {
			t.Fatalf("updated %q under %q", updated.Text, under)
		}
	})
}

func TestStreamDeleteReplyHandler(t *testing.T) {
	const author, parentId, replyId = "alice", "parent-1", "reply-1"
	nodeId, s := actorNodeStream(t)
	users := stubReplyUserRepo{getFn: func(id string) (domain.User, error) {
		return domain.User{Id: id, NodeId: nodeId.String()}, nil
	}}
	ev := event.DeleteTweetEvent{UserId: author, TweetId: replyId, ParentId: parentId}

	t.Run("required fields", func(t *testing.T) {
		for _, bad := range []event.DeleteTweetEvent{
			{TweetId: replyId, ParentId: parentId},
			{UserId: author, ParentId: parentId},
			{UserId: author, TweetId: replyId},
		} {
			if _, err := StreamDeleteReplyHandler(&mockTweetsStorer{}, users, stubStreamer{})(marshal(t, bad), s); err == nil {
				t.Fatalf("expected an error for %+v", bad)
			}
		}
	})

	t.Run("an already missing reply is accepted", func(t *testing.T) {
		repo := &mockTweetsStorer{GetReplyFunc: func(string, string) (domain.Tweet, error) {
			return domain.Tweet{}, database.ErrTweetNotFound
		}}
		out, err := StreamDeleteReplyHandler(repo, users, stubStreamer{})(marshal(t, ev), s)
		if err != nil || out != event.Accepted {
			t.Fatalf("out=%v err=%v", out, err)
		}
	})

	t.Run("someone else's reply is refused", func(t *testing.T) {
		repo := &mockTweetsStorer{
			GetReplyFunc: func(string, string) (domain.Tweet, error) {
				return domain.Tweet{Id: replyId, UserId: "mallory"}, nil
			},
			DeleteReplyFunc: func(string, string, bool) (domain.Tweet, error) {
				t.Fatal("must not delete")
				return domain.Tweet{}, nil
			},
		}
		_, err := StreamDeleteReplyHandler(repo, users, stubStreamer{})(marshal(t, ev), s)
		if !errors.Is(err, warpnet.ErrForeignAuthor) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("the stored copy is removed without touching the network counter", func(t *testing.T) {
		var under string
		transitive := true
		repo := &mockTweetsStorer{
			GetReplyFunc: func(p, id string) (domain.Tweet, error) {
				return domain.Tweet{Id: id, UserId: author}, nil
			},
			DeleteReplyFunc: func(p, id string, isTransitive bool) (domain.Tweet, error) {
				under, transitive = p, isTransitive
				return domain.Tweet{}, nil
			},
		}
		out, err := StreamDeleteReplyHandler(repo, users, stubStreamer{})(marshal(t, ev), s)
		if err != nil || out != event.Accepted {
			t.Fatalf("out=%v err=%v", out, err)
		}
		if under != parentId || transitive {
			t.Fatalf("deleted under %q transitive=%v", under, transitive)
		}
	})
}

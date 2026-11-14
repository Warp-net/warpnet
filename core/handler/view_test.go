//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubViewRepo struct {
	recordFn func(tweetId, viewerId string, isTransitive bool) (uint64, error)
	getFn    func(tweetId string) (uint64, error)
}

func (s stubViewRepo) RecordView(tweetId, viewerId string, isTransitive bool) (uint64, error) {
	if s.recordFn != nil {
		return s.recordFn(tweetId, viewerId, isTransitive)
	}
	return 1, nil
}

func (s stubViewRepo) GetViewsCount(tweetId string) (uint64, error) {
	if s.getFn != nil {
		return s.getFn(tweetId)
	}
	return 0, nil
}

func TestStreamViewHandler(t *testing.T) {
	owner := "owner-1"
	author := "author-1"
	tweetId := "tweet-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamViewHandler(stubViewRepo{}, stubReactionUserRepo{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty fields rejected", func(t *testing.T) {
		h := StreamViewHandler(stubViewRepo{}, stubReactionUserRepo{}, stubStreamer{})
		if _, err := h(marshal(t, event.ViewEvent{UserId: author, ViewerId: owner}), nil); err == nil {
			t.Fatal("expected tweet id error")
		}
		if _, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, ViewerId: owner}), nil); err == nil {
			t.Fatal("expected user id error")
		}
		if _, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, UserId: author}), nil); err == nil {
			t.Fatal("expected owner id error")
		}
	})

	t.Run("author views own tweet - counted", func(t *testing.T) {
		var capturedViewerId string
		users, conn := actorStream(t, author, nil)
		h := StreamViewHandler(stubViewRepo{
			recordFn: func(tweetId, viewerId string, _ bool) (uint64, error) {
				capturedViewerId = viewerId
				return 1, nil
			},
		}, users, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: author}})

		resp, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, UserId: author, ViewerId: author}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if capturedViewerId != author {
			t.Fatalf("expected RecordView with viewerId=%q, got %q", author, capturedViewerId)
		}
		if resp.(event.ViewsCountResponse).Count != 1 {
			t.Fatalf("unexpected response: %v", resp)
		}
	})

	t.Run("non-author view records and returns count - own node hosts author", func(t *testing.T) {
		var capturedTweetId, capturedViewerId string
		users, conn := actorStream(t, owner, nil)
		h := StreamViewHandler(stubViewRepo{
			recordFn: func(tweetId, viewerId string, _ bool) (uint64, error) {
				capturedTweetId = tweetId
				capturedViewerId = viewerId
				return 42, nil
			},
		}, users, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: author}})

		resp, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, UserId: author, ViewerId: owner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if capturedTweetId != tweetId || capturedViewerId != owner {
			t.Fatalf("unexpected RecordView args: tweet=%q viewer=%q", capturedTweetId, capturedViewerId)
		}
		if resp.(event.ViewsCountResponse).Count != 42 {
			t.Fatalf("unexpected response: %v", resp)
		}
	})

	t.Run("non-author view strips retweet prefix", func(t *testing.T) {
		var capturedTweetId string
		users, conn := actorStream(t, owner, nil)
		h := StreamViewHandler(stubViewRepo{
			recordFn: func(tweetId, viewerId string, _ bool) (uint64, error) {
				capturedTweetId = tweetId
				return 1, nil
			},
		}, users, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: author}})

		_, err := h(marshal(t, event.ViewEvent{TweetId: domain.RetweetPrefix + tweetId, UserId: author, ViewerId: owner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if capturedTweetId != tweetId {
			t.Fatalf("expected stripped tweet id %q, got %q", tweetId, capturedTweetId)
		}
	})

	t.Run("non-author view forwards to remote author", func(t *testing.T) {
		forwarded := false
		users, conn := actorStream(t, owner, func(userId string) (domain.User, error) {
			return domain.User{Id: userId, NodeId: "remote-node"}, nil
		})
		h := StreamViewHandler(stubViewRepo{
			recordFn: func(tweetId, viewerId string, _ bool) (uint64, error) { return 5, nil },
		}, users, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				forwarded = true
				return []byte(`{"count":5}`), nil
			},
		})

		resp, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, UserId: author, ViewerId: owner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !forwarded {
			t.Fatal("expected forwarding to remote author")
		}
		if resp.(event.ViewsCountResponse).Count != 5 {
			t.Fatalf("unexpected response: %v", resp)
		}
	})

	t.Run("viewer's node counts its own view when forwarding fails", func(t *testing.T) {
		var transitive bool
		users, conn := actorStream(t, owner, nil)
		h := StreamViewHandler(stubViewRepo{
			recordFn: func(tweetId, viewerId string, isTransitive bool) (uint64, error) {
				transitive = isTransitive
				return 9, nil
			},
		}, users, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, errors.New("network down")
			},
		})

		resp, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, UserId: author, ViewerId: owner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !transitive {
			t.Fatal("the viewer's own node puts its view into the replicated counter")
		}
		if resp.(event.ViewsCountResponse).Count != 9 {
			t.Fatalf("expected the local count 9, got: %v", resp)
		}
	})

	t.Run("author's node keeps a remote viewer's view local", func(t *testing.T) {
		transitive := true
		users, conn := actorStream(t, owner, nil)
		h := StreamViewHandler(stubViewRepo{
			recordFn: func(tweetId, viewerId string, isTransitive bool) (uint64, error) {
				transitive = isTransitive
				return 3, nil
			},
		}, users, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: author}})

		if _, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, UserId: author, ViewerId: owner}), conn); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if transitive {
			t.Fatal("only the viewer's node may count the view in the replicated counter")
		}
	})
}

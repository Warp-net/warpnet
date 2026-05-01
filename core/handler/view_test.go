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
)

type stubViewRepo struct {
	recordFn func(tweetId, viewerId string) (uint64, error)
	getFn    func(tweetId string) (uint64, error)
}

func (s stubViewRepo) RecordView(tweetId, viewerId string) (uint64, error) {
	if s.recordFn != nil {
		return s.recordFn(tweetId, viewerId)
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
		h := StreamViewHandler(stubViewRepo{}, stubLikeUserRepo{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty fields rejected", func(t *testing.T) {
		h := StreamViewHandler(stubViewRepo{}, stubLikeUserRepo{}, stubStreamer{})
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

	t.Run("author views own tweet - not counted", func(t *testing.T) {
		recorded := false
		h := StreamViewHandler(stubViewRepo{
			recordFn: func(tweetId, viewerId string) (uint64, error) {
				recorded = true
				return 0, nil
			},
			getFn: func(tweetId string) (uint64, error) { return 7, nil },
		}, stubLikeUserRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: author}})

		resp, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, UserId: author, ViewerId: author}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if recorded {
			t.Fatal("RecordView must not be called for author")
		}
		if resp.(event.ViewsCountResponse).Count != 7 {
			t.Fatalf("unexpected response: %v", resp)
		}
	})

	t.Run("author views own tweet - missing views returns zero", func(t *testing.T) {
		h := StreamViewHandler(stubViewRepo{
			getFn: func(tweetId string) (uint64, error) { return 0, database.ErrViewsNotFound },
		}, stubLikeUserRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: author}})

		resp, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, UserId: author, ViewerId: author}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.ViewsCountResponse).Count != 0 {
			t.Fatalf("unexpected response: %v", resp)
		}
	})

	t.Run("non-author view records and returns count - own node hosts author", func(t *testing.T) {
		var capturedTweetId, capturedViewerId string
		h := StreamViewHandler(stubViewRepo{
			recordFn: func(tweetId, viewerId string) (uint64, error) {
				capturedTweetId = tweetId
				capturedViewerId = viewerId
				return 42, nil
			},
		}, stubLikeUserRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: author}})

		resp, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, UserId: author, ViewerId: owner}), nil)
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
		h := StreamViewHandler(stubViewRepo{
			recordFn: func(tweetId, viewerId string) (uint64, error) {
				capturedTweetId = tweetId
				return 1, nil
			},
		}, stubLikeUserRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: author}})

		_, err := h(marshal(t, event.ViewEvent{TweetId: domain.RetweetPrefix + tweetId, UserId: author, ViewerId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if capturedTweetId != tweetId {
			t.Fatalf("expected stripped tweet id %q, got %q", tweetId, capturedTweetId)
		}
	})

	t.Run("non-author view forwards to remote author", func(t *testing.T) {
		forwarded := false
		h := StreamViewHandler(stubViewRepo{
			recordFn: func(tweetId, viewerId string) (uint64, error) { return 5, nil },
		}, stubLikeUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{Id: userId, NodeId: "remote-node"}, nil
		}}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				forwarded = true
				return []byte(`{"count":5}`), nil
			},
		})

		resp, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, UserId: author, ViewerId: owner}), nil)
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

	t.Run("forwarding errors are not fatal", func(t *testing.T) {
		h := StreamViewHandler(stubViewRepo{
			recordFn: func(tweetId, viewerId string) (uint64, error) { return 9, nil },
		}, stubLikeUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, errors.New("network down")
			},
		})

		resp, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, UserId: author, ViewerId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.ViewsCountResponse).Count != 9 {
			t.Fatalf("unexpected response: %v", resp)
		}
	})
}

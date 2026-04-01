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

type stubLikeRepo struct {
	likeFn       func(tweetId, userId string) (uint64, error)
	unlikeFn     func(tweetId, userId string) (uint64, error)
	likesCountFn func(tweetId string) (uint64, error)
	likersFn     func(tweetId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubLikeRepo) Like(tweetId, userId string) (uint64, error) {
	if s.likeFn != nil {
		return s.likeFn(tweetId, userId)
	}
	return 1, nil
}
func (s stubLikeRepo) Unlike(tweetId, userId string) (uint64, error) {
	if s.unlikeFn != nil {
		return s.unlikeFn(tweetId, userId)
	}
	return 0, nil
}
func (s stubLikeRepo) LikesCount(tweetId string) (uint64, error) {
	if s.likesCountFn != nil {
		return s.likesCountFn(tweetId)
	}
	return 0, nil
}
func (s stubLikeRepo) Likers(tweetId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.likersFn != nil {
		return s.likersFn(tweetId, limit, cursor)
	}
	return nil, "", nil
}

type stubLikeUserRepo struct {
	getBatchFn func(userIds ...string) ([]domain.User, error)
	getFn      func(userId string) (domain.User, error)
}

func (s stubLikeUserRepo) GetBatch(userIds ...string) ([]domain.User, error) {
	if s.getBatchFn != nil {
		return s.getBatchFn(userIds...)
	}
	return nil, nil
}
func (s stubLikeUserRepo) Get(userId string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return domain.User{Id: userId, NodeId: "node-2"}, nil
}

func TestStreamLikeHandler(t *testing.T) {
	owner := "owner-1"
	tweetOwner := "tweet-owner"
	tweetId := "tweet-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamLikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty owner id", func(t *testing.T) {
		h := StreamLikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.LikeEvent{TweetId: tweetId, UserId: tweetOwner}), nil)
		if err == nil || err.Error() != "like: empty owner id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamLikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.LikeEvent{TweetId: tweetId, OwnerId: owner}), nil)
		if err == nil || err.Error() != "like: empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty tweet id", func(t *testing.T) {
		h := StreamLikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.LikeEvent{OwnerId: owner, UserId: tweetOwner}), nil)
		if err == nil || err.Error() != "like: empty tweet id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("like repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamLikeHandler(stubLikeRepo{likeFn: func(tweetId, userId string) (uint64, error) {
			return 0, repoErr
		}}, stubLikeUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.LikeEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got: %v", err)
		}
	})

	t.Run("own tweet like", func(t *testing.T) {
		h := StreamLikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.LikeEvent{TweetId: tweetId, OwnerId: owner, UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.LikesCountResponse).Count != 1 {
			t.Fatalf("unexpected count: %v", resp)
		}
	})

	t.Run("someone else liked (exchange finished)", func(t *testing.T) {
		h := StreamLikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: "other-node"}})
		resp, err := h(marshal(t, event.LikeEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.LikesCountResponse).Count != 1 {
			t.Fatalf("unexpected count: %v", resp)
		}
	})

	t.Run("liked user not found", func(t *testing.T) {
		h := StreamLikeHandler(stubLikeRepo{}, stubLikeUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.LikeEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.LikesCountResponse).Count != 1 {
			t.Fatalf("unexpected count: %v", resp)
		}
	})

	t.Run("user repo error", func(t *testing.T) {
		repoErr := errors.New("user repo")
		h := StreamLikeHandler(stubLikeRepo{}, stubLikeUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, repoErr
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		_, err := h(marshal(t, event.LikeEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected user repo error: %v", err)
		}
	})

	t.Run("stream node offline", func(t *testing.T) {
		h := StreamLikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, warpnet.ErrNodeIsOffline
			},
		})
		resp, err := h(marshal(t, event.LikeEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.LikesCountResponse).Count != 1 {
			t.Fatalf("unexpected count: %v", resp)
		}
	})

	t.Run("stream error", func(t *testing.T) {
		streamErr := errors.New("stream broken")
		h := StreamLikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, streamErr
			},
		})
		_, err := h(marshal(t, event.LikeEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), nil)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})

	t.Run("remote response with error payload", func(t *testing.T) {
		respErr, _ := json.Marshal(event.ResponseError{Code: 500, Message: "remote error"})
		h := StreamLikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return respErr, nil
			},
		})
		resp, err := h(marshal(t, event.LikeEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.LikesCountResponse).Count != 1 {
			t.Fatalf("unexpected count: %v", resp)
		}
	})

	t.Run("strips retweet prefix from tweet id", func(t *testing.T) {
		var capturedTweetId string
		h := StreamLikeHandler(stubLikeRepo{likeFn: func(tweetId, userId string) (uint64, error) {
			capturedTweetId = tweetId
			return 1, nil
		}}, stubLikeUserRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		_, err := h(marshal(t, event.LikeEvent{TweetId: domain.RetweetPrefix + tweetId, OwnerId: owner, UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if capturedTweetId != tweetId {
			t.Fatalf("expected stripped tweet id %q, got %q", tweetId, capturedTweetId)
		}
	})

	t.Run("successful stream", func(t *testing.T) {
		h := StreamLikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return []byte("{}"), nil
			},
		})
		resp, err := h(marshal(t, event.LikeEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.LikesCountResponse).Count != 1 {
			t.Fatalf("unexpected count: %v", resp)
		}
	})
}

func TestStreamUnlikeHandler(t *testing.T) {
	owner := "owner-1"
	tweetOwner := "tweet-owner"
	tweetId := "tweet-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamUnlikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamUnlikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.UnlikeEvent{TweetId: tweetId, OwnerId: owner}), nil)
		if err == nil || err.Error() != "empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty tweet id", func(t *testing.T) {
		h := StreamUnlikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.UnlikeEvent{OwnerId: owner, UserId: tweetOwner}), nil)
		if err == nil || err.Error() != "empty tweet id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("unlike repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamUnlikeHandler(stubLikeRepo{unlikeFn: func(tweetId, userId string) (uint64, error) {
			return 0, repoErr
		}}, stubLikeUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.UnlikeEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got: %v", err)
		}
	})

	t.Run("own tweet unlike", func(t *testing.T) {
		h := StreamUnlikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.UnlikeEvent{TweetId: tweetId, OwnerId: owner, UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		_ = resp.(event.LikesCountResponse)
	})

	t.Run("someone else unliked (exchange finished)", func(t *testing.T) {
		h := StreamUnlikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: "other-node"}})
		resp, err := h(marshal(t, event.UnlikeEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		_ = resp.(event.LikesCountResponse)
	})

	t.Run("unliked user not found", func(t *testing.T) {
		h := StreamUnlikeHandler(stubLikeRepo{}, stubLikeUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		_, err := h(marshal(t, event.UnlikeEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("stream node offline", func(t *testing.T) {
		h := StreamUnlikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, warpnet.ErrNodeIsOffline
			},
		})
		_, err := h(marshal(t, event.UnlikeEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("stream error", func(t *testing.T) {
		streamErr := errors.New("stream broken")
		h := StreamUnlikeHandler(stubLikeRepo{}, stubLikeUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, streamErr
			},
		})
		_, err := h(marshal(t, event.UnlikeEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), nil)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})

	t.Run("strips retweet prefix", func(t *testing.T) {
		var capturedTweetId string
		h := StreamUnlikeHandler(stubLikeRepo{unlikeFn: func(tweetId, userId string) (uint64, error) {
			capturedTweetId = tweetId
			return 0, nil
		}}, stubLikeUserRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		_, err := h(marshal(t, event.UnlikeEvent{TweetId: domain.RetweetPrefix + tweetId, OwnerId: owner, UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if capturedTweetId != tweetId {
			t.Fatalf("expected stripped id %q, got %q", tweetId, capturedTweetId)
		}
	})
}

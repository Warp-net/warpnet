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

type stubRetweetUserRepo struct {
	getBatchFn func(ids ...string) ([]domain.User, error)
	getFn      func(userId string) (domain.User, error)
}

func (s stubRetweetUserRepo) GetBatch(ids ...string) ([]domain.User, error) {
	if s.getBatchFn != nil {
		return s.getBatchFn(ids...)
	}
	return nil, nil
}
func (s stubRetweetUserRepo) Get(userId string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return domain.User{Id: userId, NodeId: "node-2"}, nil
}

type stubReTweetRepo struct {
	getFn           func(userID, tweetID string) (domain.Tweet, error)
	newRetweetFn    func(tweet domain.Tweet) (domain.Tweet, error)
	unRetweetFn     func(retweetedByUserID, tweetId string) error
	retweetsCountFn func(tweetId string) (uint64, error)
	retweetersFn    func(tweetId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubReTweetRepo) Get(userID, tweetID string) (domain.Tweet, error) {
	if s.getFn != nil {
		return s.getFn(userID, tweetID)
	}
	return domain.Tweet{Id: tweetID, UserId: userID}, nil
}
func (s stubReTweetRepo) NewRetweet(tweet domain.Tweet) (domain.Tweet, error) {
	if s.newRetweetFn != nil {
		return s.newRetweetFn(tweet)
	}
	return tweet, nil
}
func (s stubReTweetRepo) UnRetweet(retweetedByUserID, tweetId string) error {
	if s.unRetweetFn != nil {
		return s.unRetweetFn(retweetedByUserID, tweetId)
	}
	return nil
}
func (s stubReTweetRepo) RetweetsCount(tweetId string) (uint64, error) {
	if s.retweetsCountFn != nil {
		return s.retweetsCountFn(tweetId)
	}
	return 0, nil
}
func (s stubReTweetRepo) Retweeters(tweetId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.retweetersFn != nil {
		return s.retweetersFn(tweetId, limit, cursor)
	}
	return nil, "", nil
}

type stubTimelineRepo struct {
	addFn func(userId string, tweet domain.Tweet) error
}

func (s stubTimelineRepo) AddTweetToTimeline(userId string, tweet domain.Tweet) error {
	if s.addFn != nil {
		return s.addFn(userId, tweet)
	}
	return nil
}

func TestStreamNewReTweetHandler(t *testing.T) {
	owner := "owner-1"
	tweetOwner := "tweet-owner"
	retweeter := owner
	tweetId := "tweet-1"

	makeTweet := func() event.NewRetweetEvent {
		rt := retweeter
		return domain.Tweet{
			Id:          tweetId,
			UserId:      tweetOwner,
			Text:        "original",
			RetweetedBy: &rt,
			CreatedAt:   time.Now(),
		}
	}

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamNewReTweetHandler(stubRetweetUserRepo{}, stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing retweeted by", func(t *testing.T) {
		h := StreamNewReTweetHandler(stubRetweetUserRepo{}, stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{})
		tw := domain.Tweet{Id: tweetId, UserId: tweetOwner}
		_, err := h(marshal(t, event.NewRetweetEvent(tw)), nil)
		if err == nil || err.Error() != "retweeted by unknown" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("missing tweet id", func(t *testing.T) {
		h := StreamNewReTweetHandler(stubRetweetUserRepo{}, stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{})
		rt := retweeter
		tw := domain.Tweet{UserId: tweetOwner, RetweetedBy: &rt}
		_, err := h(marshal(t, event.NewRetweetEvent(tw)), nil)
		if err == nil || err.Error() != "empty retweet id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamNewReTweetHandler(stubRetweetUserRepo{}, stubReTweetRepo{
			newRetweetFn: func(tweet domain.Tweet) (domain.Tweet, error) { return domain.Tweet{}, repoErr },
		}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{})
		_, err := h(marshal(t, makeTweet()), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("own tweet retweet", func(t *testing.T) {
		h := StreamNewReTweetHandler(stubRetweetUserRepo{}, stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: tweetOwner},
		})
		rt := tweetOwner
		tw := domain.Tweet{Id: tweetId, UserId: tweetOwner, RetweetedBy: &rt, CreatedAt: time.Now()}
		resp, err := h(marshal(t, event.NewRetweetEvent(tw)), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Id == "" {
			t.Fatalf("expected tweet in response")
		}
	})

	t.Run("someone retweeted my tweet - adds notification", func(t *testing.T) {
		notified := false
		h := StreamNewReTweetHandler(stubRetweetUserRepo{}, stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{addFn: func(not domain.Notification) error {
			notified = true
			if not.Type != domain.NotificationRetweetType {
				t.Fatalf("expected retweet type, got: %v", not.Type)
			}
			if not.UserId != tweetOwner {
				t.Fatalf("expected notification for tweet owner, got: %v", not.UserId)
			}
			return nil
		}}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: tweetOwner},
		})
		resp, err := h(marshal(t, makeTweet()), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Id == "" {
			t.Fatalf("expected tweet in response")
		}
		if !notified {
			t.Fatal("expected notification to be added")
		}
	})

	t.Run("owner retweeter adds to timeline", func(t *testing.T) {
		timelineAdded := false
		h := StreamNewReTweetHandler(stubRetweetUserRepo{}, stubReTweetRepo{}, stubTimelineRepo{
			addFn: func(userId string, tweet domain.Tweet) error {
				timelineAdded = true
				return nil
			},
		}, stubModerationNotifier{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, makeTweet()), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !timelineAdded {
			t.Fatal("expected timeline to be updated")
		}
		_ = resp
	})

	t.Run("tweet owner user not found", func(t *testing.T) {
		h := StreamNewReTweetHandler(stubRetweetUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, makeTweet()), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Id == "" {
			t.Fatal("expected tweet response")
		}
	})

	t.Run("stream node offline", func(t *testing.T) {
		h := StreamNewReTweetHandler(stubRetweetUserRepo{}, stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, warpnet.ErrNodeIsOffline
			},
		})
		_, err := h(marshal(t, makeTweet()), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("stream error", func(t *testing.T) {
		streamErr := errors.New("broken")
		h := StreamNewReTweetHandler(stubRetweetUserRepo{}, stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, streamErr
			},
		})
		_, err := h(marshal(t, makeTweet()), nil)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})

	t.Run("remote response with error payload", func(t *testing.T) {
		respErr, _ := json.Marshal(event.ResponseError{Code: 500, Message: "oops"})
		h := StreamNewReTweetHandler(stubRetweetUserRepo{}, stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return respErr, nil
			},
		})
		_, err := h(marshal(t, makeTweet()), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})
}

func TestStreamUnretweetHandler(t *testing.T) {
	owner := "owner-1"
	tweetOwner := "tweet-owner"
	tweetId := "tweet-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamUnretweetHandler(stubReTweetRepo{}, stubRetweetUserRepo{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty retweeter id", func(t *testing.T) {
		h := StreamUnretweetHandler(stubReTweetRepo{}, stubRetweetUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId}), nil)
		if err == nil || err.Error() != "empty retweeter id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty tweet id", func(t *testing.T) {
		h := StreamUnretweetHandler(stubReTweetRepo{}, stubRetweetUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.UnretweetEvent{RetweeterId: owner}), nil)
		if err == nil || err.Error() != "empty tweet id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("get tweet error", func(t *testing.T) {
		repoErr := errors.New("not found")
		h := StreamUnretweetHandler(stubReTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) {
			return domain.Tweet{}, repoErr
		}}, stubRetweetUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId, RetweeterId: owner}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("unretweet error", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamUnretweetHandler(stubReTweetRepo{
			unRetweetFn: func(retweetedByUserID, tweetId string) error { return repoErr },
		}, stubRetweetUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId, RetweeterId: owner}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected unretweet error: %v", err)
		}
	})

	t.Run("own tweet unretweet", func(t *testing.T) {
		h := StreamUnretweetHandler(stubReTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) {
			return domain.Tweet{Id: tweetID, UserId: owner}, nil
		}}, stubRetweetUserRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId, RetweeterId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted: %v", resp)
		}
	})

	t.Run("tweet owner not found", func(t *testing.T) {
		h := StreamUnretweetHandler(stubReTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) {
			return domain.Tweet{Id: tweetID, UserId: tweetOwner}, nil
		}}, stubRetweetUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId, RetweeterId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted: %v", resp)
		}
	})

	t.Run("stream node offline", func(t *testing.T) {
		h := StreamUnretweetHandler(stubReTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) {
			return domain.Tweet{Id: tweetID, UserId: tweetOwner}, nil
		}}, stubRetweetUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, warpnet.ErrNodeIsOffline
			},
		})
		resp, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId, RetweeterId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted: %v", resp)
		}
	})

	t.Run("stream error", func(t *testing.T) {
		streamErr := errors.New("broken")
		h := StreamUnretweetHandler(stubReTweetRepo{getFn: func(userID, tweetID string) (domain.Tweet, error) {
			return domain.Tweet{Id: tweetID, UserId: tweetOwner}, nil
		}}, stubRetweetUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, streamErr
			},
		})
		_, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetId, RetweeterId: owner}), nil)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})
}

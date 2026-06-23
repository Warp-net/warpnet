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

type stubTweetRepo struct {
	tweetsCountFn   func(userId string) (uint64, error)
	getViewsCountFn func(tweetId string) (uint64, error)
	isBlocklistedFn func(tweetId string) bool
	blocklistFn     func(tweetId string) error
	getFn           func(userID, tweetID string) (domain.Tweet, error)
	listFn          func(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error)
	createFn        func(userId string, tweet domain.Tweet) (domain.Tweet, error)
	deleteFn        func(userID, tweetID string) error
	unRetweetFn     func(retweetedByUserID, tweetId string) error
	createWithTTLFn func(userId string, tweet domain.Tweet, duration time.Duration) (domain.Tweet, error)
	addReplyFn      func(reply domain.Tweet) (domain.Tweet, error)
	getReplyFn      func(rootID, replyID string) (domain.Tweet, error)
	deleteReplyFn   func(rootID, replyID string) (domain.Tweet, error)
	repliesFn       func(parentID string, limit *uint64, cursor *string) ([]domain.Tweet, string, error)
}

func (s stubTweetRepo) TweetsCount(userId string) (uint64, error) {
	if s.tweetsCountFn != nil {
		return s.tweetsCountFn(userId)
	}
	return 0, nil
}
func (s stubTweetRepo) GetViewsCount(tweetId string) (uint64, error) {
	if s.getViewsCountFn != nil {
		return s.getViewsCountFn(tweetId)
	}
	return 0, nil
}
func (s stubTweetRepo) IsBlocklisted(tweetId string) bool {
	if s.isBlocklistedFn != nil {
		return s.isBlocklistedFn(tweetId)
	}
	return false
}
func (s stubTweetRepo) Blocklist(tweetId string) error {
	if s.blocklistFn != nil {
		return s.blocklistFn(tweetId)
	}
	return nil
}
func (s stubTweetRepo) Get(userID, tweetID string) (domain.Tweet, error) {
	if s.getFn != nil {
		return s.getFn(userID, tweetID)
	}
	return domain.Tweet{Id: tweetID, UserId: userID, Text: "cached"}, nil
}
func (s stubTweetRepo) List(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
	if s.listFn != nil {
		return s.listFn(userId, limit, cursor)
	}
	return nil, "", nil
}
func (s stubTweetRepo) Create(userId string, tweet domain.Tweet) (domain.Tweet, error) {
	if s.createFn != nil {
		return s.createFn(userId, tweet)
	}
	tweet.Id = "tweet-new"
	return tweet, nil
}
func (s stubTweetRepo) Delete(userID, tweetID string) error {
	if s.deleteFn != nil {
		return s.deleteFn(userID, tweetID)
	}
	return nil
}
func (s stubTweetRepo) UnRetweet(retweetedByUserID, tweetId string) error {
	if s.unRetweetFn != nil {
		return s.unRetweetFn(retweetedByUserID, tweetId)
	}
	return nil
}
func (s stubTweetRepo) Retweeters(tweetId string, limit *uint64, cursor *string) ([]string, string, error) {
	return nil, "", nil
}
func (s stubTweetRepo) CreateWithTTL(userId string, tweet domain.Tweet, duration time.Duration) (domain.Tweet, error) {
	if s.createWithTTLFn != nil {
		return s.createWithTTLFn(userId, tweet, duration)
	}
	return tweet, nil
}
func (s stubTweetRepo) Update(tweet domain.Tweet) error { return nil }
func (s stubTweetRepo) Pin(userId, tweetId string) (domain.Tweet, error) {
	return domain.Tweet{Id: tweetId, UserId: userId, Pinned: true}, nil
}
func (s stubTweetRepo) Unpin(userId, tweetId string) (domain.Tweet, error) {
	return domain.Tweet{Id: tweetId, UserId: userId}, nil
}
func (s stubTweetRepo) AppendEdit(edit domain.TweetEdit) (domain.TweetEdit, error) {
	return edit, nil
}
func (s stubTweetRepo) AddReply(reply domain.Tweet) (domain.Tweet, error) {
	if s.addReplyFn != nil {
		return s.addReplyFn(reply)
	}
	if reply.Id == "" {
		reply.Id = "reply-1"
	}
	return reply, nil
}
func (s stubTweetRepo) GetReply(rootID, replyID string) (domain.Tweet, error) {
	if s.getReplyFn != nil {
		return s.getReplyFn(rootID, replyID)
	}
	return domain.Tweet{Id: replyID, RootId: rootID}, nil
}
func (s stubTweetRepo) DeleteReply(rootID, replyID string) (domain.Tweet, error) {
	if s.deleteReplyFn != nil {
		return s.deleteReplyFn(rootID, replyID)
	}
	return domain.Tweet{Id: replyID, RootId: rootID}, nil
}
func (s stubTweetRepo) GetReplies(parentID string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
	if s.repliesFn != nil {
		return s.repliesFn(parentID, limit, cursor)
	}
	return nil, "", nil
}

type stubFollowChecker struct {
	following bool
}

func (s stubFollowChecker) IsFollowing(_, _ string) bool {
	return s.following
}

type stubTweetBroadcaster struct {
	publishFn func(ownerId, dest string, bt []byte) error
}

func (s stubTweetBroadcaster) PublishUpdateToFollowers(ownerId, dest string, bt []byte) error {
	if s.publishFn != nil {
		return s.publishFn(ownerId, dest, bt)
	}
	return nil
}

type stubTweetUserRepo struct {
	getFn func(userId string) (domain.User, error)
}

func (s stubTweetUserRepo) Get(userId string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return domain.User{Id: userId, NodeId: "node-2"}, nil
}

type stubTweetLikeRepo struct {
	likeFn       func(tweetId, userId string) (uint64, error)
	unlikeFn     func(tweetId, userId string) (uint64, error)
	likesCountFn func(tweetId string) (uint64, error)
	likersFn     func(tweetId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubTweetLikeRepo) Like(tweetId, userId string) (uint64, error) {
	if s.likeFn != nil {
		return s.likeFn(tweetId, userId)
	}
	return 0, nil
}
func (s stubTweetLikeRepo) Unlike(tweetId, userId string) (uint64, error) {
	if s.unlikeFn != nil {
		return s.unlikeFn(tweetId, userId)
	}
	return 0, nil
}
func (s stubTweetLikeRepo) LikesCount(tweetId string) (uint64, error) {
	if s.likesCountFn != nil {
		return s.likesCountFn(tweetId)
	}
	return 0, nil
}
func (s stubTweetLikeRepo) Likers(tweetId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.likersFn != nil {
		return s.likersFn(tweetId, limit, cursor)
	}
	return nil, "", nil
}

type stubTweetRetweetRepo struct {
	getFn           func(userID, tweetID string) (domain.Tweet, error)
	newRetweetFn    func(tweet domain.Tweet) (domain.Tweet, error)
	unRetweetFn     func(retweetedByUserID, tweetId string) error
	retweetsCountFn func(tweetId string) (uint64, error)
	retweetersFn    func(tweetId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubTweetRetweetRepo) Get(userID, tweetID string) (domain.Tweet, error) {
	if s.getFn != nil {
		return s.getFn(userID, tweetID)
	}
	return domain.Tweet{Id: tweetID, UserId: userID}, nil
}
func (s stubTweetRetweetRepo) NewRetweet(tweet domain.Tweet) (domain.Tweet, error) {
	if s.newRetweetFn != nil {
		return s.newRetweetFn(tweet)
	}
	return tweet, nil
}
func (s stubTweetRetweetRepo) UnRetweet(retweetedByUserID, tweetId string) error {
	if s.unRetweetFn != nil {
		return s.unRetweetFn(retweetedByUserID, tweetId)
	}
	return nil
}
func (s stubTweetRetweetRepo) RetweetsCount(tweetId string) (uint64, error) {
	if s.retweetsCountFn != nil {
		return s.retweetsCountFn(tweetId)
	}
	return 0, nil
}
func (s stubTweetRetweetRepo) Retweeters(tweetId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.retweetersFn != nil {
		return s.retweetersFn(tweetId, limit, cursor)
	}
	return nil, "", nil
}

type stubRepliesCounter struct {
	repliesCountFn func(tweetId string) (uint64, error)
}

func (s stubRepliesCounter) RepliesCount(tweetId string) (uint64, error) {
	if s.repliesCountFn != nil {
		return s.repliesCountFn(tweetId)
	}
	return 0, nil
}

func TestStreamNewTweetHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamNewTweetHandler(stubTweetBroadcaster{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetRepo{}, stubTimelineRepo{}, stubFollowChecker{}, stubTweetUserRepo{}, stubModerationNotifier{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamNewTweetHandler(stubTweetBroadcaster{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetRepo{}, stubTimelineRepo{}, stubFollowChecker{}, stubTweetUserRepo{}, stubModerationNotifier{}, stubStreamer{})
		_, err := h(marshal(t, event.NewTweetEvent{Text: "hello"}), nil)
		if err == nil || err.Error() != "empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty tweet text", func(t *testing.T) {
		h := StreamNewTweetHandler(stubTweetBroadcaster{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetRepo{}, stubTimelineRepo{}, stubFollowChecker{}, stubTweetUserRepo{}, stubModerationNotifier{}, stubStreamer{})
		_, err := h(marshal(t, event.NewTweetEvent{UserId: owner}), nil)
		if err == nil || err.Error() != "empty tweet text" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("tweet text too long", func(t *testing.T) {
		h := StreamNewTweetHandler(stubTweetBroadcaster{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetRepo{}, stubTimelineRepo{}, stubFollowChecker{}, stubTweetUserRepo{}, stubModerationNotifier{}, stubStreamer{})
		longText := make([]byte, tweetCharLimit+1)
		for i := range longText {
			longText[i] = 'a'
		}
		_, err := h(marshal(t, event.NewTweetEvent{UserId: owner, Text: string(longText)}), nil)
		if err == nil || err.Error() != "tweet text is too long" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("moderated tweet gets blocklisted", func(t *testing.T) {
		blocklisted := false
		fail := domain.FAIL
		h := StreamNewTweetHandler(stubTweetBroadcaster{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetRepo{blocklistFn: func(tweetId string) error {
			blocklisted = true
			return nil
		}}, stubTimelineRepo{}, stubFollowChecker{}, stubTweetUserRepo{}, stubModerationNotifier{}, stubStreamer{})
		_, err := h(marshal(t, event.NewTweetEvent{Id: "t1", UserId: owner, Text: "bad", Moderation: &domain.TweetModeration{IsOk: fail}}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !blocklisted {
			t.Fatal("expected tweet to be blocklisted")
		}
	})

	t.Run("create error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamNewTweetHandler(stubTweetBroadcaster{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetRepo{createFn: func(userId string, tweet domain.Tweet) (domain.Tweet, error) {
			return domain.Tweet{}, repoErr
		}}, stubTimelineRepo{}, stubFollowChecker{}, stubTweetUserRepo{}, stubModerationNotifier{}, stubStreamer{})
		_, err := h(marshal(t, event.NewTweetEvent{UserId: owner, Text: "hello"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("own tweet - success with broadcast", func(t *testing.T) {
		published := false
		h := StreamNewTweetHandler(stubTweetBroadcaster{publishFn: func(ownerId, dest string, bt []byte) error {
			published = true
			return nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetRepo{}, stubTimelineRepo{}, stubFollowChecker{}, stubTweetUserRepo{}, stubModerationNotifier{}, stubStreamer{})
		resp, err := h(marshal(t, event.NewTweetEvent{UserId: owner, Text: "hello"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Text != "hello" {
			t.Fatalf("unexpected response: %v", resp)
		}
		if !published {
			t.Fatal("expected broadcast to be called")
		}
	})

	t.Run("followed user tweet - stored, no broadcast", func(t *testing.T) {
		published := false
		h := StreamNewTweetHandler(stubTweetBroadcaster{publishFn: func(ownerId, dest string, bt []byte) error {
			published = true
			return nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetRepo{}, stubTimelineRepo{}, stubFollowChecker{following: true}, stubTweetUserRepo{}, stubModerationNotifier{}, stubStreamer{})
		resp, err := h(marshal(t, event.NewTweetEvent{UserId: "other-1", Text: "from friend"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Text != "from friend" {
			t.Fatalf("unexpected response: %v", resp)
		}
		if published {
			t.Fatal("should not broadcast other user's tweet")
		}
	})

	t.Run("unfollowed user tweet - dropped, not stored", func(t *testing.T) {
		created := false
		added := false
		h := StreamNewTweetHandler(
			stubTweetBroadcaster{},
			stubAuth{owner: domain.Owner{UserId: owner}},
			stubTweetRepo{createFn: func(userId string, tweet domain.Tweet) (domain.Tweet, error) {
				created = true
				return tweet, nil
			}},
			stubTimelineRepo{addFn: func(userId string, tweet domain.Tweet) error {
				added = true
				return nil
			}},
			stubFollowChecker{following: false},
			stubTweetUserRepo{},
			stubModerationNotifier{},
			stubStreamer{},
		)
		resp, err := h(marshal(t, event.NewTweetEvent{Id: "x1", UserId: "stranger-1", Text: "unsolicited"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected Accepted ack, got: %v", resp)
		}
		if created {
			t.Fatal("unfollowed author's tweet must not be stored")
		}
		if added {
			t.Fatal("unfollowed author's tweet must not reach the timeline")
		}
	})
}

// TestStreamNewTweetHandler_TweetThenReply exercises both branches of the
// unified handler: a top-level tweet is created and added to the timeline,
// then a reply to it is routed into the thread store (AddReply) and kept out
// of the timeline.
func TestStreamNewTweetHandler_TweetThenReply(t *testing.T) {
	owner := "owner-1"

	var timelineIds []string
	replyStored := false
	repo := stubTweetRepo{
		createFn: func(userId string, tweet domain.Tweet) (domain.Tweet, error) {
			tweet.Id = "tweet-1"
			tweet.RootId = tweet.Id
			return tweet, nil
		},
		addReplyFn: func(reply domain.Tweet) (domain.Tweet, error) {
			replyStored = true
			if reply.Id == "" {
				reply.Id = "reply-1"
			}
			return reply, nil
		},
	}
	timeline := stubTimelineRepo{addFn: func(userId string, tweet domain.Tweet) error {
		timelineIds = append(timelineIds, tweet.Id)
		return nil
	}}
	h := StreamNewTweetHandler(
		stubTweetBroadcaster{},
		stubAuth{owner: domain.Owner{UserId: owner}},
		repo,
		timeline,
		stubFollowChecker{},
		stubTweetUserRepo{},
		stubModerationNotifier{},
		stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}},
	)

	// 1) a top-level tweet: stored and added to the timeline.
	resp, err := h(marshal(t, event.NewTweetEvent{UserId: owner, Text: "hello"}), nil)
	if err != nil {
		t.Fatalf("tweet: unexpected err: %v", err)
	}
	tweet := resp.(domain.Tweet)
	if tweet.IsReply() {
		t.Fatal("top-level tweet must not be a reply")
	}
	if tweet.RootId != tweet.Id {
		t.Fatalf("top-level tweet RootId must equal Id, got root=%q id=%q", tweet.RootId, tweet.Id)
	}
	if len(timelineIds) != 1 || timelineIds[0] != tweet.Id {
		t.Fatalf("tweet must be added to the timeline, got %v", timelineIds)
	}

	// 2) a reply to that tweet: stored in the thread, NOT added to timeline.
	parentUserID := owner
	replyResp, err := h(marshal(t, event.NewTweetEvent{
		UserId:       owner,
		Text:         "a reply",
		ParentId:     &tweet.Id,
		ParentUserId: &parentUserID,
		RootId:       tweet.Id,
	}), nil)
	if err != nil {
		t.Fatalf("reply: unexpected err: %v", err)
	}
	reply := replyResp.(domain.Tweet)
	if !reply.IsReply() {
		t.Fatal("reply must have a parent")
	}
	if reply.RootId != tweet.Id {
		t.Fatalf("reply RootId must be the thread root %q, got %q", tweet.Id, reply.RootId)
	}
	if !replyStored {
		t.Fatal("reply must be stored via AddReply")
	}
	if len(timelineIds) != 1 {
		t.Fatalf("reply must NOT be added to the timeline, ids=%v", timelineIds)
	}
}

func TestStreamGetTweetHandler(t *testing.T) {
	owner := "owner-1"
	tweetId := "tweet-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetTweetHandler(stubTweetRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamGetTweetHandler(stubTweetRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.GetTweetEvent{TweetId: tweetId}), nil)
		if err == nil || err.Error() != "empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty tweet id", func(t *testing.T) {
		h := StreamGetTweetHandler(stubTweetRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.GetTweetEvent{UserId: owner}), nil)
		if err == nil || err.Error() != "empty tweet id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("blocklisted tweet", func(t *testing.T) {
		h := StreamGetTweetHandler(stubTweetRepo{isBlocklistedFn: func(tweetId string) bool { return true }}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.GetTweetEvent{UserId: owner, TweetId: tweetId}), nil)
		if err == nil || err.Error() != "tweet is moderated" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("own tweet - local fetch", func(t *testing.T) {
		h := StreamGetTweetHandler(stubTweetRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetUserRepo{}, stubStreamer{})
		resp, err := h(marshal(t, event.GetTweetEvent{UserId: owner, TweetId: tweetId}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Id != tweetId {
			t.Fatalf("unexpected response: %v", resp)
		}
	})

	t.Run("other user tweet - user not found fallback", func(t *testing.T) {
		h := StreamGetTweetHandler(stubTweetRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubStreamer{})
		resp, err := h(marshal(t, event.GetTweetEvent{UserId: "other-1", TweetId: tweetId}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Text != "cached" {
			t.Fatalf("expected cached tweet fallback: %v", resp)
		}
	})

	t.Run("other user tweet - node offline fallback", func(t *testing.T) {
		h := StreamGetTweetHandler(stubTweetRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetUserRepo{}, stubStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return nil, warpnet.ErrNodeIsOffline
		}})
		resp, err := h(marshal(t, event.GetTweetEvent{UserId: "other-1", TweetId: tweetId}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Text != "cached" {
			t.Fatalf("expected cached tweet fallback: %v", resp)
		}
	})

	t.Run("other user tweet - remote error response falls back to local", func(t *testing.T) {
		errResp, _ := json.Marshal(event.ResponseError{Code: 500, Message: "remote error"})
		h := StreamGetTweetHandler(stubTweetRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetUserRepo{}, stubStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return errResp, nil
		}})
		resp, err := h(marshal(t, event.GetTweetEvent{UserId: "other-1", TweetId: tweetId}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Text != "cached" {
			t.Fatalf("expected local fallback on remote error, got: %v", resp)
		}
	})

	t.Run("other user tweet - remote success", func(t *testing.T) {
		remoteTweet, _ := json.Marshal(domain.Tweet{Id: tweetId, UserId: "other-1", Text: "remote tweet"})
		h := StreamGetTweetHandler(stubTweetRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetUserRepo{}, stubStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return remoteTweet, nil
		}})
		resp, err := h(marshal(t, event.GetTweetEvent{UserId: "other-1", TweetId: tweetId}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(domain.Tweet).Text != "remote tweet" {
			t.Fatalf("expected remote tweet: %v", resp)
		}
	})

	t.Run("other user tweet - stream error", func(t *testing.T) {
		streamErr := errors.New("stream broken")
		h := StreamGetTweetHandler(stubTweetRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetUserRepo{}, stubStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return nil, streamErr
		}})
		_, err := h(marshal(t, event.GetTweetEvent{UserId: "other-1", TweetId: tweetId}), nil)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})
}

func TestStreamGetTweetsHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetTweetsHandler(stubTweetRepo{}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamGetTweetsHandler(stubTweetRepo{}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.GetAllTweetsEvent{}), nil)
		if err == nil || err.Error() != "empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("tweets exist locally - returns immediately", func(t *testing.T) {
		h := StreamGetTweetsHandler(stubTweetRepo{listFn: func(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
			return []domain.Tweet{{Id: "t1", UserId: owner, Text: "hello"}}, "end", nil
		}}, stubTweetUserRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.GetAllTweetsEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.TweetsResponse)
		if len(r.Tweets) != 1 {
			t.Fatalf("expected 1 tweet, got %d", len(r.Tweets))
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamGetTweetsHandler(stubTweetRepo{listFn: func(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
			return nil, "", repoErr
		}}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.GetAllTweetsEvent{UserId: owner}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
}

func TestStreamDeleteTweetHandler(t *testing.T) {
	owner := "owner-1"
	tweetId := "tweet-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamDeleteTweetHandler(stubTweetBroadcaster{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetRepo{}, stubTimelineRepo{}, stubTweetLikeRepo{}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamDeleteTweetHandler(stubTweetBroadcaster{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetRepo{}, stubTimelineRepo{}, stubTweetLikeRepo{}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.DeleteTweetEvent{TweetId: tweetId}), nil)
		if err == nil || err.Error() != "empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty tweet id", func(t *testing.T) {
		h := StreamDeleteTweetHandler(stubTweetBroadcaster{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetRepo{}, stubTimelineRepo{}, stubTweetLikeRepo{}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.DeleteTweetEvent{UserId: owner}), nil)
		if err == nil || err.Error() != "empty tweet id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("delete error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamDeleteTweetHandler(stubTweetBroadcaster{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetRepo{deleteFn: func(userID, tweetID string) error {
			return repoErr
		}}, stubTimelineRepo{}, stubTweetLikeRepo{}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.DeleteTweetEvent{UserId: owner, TweetId: tweetId}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("own tweet delete - broadcasts", func(t *testing.T) {
		published := false
		h := StreamDeleteTweetHandler(stubTweetBroadcaster{publishFn: func(ownerId, dest string, bt []byte) error {
			published = true
			return nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetRepo{}, stubTimelineRepo{}, stubTweetLikeRepo{}, stubTweetUserRepo{}, stubStreamer{})
		resp, err := h(marshal(t, event.DeleteTweetEvent{UserId: owner, TweetId: tweetId}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if !published {
			t.Fatal("expected broadcast to be called")
		}
	})

	t.Run("other user tweet delete - no broadcast", func(t *testing.T) {
		published := false
		h := StreamDeleteTweetHandler(stubTweetBroadcaster{publishFn: func(ownerId, dest string, bt []byte) error {
			published = true
			return nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}}, stubTweetRepo{}, stubTimelineRepo{}, stubTweetLikeRepo{}, stubTweetUserRepo{}, stubStreamer{})
		resp, err := h(marshal(t, event.DeleteTweetEvent{UserId: "other-1", TweetId: tweetId}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if published {
			t.Fatal("should not broadcast other user's tweet delete")
		}
	})
}

func TestStreamGetTweetStatsHandler(t *testing.T) {
	owner := "owner-1"
	tweetId := "tweet-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetTweetStatsHandler(stubTweetRepo{}, stubTweetLikeRepo{}, stubTweetRetweetRepo{}, stubRepliesCounter{}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty tweet id", func(t *testing.T) {
		h := StreamGetTweetStatsHandler(stubTweetRepo{}, stubTweetLikeRepo{}, stubTweetRetweetRepo{}, stubRepliesCounter{}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.GetTweetStatsEvent{UserId: owner}), nil)
		if err == nil || err.Error() != "empty tweet id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamGetTweetStatsHandler(stubTweetRepo{}, stubTweetLikeRepo{}, stubTweetRetweetRepo{}, stubRepliesCounter{}, stubTweetUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.GetTweetStatsEvent{TweetId: tweetId}), nil)
		if err == nil || err.Error() != "empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("own tweet stats - concurrent gathering", func(t *testing.T) {
		h := StreamGetTweetStatsHandler(
			stubTweetRepo{tweetsCountFn: func(userId string) (uint64, error) { return 10, nil }, getViewsCountFn: func(tweetId string) (uint64, error) { return 100, nil }},
			stubTweetLikeRepo{likesCountFn: func(tweetId string) (uint64, error) { return 5, nil }},
			stubTweetRetweetRepo{retweetsCountFn: func(tweetId string) (uint64, error) { return 3, nil }},
			stubRepliesCounter{repliesCountFn: func(tweetId string) (uint64, error) { return 2, nil }},
			stubTweetUserRepo{},
			stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}},
		)
		resp, err := h(marshal(t, event.GetTweetStatsEvent{TweetId: tweetId, UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		stats := resp.(event.TweetStatsResponse)
		if stats.ViewsCount != 100 || stats.LikeCount != 5 || stats.RetweetsCount != 3 || stats.RepliesCount != 2 {
			t.Fatalf("unexpected stats: %+v", stats)
		}
	})

	t.Run("other user tweet stats - user not found", func(t *testing.T) {
		h := StreamGetTweetStatsHandler(stubTweetRepo{}, stubTweetLikeRepo{}, stubTweetRetweetRepo{}, stubRepliesCounter{}, stubTweetUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.GetTweetStatsEvent{TweetId: tweetId, UserId: "other-1"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		stats := resp.(event.TweetStatsResponse)
		if stats.TweetId != tweetId {
			t.Fatalf("expected tweet id in response: %v", stats)
		}
	})

	t.Run("other user tweet stats - node offline", func(t *testing.T) {
		h := StreamGetTweetStatsHandler(stubTweetRepo{}, stubTweetLikeRepo{}, stubTweetRetweetRepo{}, stubRepliesCounter{}, stubTweetUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, warpnet.ErrNodeIsOffline
			},
		})
		resp, err := h(marshal(t, event.GetTweetStatsEvent{TweetId: tweetId, UserId: "other-1"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		stats := resp.(event.TweetStatsResponse)
		if stats.TweetId != tweetId {
			t.Fatalf("expected tweet id in response: %v", stats)
		}
	})

	t.Run("other user tweet stats - remote success", func(t *testing.T) {
		remoteStats, _ := json.Marshal(event.TweetStatsResponse{
			TweetId:      tweetId,
			LikeCount:    42,
			ViewsCount:   1000,
			RepliesCount: 7,
		})
		h := StreamGetTweetStatsHandler(stubTweetRepo{}, stubTweetLikeRepo{}, stubTweetRetweetRepo{}, stubRepliesCounter{}, stubTweetUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return remoteStats, nil
			},
		})
		resp, err := h(marshal(t, event.GetTweetStatsEvent{TweetId: tweetId, UserId: "other-1"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		stats := resp.(event.TweetStatsResponse)
		if stats.LikeCount != 42 {
			t.Fatalf("expected remote stats: %+v", stats)
		}
	})

	t.Run("other user tweet stats - stream error", func(t *testing.T) {
		streamErr := errors.New("broken")
		h := StreamGetTweetStatsHandler(stubTweetRepo{}, stubTweetLikeRepo{}, stubTweetRetweetRepo{}, stubRepliesCounter{}, stubTweetUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, streamErr
			},
		})
		_, err := h(marshal(t, event.GetTweetStatsEvent{TweetId: tweetId, UserId: "other-1"}), nil)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})

	t.Run("own tweet stats - strips retweet prefix", func(t *testing.T) {
		var capturedTweetId string
		h := StreamGetTweetStatsHandler(
			stubTweetRepo{},
			stubTweetLikeRepo{likesCountFn: func(tweetId string) (uint64, error) {
				capturedTweetId = tweetId
				return 0, nil
			}},
			stubTweetRetweetRepo{},
			stubRepliesCounter{},
			stubTweetUserRepo{},
			stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}},
		)
		_, err := h(marshal(t, event.GetTweetStatsEvent{TweetId: domain.RetweetPrefix + tweetId, UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if capturedTweetId != tweetId {
			t.Fatalf("expected stripped tweet id %q, got %q", tweetId, capturedTweetId)
		}
	})
}

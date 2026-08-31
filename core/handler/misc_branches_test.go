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
	"github.com/stretchr/testify/require"
)

func TestStreamTimelineNewTweetHandlerBranches(t *testing.T) {
	const owner, author = "owner-1", "author-1"
	nodeId, s := nodeIdentity(t)

	authorRepo := stubTweetUserRepo{getFn: func(id string) (domain.User, error) {
		return domain.User{Id: id, NodeId: nodeId.String()}, nil
	}}
	auth := stubAuth{owner: domain.Owner{UserId: owner}}
	newTweet := event.NewTweetEvent{Id: "tweet-1", UserId: author, Text: "hello", Username: "author"}

	handler := func(repo stubTweetRepo, timeline stubTimelineRepo, follows stubFollowChecker) warpnet.WarpHandlerFunc {
		return StreamTimelineNewTweetHandler(auth, repo, timeline, follows, authorRepo)
	}

	t.Run("invalid payload", func(t *testing.T) {
		_, err := handler(stubTweetRepo{}, stubTimelineRepo{}, stubFollowChecker{})([]byte("{"), s)
		require.Error(t, err)
	})

	t.Run("foreign author is rejected", func(t *testing.T) {
		foreign := stubTweetUserRepo{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "another-node"}, nil
		}}
		_, err := StreamTimelineNewTweetHandler(auth, stubTweetRepo{}, stubTimelineRepo{}, stubFollowChecker{}, foreign)(
			marshal(t, newTweet), s)
		require.ErrorIs(t, err, warpnet.ErrForeignAuthor)
	})

	t.Run("a moderated tweet is blocklisted instead of stored", func(t *testing.T) {
		blocklisted := ""
		repo := stubTweetRepo{blocklistFn: func(id string) error {
			blocklisted = id
			return nil
		}}
		fail := domain.FAIL
		moderated := newTweet
		moderated.Moderation = &domain.TweetModeration{IsOk: fail}

		_, err := handler(repo, stubTimelineRepo{}, stubFollowChecker{})(marshal(t, moderated), s)
		require.NoError(t, err)
		require.Equal(t, "tweet-1", blocklisted)
	})

	t.Run("an invalid tweet is rejected", func(t *testing.T) {
		empty := newTweet
		empty.Text = ""
		_, err := handler(stubTweetRepo{}, stubTimelineRepo{}, stubFollowChecker{})(marshal(t, empty), s)
		require.Error(t, err)
	})

	t.Run("the owner's own tweet is acknowledged, not re-stored", func(t *testing.T) {
		own := newTweet
		own.UserId = owner

		out, err := handler(stubTweetRepo{}, stubTimelineRepo{}, stubFollowChecker{})(marshal(t, own), s)
		require.NoError(t, err)
		require.Equal(t, event.Accepted, out)
	})

	t.Run("a stranger's tweet is acknowledged, not stored", func(t *testing.T) {
		out, err := handler(stubTweetRepo{}, stubTimelineRepo{}, stubFollowChecker{})(marshal(t, newTweet), s)
		require.NoError(t, err)
		require.Equal(t, event.Accepted, out)
	})

	following := stubFollowChecker{following: true}

	t.Run("a store failure surfaces", func(t *testing.T) {
		repoErr := errors.New("store down")
		repo := stubTweetRepo{createFn: func(string, domain.Tweet) (domain.Tweet, error) {
			return domain.Tweet{}, repoErr
		}}
		_, err := handler(repo, stubTimelineRepo{}, following)(marshal(t, newTweet), s)
		require.ErrorIs(t, err, repoErr)
	})

	t.Run("a tweet stored without an id is rejected", func(t *testing.T) {
		repo := stubTweetRepo{createFn: func(_ string, tw domain.Tweet) (domain.Tweet, error) {
			tw.Id = ""
			return tw, nil
		}}
		_, err := handler(repo, stubTimelineRepo{}, following)(marshal(t, newTweet), s)
		require.Error(t, err)
	})

	t.Run("a timeline failure is tolerated", func(t *testing.T) {
		timeline := stubTimelineRepo{addFn: func(string, domain.Tweet) error {
			return errors.New("timeline down")
		}}
		out, err := handler(stubTweetRepo{}, timeline, following)(marshal(t, newTweet), s)
		require.NoError(t, err)
		require.NotEmpty(t, out.(domain.Tweet).Id)
	})
}

func TestStreamUnsubscribeUserHandlerBranches(t *testing.T) {
	const self, target = "self-1", "target-1"

	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamUnsubscribeUserHandler(stubSubscriptionRepo{})([]byte("{"), nil)
		require.Error(t, err)
	})

	t.Run("empty ids", func(t *testing.T) {
		h := StreamUnsubscribeUserHandler(stubSubscriptionRepo{})
		_, err := h(marshal(t, event.UnsubscribeUserEvent{TargetId: target}), nil)
		require.Error(t, err)
		_, err = h(marshal(t, event.UnsubscribeUserEvent{SelfId: self}), nil)
		require.Error(t, err)
	})

	t.Run("a store failure surfaces", func(t *testing.T) {
		repoErr := errors.New("store down")
		h := StreamUnsubscribeUserHandler(stubSubscriptionRepo{
			unsubscribeFn: func(string, string) error { return repoErr },
		})
		_, err := h(marshal(t, event.UnsubscribeUserEvent{SelfId: self, TargetId: target}), nil)
		require.ErrorIs(t, err, repoErr)
	})

	t.Run("accepted", func(t *testing.T) {
		out, err := StreamUnsubscribeUserHandler(stubSubscriptionRepo{})(
			marshal(t, event.UnsubscribeUserEvent{SelfId: self, TargetId: target}), nil)
		require.NoError(t, err)
		require.Equal(t, event.Accepted, out)
	})
}

type stubSubscriptionRepo struct {
	subscribeFn   func(selfId, targetId string) error
	unsubscribeFn func(selfId, targetId string) error
}

func (s stubSubscriptionRepo) Subscribe(selfId, targetId string) error {
	if s.subscribeFn != nil {
		return s.subscribeFn(selfId, targetId)
	}
	return nil
}

func (s stubSubscriptionRepo) Unsubscribe(selfId, targetId string) error {
	if s.unsubscribeFn != nil {
		return s.unsubscribeFn(selfId, targetId)
	}
	return nil
}

func TestForwardToOwnerBranches(t *testing.T) {
	nodeId, _ := nodeIdentity(t)
	selfInfo := warpnet.NodeInfo{ID: nodeId, OwnerId: "owner-1"}
	ev := event.GetTweetReactorsEvent{TweetId: "tweet-1", OwnerUserId: "author-1"}

	t.Run("no owner or no streamer stays local", func(t *testing.T) {
		_, ok, err := forwardToOwner("", &fakeEngagementStreamer{}, stubReactedUserFetcher{}, event.PUBLIC_GET_TWEET_REACTORS, ev)
		require.NoError(t, err)
		require.False(t, ok)

		_, ok, err = forwardToOwner("author-1", nil, stubReactedUserFetcher{}, event.PUBLIC_GET_TWEET_REACTORS, ev)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("the owner is this node", func(t *testing.T) {
		_, ok, err := forwardToOwner(selfInfo.OwnerId, &fakeEngagementStreamer{info: selfInfo},
			stubReactedUserFetcher{}, event.PUBLIC_GET_TWEET_REACTORS, ev)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("an unknown owner stays local", func(t *testing.T) {
		users := stubReactedUserFetcher{getFn: func(string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}
		_, ok, err := forwardToOwner("author-1", &fakeEngagementStreamer{info: selfInfo},
			users, event.PUBLIC_GET_TWEET_REACTORS, ev)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("a lookup failure surfaces", func(t *testing.T) {
		lookupErr := errors.New("store down")
		users := stubReactedUserFetcher{getFn: func(string) (domain.User, error) {
			return domain.User{}, lookupErr
		}}
		_, _, err := forwardToOwner("author-1", &fakeEngagementStreamer{info: selfInfo},
			users, event.PUBLIC_GET_TWEET_REACTORS, ev)
		require.ErrorIs(t, err, lookupErr)
	})

	t.Run("an owner hosted here stays local", func(t *testing.T) {
		users := stubReactedUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: nodeId.String()}, nil
		}}
		_, ok, err := forwardToOwner("author-1", &fakeEngagementStreamer{info: selfInfo},
			users, event.PUBLIC_GET_TWEET_REACTORS, ev)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("a remote page is used", func(t *testing.T) {
		users := stubReactedUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "other-node"}, nil
		}}
		body, err := json.Marshal(event.UsersResponse{Users: []domain.User{{Id: "reactor-1"}}})
		require.NoError(t, err)

		out, ok, err := forwardToOwner("author-1",
			&fakeEngagementStreamer{info: selfInfo, response: body},
			users, event.PUBLIC_GET_TWEET_REACTORS, ev)
		require.NoError(t, err)
		require.True(t, ok)
		require.Len(t, out.Users, 1)
	})
}

func TestEngagementHandlersSurfaceStoreFailures(t *testing.T) {
	repoErr := errors.New("index down")

	_, err := StreamGetTweetReactorsHandler(
		stubReactorsRepo{reactorsFn: func(string, *uint64, *string) ([]string, string, error) {
			return nil, "", repoErr
		}}, stubReactedUserFetcher{}, &fakeEngagementStreamer{},
	)(marshal(t, event.GetTweetReactorsEvent{TweetId: "tweet-1"}), nil)
	require.ErrorIs(t, err, repoErr)

	_, err = StreamGetTweetRetweetersHandler(
		stubRetweetersRepo{retweetersFn: func(string, *uint64, *string) ([]string, string, error) {
			return nil, "", repoErr
		}}, stubReactedUserFetcher{}, &fakeEngagementStreamer{},
	)(marshal(t, event.GetTweetRetweetersEvent{TweetId: "tweet-1"}), nil)
	require.ErrorIs(t, err, repoErr)
}

var _ = stream.WarpRoute("")

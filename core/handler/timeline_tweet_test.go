// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamTimelineTweetHandler(t *testing.T) {
	t.Parallel()

	const owner = "owner-1"
	ev := event.NewTweetEvent{Id: "t1", UserId: "friend-1", Text: "hello"}

	newHandler := func(users TimelineUserFetcher, following bool, created, timelined *bool) warpnet.WarpHandlerFunc {
		return StreamTimelineNewTweetHandler(
			stubAuth{owner: domain.Owner{UserId: owner}},
			stubTweetRepo{createFn: func(_ string, tweet domain.Tweet) (domain.Tweet, error) {
				*created = true
				return tweet, nil
			}},
			stubTimelineRepo{addFn: func(string, domain.Tweet) error {
				*timelined = true
				return nil
			}},
			stubFollowChecker{following: following},
			users,
			stubPeersRatings{})
	}

	t.Run("sent by its author's node", func(t *testing.T) {
		t.Parallel()
		users, conn := authorStream(t)

		var created, timelined bool
		_, err := newHandler(users, true, &created, &timelined)(marshal(t, ev), conn)

		require.NoError(t, err)
		assert.True(t, created)
		assert.True(t, timelined)
	})

	t.Run("sent by another node", func(t *testing.T) {
		t.Parallel()
		users, _ := authorStream(t)
		_, attacker := authorStream(t)

		var created, timelined bool
		_, err := newHandler(users, true, &created, &timelined)(marshal(t, ev), attacker)

		require.ErrorIs(t, err, warpnet.ErrForeignAuthor)
		assert.False(t, created)
		assert.False(t, timelined)
	})

	t.Run("no sender", func(t *testing.T) {
		t.Parallel()
		users, _ := authorStream(t)

		var created, timelined bool
		_, err := newHandler(users, true, &created, &timelined)(marshal(t, ev), nil)

		require.ErrorIs(t, err, warpnet.ErrForeignAuthor)
		assert.False(t, created)
	})

	t.Run("an author the owner does not follow", func(t *testing.T) {
		t.Parallel()
		users, conn := authorStream(t)

		var created, timelined bool
		resp, err := newHandler(users, false, &created, &timelined)(marshal(t, ev), conn)

		require.NoError(t, err)
		assert.Equal(t, event.Accepted, resp)
		assert.False(t, created, "an unsolicited tweet must not enter the timeline")
	})
}

func TestStreamTimelineTweetHandler_FlooredPeerMedia(t *testing.T) {
	t.Parallel()

	video := "video-1"
	ev := event.NewTweetEvent{Id: "t1", UserId: "friend-1", Text: "hello", ImageKeys: []string{"image-1"}, VideoKey: &video}

	storeTweet := func(t *testing.T, isFloor bool) domain.Tweet {
		t.Helper()
		users, conn := authorStream(t)
		ratings := stubPeersRatings{}
		if isFloor {
			ratings.floored = conn.Conn().RemotePeer()
		}

		var stored domain.Tweet
		_, err := StreamTimelineNewTweetHandler(
			stubAuth{owner: domain.Owner{UserId: "owner-1"}},
			stubTweetRepo{createFn: func(_ string, tweet domain.Tweet) (domain.Tweet, error) {
				stored = tweet
				return tweet, nil
			}},
			stubTimelineRepo{},
			stubFollowChecker{following: true},
			users,
			ratings,
		)(marshal(t, ev), conn)
		require.NoError(t, err)
		return stored
	}

	t.Run("a floored peer's tweet is stored without media", func(t *testing.T) {
		t.Parallel()
		stored := storeTweet(t, true)

		assert.Equal(t, "hello", stored.Text)
		assert.Empty(t, stored.ImageKeys)
		assert.Nil(t, stored.VideoKey)
	})

	t.Run("a peer above the floor keeps its media", func(t *testing.T) {
		t.Parallel()
		stored := storeTweet(t, false)

		assert.Equal(t, []string{"image-1"}, stored.ImageKeys)
		require.NotNil(t, stored.VideoKey)
		assert.Equal(t, video, *stored.VideoKey)
	})
}

func TestStreamTimelineDeleteTweetHandler(t *testing.T) {
	t.Parallel()

	const owner = "owner-1"
	ev := event.DeleteTweetEvent{UserId: "friend-1", TweetId: "t1"}

	newHandler := func(users TimelineUserFetcher, deleteErr error, deleted, untimelined *string) warpnet.WarpHandlerFunc {
		return StreamTimelineDeleteTweetHandler(
			stubAuth{owner: domain.Owner{UserId: owner}},
			stubTweetRepo{deleteFn: func(userId, tweetId string) error {
				*deleted = userId + "/" + tweetId
				return deleteErr
			}},
			stubTimelineRepo{deleteFn: func(userId, tweetId string) error {
				*untimelined = userId + "/" + tweetId
				return nil
			}},
			users)
	}

	t.Run("sent by its author's node", func(t *testing.T) {
		t.Parallel()
		users, conn := authorStream(t)

		var deleted, untimelined string
		resp, err := newHandler(users, nil, &deleted, &untimelined)(marshal(t, ev), conn)

		require.NoError(t, err)
		assert.Equal(t, event.Accepted, resp)
		assert.Equal(t, "friend-1/t1", deleted)
		assert.Equal(t, owner+"/t1", untimelined, "the tweet leaves the owner's timeline, not the author's")
	})

	t.Run("sent by another node", func(t *testing.T) {
		t.Parallel()
		users, _ := authorStream(t)
		_, attacker := authorStream(t)

		var deleted, untimelined string
		_, err := newHandler(users, nil, &deleted, &untimelined)(marshal(t, ev), attacker)

		require.ErrorIs(t, err, warpnet.ErrForeignAuthor)
		assert.Empty(t, deleted)
		assert.Empty(t, untimelined)
	})

	t.Run("the owner's own tweet", func(t *testing.T) {
		t.Parallel()
		users, conn := authorStream(t)

		var deleted, untimelined string
		resp, err := newHandler(users, nil, &deleted, &untimelined)(
			marshal(t, event.DeleteTweetEvent{UserId: owner, TweetId: "t1"}), conn)

		require.NoError(t, err)
		assert.Equal(t, event.Accepted, resp)
		assert.Empty(t, deleted)
		assert.Empty(t, untimelined)
	})

	t.Run("a tweet this node does not hold", func(t *testing.T) {
		t.Parallel()
		users, conn := authorStream(t)
		missing := errors.New("tweet not found")

		var deleted, untimelined string
		_, err := newHandler(users, missing, &deleted, &untimelined)(marshal(t, ev), conn)

		require.ErrorIs(t, err, missing)
		assert.Empty(t, untimelined, "a tweet the author does not own here must stay in the timeline")
	})
}

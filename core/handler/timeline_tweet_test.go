// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
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

	newHandler := func(users TweetUserFetcher, following bool, created, timelined *bool) warpnet.WarpHandlerFunc {
		return StreamTimelineTweetHandler(
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
			users)
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

		require.ErrorIs(t, err, ErrForeignTweetAuthor)
		assert.False(t, created)
		assert.False(t, timelined)
	})

	t.Run("no sender", func(t *testing.T) {
		t.Parallel()
		users, _ := authorStream(t)

		var created, timelined bool
		_, err := newHandler(users, true, &created, &timelined)(marshal(t, ev), nil)

		require.ErrorIs(t, err, ErrForeignTweetAuthor)
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

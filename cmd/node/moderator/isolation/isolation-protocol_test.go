//nolint:all
package isolation

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/stretchr/testify/require"
)

type recordingPublisher struct {
	err        error
	ownerIds   []string
	dests      []string
	published  []event.ModerationVerdictEvent
	callsCount int
}

func (p *recordingPublisher) PublishUpdateToFollowers(ownerId, dest string, body any) error {
	p.callsCount++
	p.ownerIds = append(p.ownerIds, ownerId)
	p.dests = append(p.dests, dest)
	if ev, ok := body.(event.ModerationVerdictEvent); ok {
		p.published = append(p.published, ev)
	}
	return p.err
}

func testKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return priv
}

func TestIsolateTweet(t *testing.T) {
	reason := "spam"
	tweet := &domain.Tweet{Id: "tweet-1", UserId: "author-1"}
	moderation := &domain.TweetModeration{
		ModeratorID: "moderator-1",
		Model:       "llama-guard",
		IsOk:        domain.FAIL,
		Reason:      &reason,
	}
	voters := []domain.ID{"moderator-1", "moderator-2"}

	t.Run("publishes a signed verdict to the author's followers", func(t *testing.T) {
		pub := &recordingPublisher{}
		NewIsolationProtocol(pub, testKey(t)).IsolateTweet(tweet, moderation, voters)

		require.Equal(t, 1, pub.callsCount)
		require.Equal(t, []string{"author-1"}, pub.ownerIds)
		require.Equal(t, []string{event.PUBLIC_POST_MODERATION_RESULT}, pub.dests)

		require.Len(t, pub.published, 1)
		got := pub.published[0]
		require.Equal(t, domain.ModerationTweetType, got.Type)
		require.Equal(t, domain.ID("author-1"), got.UserID)
		require.NotNil(t, got.ObjectID)
		require.Equal(t, domain.ID("tweet-1"), *got.ObjectID)
		require.Equal(t, voters, got.Voters)
		require.NotEmpty(t, got.Signature, "the verdict must carry the moderator's signature")
	})

	t.Run("nil inputs publish nothing", func(t *testing.T) {
		pub := &recordingPublisher{}
		ip := NewIsolationProtocol(pub, testKey(t))
		ip.IsolateTweet(nil, moderation, voters)
		ip.IsolateTweet(tweet, nil, voters)
		require.Zero(t, pub.callsCount)
	})

	t.Run("a publish failure is logged, not returned", func(t *testing.T) {
		pub := &recordingPublisher{err: errors.New("no subscribers")}
		NewIsolationProtocol(pub, testKey(t)).IsolateTweet(tweet, moderation, voters)
		require.Equal(t, 1, pub.callsCount)
	})
}

func TestIsolateUser(t *testing.T) {
	reason := "abuse"
	user := &domain.User{Id: "user-1"}
	moderation := &domain.UserModeration{
		Model:  "llama-guard",
		IsOk:   false,
		Reason: &reason,
	}
	voters := []domain.ID{"moderator-1"}

	t.Run("publishes a signed verdict on the user's own topic", func(t *testing.T) {
		pub := &recordingPublisher{}
		NewIsolationProtocol(pub, testKey(t)).IsolateUser("moderator-1", user, moderation, voters)

		require.Equal(t, 1, pub.callsCount)
		require.Equal(t, []string{"user-1"}, pub.ownerIds)

		require.Len(t, pub.published, 1)
		got := pub.published[0]
		require.Equal(t, domain.ModerationUserType, got.Type)
		require.Equal(t, domain.ID("user-1"), got.UserID)
		require.Equal(t, domain.ID("moderator-1"), got.ModeratorID)
		require.Nil(t, got.ObjectID, "a user verdict names no object")
		require.NotEmpty(t, got.Signature)
	})

	t.Run("nil inputs publish nothing", func(t *testing.T) {
		pub := &recordingPublisher{}
		ip := NewIsolationProtocol(pub, testKey(t))
		ip.IsolateUser("moderator-1", nil, moderation, voters)
		ip.IsolateUser("moderator-1", user, nil, voters)
		require.Zero(t, pub.callsCount)
	})

	t.Run("a publish failure is logged, not returned", func(t *testing.T) {
		pub := &recordingPublisher{err: errors.New("no subscribers")}
		NewIsolationProtocol(pub, testKey(t)).IsolateUser("moderator-1", user, moderation, voters)
		require.Equal(t, 1, pub.callsCount)
	})
}

package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestError_Error(t *testing.T) {
	e := &Error{Code: 404, Message: "not found"}
	assert.Equal(t, "not found", e.Error())
	assert.Equal(t, 404, e.Code)
}

func TestTweet_IsModerated(t *testing.T) {
	tweet := Tweet{Id: "t1"}
	assert.False(t, tweet.IsModerated())

	tweet.Moderation = &TweetModeration{ModeratorID: "mod1"}
	assert.True(t, tweet.IsModerated())
}

func TestRetweetPrefix(t *testing.T) {
	assert.Equal(t, "RT:", RetweetPrefix)
}

func TestNotificationType_String(t *testing.T) {
	assert.Equal(t, "moderation", NotificationModerationType.String())
	assert.Equal(t, "retweet", NotificationRetweetType.String())
	assert.Equal(t, "follow", NotificationFollowType.String())
	assert.Equal(t, "like", NotificationLikeType.String())
	assert.Equal(t, "mention", NotificationMentionType.String())
	assert.Equal(t, "reply", NotificationReplyType.String())
}

func TestModerationResult(t *testing.T) {
	assert.Equal(t, ModerationResult(true), OK)
	assert.Equal(t, ModerationResult(false), FAIL)
}

func TestModerationObjectType_String(t *testing.T) {
	assert.Equal(t, "user description", ModerationUserType.String())
	assert.Equal(t, "tweet text", ModerationTweetType.String())
	assert.Equal(t, "reply text", ModerationReplyType.String())
	assert.Equal(t, "image content", ModerationImageType.String())
	assert.Equal(t, "unknown", ModerationObjectType(99).String())
}

func TestModelType(t *testing.T) {
	assert.Equal(t, ModelType("llama2"), LLAMA2)
}

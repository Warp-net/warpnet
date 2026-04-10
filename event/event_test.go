package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAcceptedResponse(t *testing.T) {
	assert.Equal(t, `{"code":0,"message":"Accepted"}`, string(Accepted))
}

func TestInternalRoutePrefix(t *testing.T) {
	assert.Equal(t, "/internal", InternalRoutePrefix)
}

func TestEndCursor(t *testing.T) {
	assert.Equal(t, "end", EndCursor)
}

func TestResponseError_Error(t *testing.T) {
	err := ResponseError{Code: 500, Message: "internal error"}
	assert.Equal(t, "internal error", err.Error())
	assert.Equal(t, 500, err.Code)
}

func TestValidationResult_String(t *testing.T) {
	assert.Equal(t, "invalid", Invalid.String())
	assert.Equal(t, "valid", Valid.String())
}

func TestValidationResult_Values(t *testing.T) {
	assert.Equal(t, ValidationResult(0), Invalid)
	assert.Equal(t, ValidationResult(1), Valid)
}

func TestPaths_PrivateRoutes(t *testing.T) {
	privateRoutes := []string{
		PRIVATE_POST_PAIR,
		PRIVATE_GET_STATS,
		PRIVATE_DELETE_CHAT,
		PRIVATE_DELETE_MESSAGE,
		PRIVATE_DELETE_TWEET,
		PRIVATE_GET_CHAT,
		PRIVATE_GET_CHATS,
		PRIVATE_GET_NOTIFICATIONS,
		PRIVATE_GET_MESSAGE,
		PRIVATE_GET_MESSAGES,
		PRIVATE_GET_TIMELINE,
		PRIVATE_POST_LOGIN,
		PRIVATE_POST_LOGOUT,
		PRIVATE_POST_TWEET,
		PRIVATE_POST_USER,
		PRIVATE_POST_UPLOAD_IMAGE,
	}
	for _, r := range privateRoutes {
		assert.Contains(t, r, "/private/")
	}
}

func TestPaths_PublicRoutes(t *testing.T) {
	publicRoutes := []string{
		PUBLIC_POST_NODE_CHALLENGE,
		PUBLIC_POST_MODERATION_RESULT,
		PUBLIC_DELETE_REPLY,
		PUBLIC_GET_FOLLOWINGS,
		PUBLIC_GET_FOLLOWERS,
		PUBLIC_GET_INFO,
		PUBLIC_GET_REPLIES,
		PUBLIC_GET_REPLY,
		PUBLIC_GET_TWEET,
		PUBLIC_GET_TWEET_STATS,
		PUBLIC_GET_TWEETS,
		PUBLIC_GET_USER,
		PUBLIC_GET_USERS,
		PUBLIC_GET_WHOTOFOLLOW,
		PUBLIC_POST_CHAT,
		PUBLIC_POST_FOLLOW,
		PUBLIC_POST_LIKE,
		PUBLIC_POST_MESSAGE,
		PUBLIC_POST_REPLY,
		PUBLIC_POST_RETWEET,
		PUBLIC_POST_IS_FOLLOWING,
		PUBLIC_POST_IS_FOLLOWER,
		PUBLIC_POST_UNFOLLOW,
		PUBLIC_POST_UNLIKE,
		PUBLIC_POST_UNRETWEET,
		PUBLIC_GET_IMAGE,
	}
	for _, r := range publicRoutes {
		assert.Contains(t, r, "/public/")
	}
}

func TestPaths_VersionSuffix(t *testing.T) {
	routes := []string{
		PRIVATE_POST_PAIR,
		PUBLIC_GET_INFO,
		PRIVATE_POST_LOGIN,
		PUBLIC_POST_LIKE,
	}
	for _, r := range routes {
		assert.Contains(t, r, "/0.0.0")
	}
}

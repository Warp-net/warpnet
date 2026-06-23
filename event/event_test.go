package event

import (
	"strings"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
)

func TestAcceptedResponse(t *testing.T) {
	assert.Equal(t, `{"code":0,"message":"Accepted"}`, Accepted)
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
		PUBLIC_GET_FOLLOWINGS,
		PUBLIC_GET_FOLLOWERS,
		PUBLIC_GET_INFO,
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
		PUBLIC_POST_RETWEET,
		PUBLIC_POST_IS_FOLLOWING,
		PUBLIC_POST_IS_FOLLOWER,
		PUBLIC_POST_UNFOLLOW,
		PUBLIC_POST_UNLIKE,
		PUBLIC_POST_UNRETWEET,
		PUBLIC_POST_VIEW,
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

// TestMessage_RoundTrip guards the wire contract of the envelope every node
// exchanges. The Destination field is serialized under the JSON key "path"
// (a known historical quirk), so a regression that renames the tag would
// silently route nothing — assert the key explicitly, not just the round-trip.
func TestMessage_RoundTrip(t *testing.T) {
	ts := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	in := Message{
		Body:        json.RawMessage(`{"hello":"world"}`),
		MessageId:   "msg-1",
		NodeId:      "node-1",
		Destination: PUBLIC_POST_LIKE,
		Timestamp:   ts,
		Version:     "0.7.224",
		Signature:   "sig",
	}

	data, err := json.Marshal(in)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"path":"`+PUBLIC_POST_LIKE+`"`)
	assert.NotContains(t, string(data), `"destination"`)

	var out Message
	assert.NoError(t, json.Unmarshal(data, &out))
	assert.Equal(t, in.MessageId, out.MessageId)
	assert.Equal(t, in.NodeId, out.NodeId)
	assert.Equal(t, in.Destination, out.Destination)
	assert.Equal(t, in.Version, out.Version)
	assert.Equal(t, in.Signature, out.Signature)
	assert.JSONEq(t, string(in.Body), string(out.Body))
	assert.True(t, in.Timestamp.Equal(out.Timestamp))
}

// TestGetAllTweetsEvent_OmitEmpty verifies that optional pagination fields are
// omitted from the wire when nil. A missing omitempty would emit `"cursor":null`
// and break clients (e.g. Moshi) that decode null into a blank value.
func TestGetAllTweetsEvent_OmitEmpty(t *testing.T) {
	data, err := json.Marshal(GetAllTweetsEvent{UserId: "user-1"})
	assert.NoError(t, err)
	assert.NotContains(t, string(data), "cursor")
	assert.NotContains(t, string(data), "limit")
	assert.Contains(t, string(data), `"user_id":"user-1"`)

	cursor := "next"
	limit := uint64(20)
	in := GetAllTweetsEvent{UserId: "user-1", Cursor: &cursor, Limit: &limit}
	data, err = json.Marshal(in)
	assert.NoError(t, err)

	var out GetAllTweetsEvent
	assert.NoError(t, json.Unmarshal(data, &out))
	assert.Equal(t, in.UserId, out.UserId)
	assert.NotNil(t, out.Cursor)
	assert.Equal(t, cursor, *out.Cursor)
	assert.NotNil(t, out.Limit)
	assert.Equal(t, limit, *out.Limit)
}

// TestLikeEvent_RoundTrip pins the snake_case wire keys the clients rely on.
func TestLikeEvent_RoundTrip(t *testing.T) {
	in := LikeEvent{TweetId: "tweet-1", UserId: "user-1", OwnerId: "owner-1"}

	data, err := json.Marshal(in)
	assert.NoError(t, err)
	for _, key := range []string{`"tweet_id"`, `"user_id"`, `"owner_id"`} {
		assert.True(t, strings.Contains(string(data), key), "missing key %s", key)
	}

	var out LikeEvent
	assert.NoError(t, json.Unmarshal(data, &out))
	assert.Equal(t, in, out)
}

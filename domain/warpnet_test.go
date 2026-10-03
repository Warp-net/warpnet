package domain

import (
	"math/big"
	"testing"

	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Equal(t, "reaction", NotificationReactionType.String())
	assert.Equal(t, "mention", NotificationMentionType.String())
	assert.Equal(t, "reply", NotificationReplyType.String())
	assert.Equal(t, "order_limit", NotificationOrderLimitType.String())
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

func TestPrice_AmountFollowsUnits(t *testing.T) {
	for units, amount := range map[string]string{
		"1":         "0.000001",
		"1500000":   "1.5",
		"10000000":  "10",
		"123456789": "123.456789",
	} {
		var p Price
		require.NoError(t, json.Unmarshal([]byte(`{"units":`+units+`}`), &p))
		assert.Equal(t, amount, p.Amount)
	}
}

func TestPrice_UnmarshalJSON(t *testing.T) {
	var tweet Tweet
	require.NoError(t, json.Unmarshal([]byte(`{"id":"t1","price":{"amount":"0.01","units":1500000}}`), &tweet))
	require.NotNil(t, tweet.Price)
	assert.Equal(t, "1.5", tweet.Price.Amount)
	assert.Equal(t, int64(1500000), tweet.Price.Units.Int64())

	var free Tweet
	require.NoError(t, json.Unmarshal([]byte(`{"id":"t2"}`), &free))
	assert.False(t, free.IsSponsored())

	var empty Price
	require.NoError(t, json.Unmarshal([]byte(`{"amount":"1.5"}`), &empty))
	assert.False(t, empty.IsPositive())
}

func TestPrice_MarshalJSON(t *testing.T) {
	bt, err := json.Marshal(Tweet{Id: "t1", Price: &Price{Amount: "1.5", Units: big.NewInt(1500000)}})
	require.NoError(t, err)
	assert.Contains(t, string(bt), `"price":{"amount":"1.5","units":1500000}`)
}

func TestTweet_Teaser(t *testing.T) {
	video := "v1"
	paid := Tweet{
		Id:        "t1",
		Text:      "paid",
		ImageKeys: []string{"i1"},
		VideoKey:  &video,
		Poll:      &Poll{Options: []string{"a", "b"}},
		Price:     &Price{Amount: "1.5", Units: big.NewInt(1500000)},
	}
	teaser := paid.Teaser()
	assert.Empty(t, teaser.Text)
	assert.Nil(t, teaser.ImageKeys)
	assert.Nil(t, teaser.VideoKey)
	assert.Nil(t, teaser.Poll)
	assert.Equal(t, "1.5", teaser.Price.Amount)
	assert.Equal(t, "paid", paid.Text)

	free := Tweet{Id: "t2", Text: "free", ImageKeys: []string{"i2"}}
	assert.Equal(t, free, free.Teaser())
}

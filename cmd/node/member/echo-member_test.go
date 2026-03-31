//go:build echo

package main

import (
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/require"
)

type streamCall struct {
	nodeID string
	path   stream.WarpRoute
	data   any
}

type fakeEchoNode struct {
	info  warpnet.NodeInfo
	calls []streamCall
}

func (f *fakeEchoNode) GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
	f.calls = append(f.calls, streamCall{nodeID: nodeId, path: path, data: data})
	return []byte(`{"accepted":true}`), nil
}

func (f *fakeEchoNode) NodeInfo() warpnet.NodeInfo { return f.info }

func TestEchoAutoActionsOnForeignTweet(t *testing.T) {
	f := &fakeEchoNode{info: warpnet.NodeInfo{OwnerId: "echo-owner", ID: warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")}}
	bot := newEchoBot(f)

	tweet := event.NewTweetEvent{Id: "tweet-1", RootId: "tweet-1", UserId: "foreign", Username: "alice", Text: "hello", CreatedAt: time.Now()}
	bt, err := json.Marshal(tweet)
	require.NoError(t, err)

	bot.handleTweet(bt)
	require.Len(t, f.calls, 3)
	require.Equal(t, event.PUBLIC_POST_LIKE, string(f.calls[0].path))
	require.Equal(t, event.PUBLIC_POST_RETWEET, string(f.calls[1].path))
	require.Equal(t, event.PUBLIC_POST_REPLY, string(f.calls[2].path))

	bot.handleTweet(bt)
	require.Len(t, f.calls, 3, "same tweet should be deduplicated in memory")
}

func TestEchoAutoReplyOnForeignReply(t *testing.T) {
	f := &fakeEchoNode{info: warpnet.NodeInfo{OwnerId: "echo-owner", ID: warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")}}
	bot := newEchoBot(f)

	parentID := "tweet-1"
	rp := event.NewReplyEvent{Id: "reply-1", RootId: "tweet-1", ParentId: &parentID, UserId: "foreign", ParentUserId: "foreign", Text: "reply"}
	bt, err := json.Marshal(rp)
	require.NoError(t, err)

	bot.handleReply(bt)
	require.Len(t, f.calls, 1)
	require.Equal(t, event.PUBLIC_POST_REPLY, string(f.calls[0].path))
}

func TestEchoAutoFollowBack(t *testing.T) {
	f := &fakeEchoNode{info: warpnet.NodeInfo{OwnerId: "echo-owner", ID: warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")}}
	bot := newEchoBot(f)

	bt, err := json.Marshal(event.NewFollowEvent{FollowerId: "foreign", FollowingId: "echo-owner"})
	require.NoError(t, err)

	bot.handleFollow(bt)
	require.Len(t, f.calls, 1)
	require.Equal(t, event.PUBLIC_POST_FOLLOW, string(f.calls[0].path))
}

func TestEchoAutoReplyOnChatMessage(t *testing.T) {
	f := &fakeEchoNode{info: warpnet.NodeInfo{OwnerId: "echo-owner", ID: warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")}}
	bot := newEchoBot(f)

	msg := event.NewMessageEvent{Id: "msg-1", ChatId: "chat-1", SenderId: "foreign", ReceiverId: "echo-owner", Text: "ping"}
	bt, err := json.Marshal(msg)
	require.NoError(t, err)

	bot.handleMessage(bt)
	require.Len(t, f.calls, 1)
	require.Equal(t, event.PUBLIC_POST_MESSAGE, string(f.calls[0].path))
}

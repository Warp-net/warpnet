//go:build echo

package main

import (
	"errors"
	"strings"
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
	if nodeId == f.info.ID.String() {
		return nil, errors.New("self stream request is forbidden")
	}
	f.calls = append(f.calls, streamCall{nodeID: nodeId, path: path, data: data})
	return []byte(`{"accepted":true}`), nil
}

func (f *fakeEchoNode) NodeInfo() warpnet.NodeInfo { return f.info }

func TestEchoAutoActionsOnForeignTweet(t *testing.T) {
	f := &fakeEchoNode{info: warpnet.NodeInfo{OwnerId: "echo-owner", ID: warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")}}
	bot := newEchoBot(f, nil)

	tweet := event.NewTweetEvent{Id: "tweet-1", RootId: "tweet-1", UserId: "foreign", Username: "alice", Text: "hello", CreatedAt: time.Now()}
	bt, err := json.Marshal(tweet)
	require.NoError(t, err)

	bot.handleTweet(bt, "remote-node-1")
	require.Len(t, f.calls, 3)
	require.Equal(t, "remote-node-1", f.calls[0].nodeID)
	require.Equal(t, event.PUBLIC_POST_LIKE, string(f.calls[0].path))
	require.Equal(t, event.PUBLIC_POST_RETWEET, string(f.calls[1].path))
	require.Equal(t, event.PUBLIC_POST_REPLY, string(f.calls[2].path))

	bot.handleTweet(bt, "remote-node-1")
	require.Len(t, f.calls, 3, "same tweet should be deduplicated in memory")
}

func TestEchoAutoReplyOnForeignReply(t *testing.T) {
	f := &fakeEchoNode{info: warpnet.NodeInfo{OwnerId: "echo-owner", ID: warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")}}
	bot := newEchoBot(f, nil)

	parentID := "tweet-1"
	rp := event.NewReplyEvent{Id: "reply-1", RootId: "tweet-1", ParentId: &parentID, UserId: "foreign", ParentUserId: "foreign", Text: "reply"}
	bt, err := json.Marshal(rp)
	require.NoError(t, err)

	bot.handleReply(bt, "remote-node-1")
	require.Len(t, f.calls, 1)
	require.Equal(t, event.PUBLIC_POST_REPLY, string(f.calls[0].path))
}

func TestEchoSkipsReplyOnEchoFormattedReply(t *testing.T) {
	f := &fakeEchoNode{info: warpnet.NodeInfo{OwnerId: "echo-owner", ID: warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")}}
	bot := newEchoBot(f, nil)

	parentID := "tweet-1"
	rp := event.NewReplyEvent{Id: "reply-1", RootId: "tweet-1", ParentId: &parentID, UserId: "foreign", ParentUserId: "foreign", Username: "Echo", Text: echoReplyPrefix + "hello"}
	bt, err := json.Marshal(rp)
	require.NoError(t, err)

	bot.handleReply(bt, "remote-node-1")
	require.Empty(t, f.calls)
}

func TestEchoAutoFollowBack(t *testing.T) {
	f := &fakeEchoNode{info: warpnet.NodeInfo{OwnerId: "echo-owner", ID: warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")}}
	bot := newEchoBot(f, nil)

	bt, err := json.Marshal(event.NewFollowEvent{FollowerId: "foreign", FollowingId: "echo-owner"})
	require.NoError(t, err)

	bot.handleFollow(bt, "remote-node-1")
	require.Len(t, f.calls, 1)
	require.Equal(t, event.PUBLIC_POST_FOLLOW, string(f.calls[0].path))
}

func TestEchoAutoReplyOnChatMessage(t *testing.T) {
	f := &fakeEchoNode{info: warpnet.NodeInfo{OwnerId: "echo-owner", ID: warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")}}
	bot := newEchoBot(f, nil)

	msg := event.NewMessageEvent{ChatId: "chat-1", SenderId: "foreign", ReceiverId: "echo-owner", Text: "ping", CreatedAt: time.Now()}
	bt, err := json.Marshal(msg)
	require.NoError(t, err)

	bot.handleMessage(bt, "remote-node-1")
	require.Len(t, f.calls, 1)
	require.Equal(t, event.PUBLIC_POST_MESSAGE, string(f.calls[0].path))
}

func TestEchoAutoReplyMessageIsTruncatedToLimit(t *testing.T) {
	f := &fakeEchoNode{info: warpnet.NodeInfo{OwnerId: "echo-owner", ID: warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")}}
	bot := newEchoBot(f, nil)

	msg := event.NewMessageEvent{
		ChatId:     "chat-1",
		SenderId:   "foreign",
		ReceiverId: "echo-owner",
		Text:       strings.Repeat("x", messageLimit),
		CreatedAt:  time.Now(),
	}
	bt, err := json.Marshal(msg)
	require.NoError(t, err)

	bot.handleMessage(bt, "remote-node-1")
	require.Len(t, f.calls, 1)
	require.Equal(t, event.PUBLIC_POST_MESSAGE, string(f.calls[0].path))

	evt, ok := f.calls[0].data.(event.NewMessageEvent)
	require.True(t, ok)
	require.LessOrEqual(t, len(evt.Text), messageLimit)
}

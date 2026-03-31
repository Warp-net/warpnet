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
	info      warpnet.NodeInfo
	calls     []streamCall
	selfCalls []streamCall
}

func (f *fakeEchoNode) GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
	if nodeId == f.info.ID.String() {
		return nil, errors.New("self stream request is forbidden")
	}
	f.calls = append(f.calls, streamCall{nodeID: nodeId, path: path, data: data})
	return []byte(`{"accepted":true}`), nil
}

func (f *fakeEchoNode) SelfStream(path stream.WarpRoute, data any) ([]byte, error) {
	f.selfCalls = append(f.selfCalls, streamCall{nodeID: f.info.ID.String(), path: path, data: data})
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
	require.Len(t, f.selfCalls, 3)
	require.Equal(t, event.PUBLIC_POST_LIKE, string(f.selfCalls[0].path))
	require.Equal(t, event.PUBLIC_POST_RETWEET, string(f.selfCalls[1].path))
	require.Equal(t, event.PUBLIC_POST_REPLY, string(f.selfCalls[2].path))
	require.Empty(t, f.calls)

	bot.handleTweet(bt)
	require.Len(t, f.selfCalls, 3, "same tweet should be deduplicated in memory")
}

func TestEchoAutoReplyOnForeignReply(t *testing.T) {
	f := &fakeEchoNode{info: warpnet.NodeInfo{OwnerId: "echo-owner", ID: warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")}}
	bot := newEchoBot(f)

	parentID := "tweet-1"
	rp := event.NewReplyEvent{Id: "reply-1", RootId: "tweet-1", ParentId: &parentID, UserId: "foreign", ParentUserId: "foreign", Text: "reply"}
	bt, err := json.Marshal(rp)
	require.NoError(t, err)

	bot.handleReply(bt)
	require.Len(t, f.selfCalls, 1)
	require.Equal(t, event.PUBLIC_POST_REPLY, string(f.selfCalls[0].path))
}

func TestEchoSkipsReplyOnEchoFormattedReply(t *testing.T) {
	f := &fakeEchoNode{info: warpnet.NodeInfo{OwnerId: "echo-owner", ID: warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")}}
	bot := newEchoBot(f)

	parentID := "tweet-1"
	rp := event.NewReplyEvent{Id: "reply-1", RootId: "tweet-1", ParentId: &parentID, UserId: "foreign", ParentUserId: "foreign", Username: "Echo", Text: echoReplyPrefix + "hello"}
	bt, err := json.Marshal(rp)
	require.NoError(t, err)

	bot.handleReply(bt)
	require.Empty(t, f.calls)
	require.Empty(t, f.selfCalls)
}

func TestEchoAutoFollowBack(t *testing.T) {
	f := &fakeEchoNode{info: warpnet.NodeInfo{OwnerId: "echo-owner", ID: warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")}}
	bot := newEchoBot(f)

	bt, err := json.Marshal(event.NewFollowEvent{FollowerId: "foreign", FollowingId: "echo-owner"})
	require.NoError(t, err)

	bot.handleFollow(bt)
	require.Len(t, f.selfCalls, 1)
	require.Equal(t, event.PUBLIC_POST_FOLLOW, string(f.selfCalls[0].path))
}

func TestEchoAutoReplyOnChatMessage(t *testing.T) {
	f := &fakeEchoNode{info: warpnet.NodeInfo{OwnerId: "echo-owner", ID: warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")}}
	bot := newEchoBot(f)

	msg := event.NewMessageEvent{ChatId: "chat-1", SenderId: "foreign", ReceiverId: "echo-owner", Text: "ping", CreatedAt: time.Now()}
	bt, err := json.Marshal(msg)
	require.NoError(t, err)

	bot.handleMessage(bt)
	require.Len(t, f.selfCalls, 1)
	require.Equal(t, event.PUBLIC_POST_MESSAGE, string(f.selfCalls[0].path))
}

func TestEchoAutoReplyMessageIsTruncatedToLimit(t *testing.T) {
	f := &fakeEchoNode{info: warpnet.NodeInfo{OwnerId: "echo-owner", ID: warpnet.FromStringToPeerID("12D3KooWQ7w6h96db3hG9s6S9xjCRz2xS9QPiQc5sKXc5teLoV6b")}}
	bot := newEchoBot(f)

	msg := event.NewMessageEvent{
		ChatId:     "chat-1",
		SenderId:   "foreign",
		ReceiverId: "echo-owner",
		Text:       strings.Repeat("x", messageLimit),
		CreatedAt:  time.Now(),
	}
	bt, err := json.Marshal(msg)
	require.NoError(t, err)

	bot.handleMessage(bt)
	require.Len(t, f.selfCalls, 1)
	require.Equal(t, event.PUBLIC_POST_MESSAGE, string(f.selfCalls[0].path))

	evt, ok := f.selfCalls[0].data.(event.NewMessageEvent)
	require.True(t, ok)
	require.LessOrEqual(t, len(evt.Text), messageLimit)
}

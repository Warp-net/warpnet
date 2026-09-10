//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/stretchr/testify/require"
)

// TestChatHandlerGuards drives the payload / empty-id / lookup-failure /
// not-a-participant guards every chat read handler shares.
func TestChatHandlerGuards(t *testing.T) {
	const owner = "owner-1"
	auth := stubAuth{owner: domain.Owner{UserId: owner}}
	lookupErr := errors.New("chat store down")

	failingLookup := stubChatRepo{getChatFn: func(string) (domain.Chat, error) {
		return domain.Chat{}, lookupErr
	}}
	foreignChat := stubChatRepo{getChatFn: func(id string) (domain.Chat, error) {
		return domain.Chat{Id: id, OwnerId: "someone", OtherUserId: "else"}, nil
	}}
	ownChat := stubChatRepo{getChatFn: func(id string) (domain.Chat, error) {
		return domain.Chat{Id: id, OwnerId: owner, OtherUserId: "other-1"}, nil
	}}

	t.Run("GetUserChat", func(t *testing.T) {
		_, err := StreamGetUserChatHandler(ownChat, auth)([]byte("{"), nil)
		require.Error(t, err)

		_, err = StreamGetUserChatHandler(ownChat, auth)(marshal(t, event.GetChatEvent{}), nil)
		require.Error(t, err)

		_, err = StreamGetUserChatHandler(failingLookup, auth)(marshal(t, event.GetChatEvent{ChatId: "chat-1"}), nil)
		require.ErrorIs(t, err, lookupErr)

		_, err = StreamGetUserChatHandler(foreignChat, auth)(marshal(t, event.GetChatEvent{ChatId: "chat-1"}), nil)
		require.Error(t, err)

		out, err := StreamGetUserChatHandler(ownChat, auth)(marshal(t, event.GetChatEvent{ChatId: "chat-1"}), nil)
		require.NoError(t, err)
		require.Equal(t, "chat-1", domain.Chat(out.(event.GetChatResponse)).Id)
	})

	t.Run("DeleteChat", func(t *testing.T) {
		_, err := StreamDeleteChatHandler(ownChat, auth)([]byte("{"), nil)
		require.Error(t, err)

		_, err = StreamDeleteChatHandler(ownChat, auth)(marshal(t, event.DeleteChatEvent{}), nil)
		require.Error(t, err)

		_, err = StreamDeleteChatHandler(failingLookup, auth)(marshal(t, event.DeleteChatEvent{ChatId: "chat-1"}), nil)
		require.ErrorIs(t, err, lookupErr)

		_, err = StreamDeleteChatHandler(foreignChat, auth)(marshal(t, event.DeleteChatEvent{ChatId: "chat-1"}), nil)
		require.Error(t, err)

		out, err := StreamDeleteChatHandler(ownChat, auth)(marshal(t, event.DeleteChatEvent{ChatId: "chat-1"}), nil)
		require.NoError(t, err)
		require.Equal(t, event.Accepted, out)
	})

	t.Run("DeleteMessage", func(t *testing.T) {
		_, err := StreamDeleteMessageHandler(ownChat, auth)([]byte("{"), nil)
		require.Error(t, err)

		_, err = StreamDeleteMessageHandler(ownChat, auth)(marshal(t, event.DeleteMessageEvent{Id: "msg-1"}), nil)
		require.Error(t, err)
		_, err = StreamDeleteMessageHandler(ownChat, auth)(marshal(t, event.DeleteMessageEvent{ChatId: "chat-1"}), nil)
		require.Error(t, err)

		ev := marshal(t, event.DeleteMessageEvent{ChatId: "chat-1", Id: "msg-1"})
		_, err = StreamDeleteMessageHandler(failingLookup, auth)(ev, nil)
		require.ErrorIs(t, err, lookupErr)

		_, err = StreamDeleteMessageHandler(foreignChat, auth)(ev, nil)
		require.Error(t, err)

		out, err := StreamDeleteMessageHandler(ownChat, auth)(ev, nil)
		require.NoError(t, err)
		require.Equal(t, event.Accepted, out)
	})

	t.Run("GetMessages", func(t *testing.T) {
		_, err := StreamGetMessagesHandler(ownChat, auth)([]byte("{"), nil)
		require.Error(t, err)

		_, err = StreamGetMessagesHandler(ownChat, auth)(marshal(t, event.GetAllMessagesEvent{OwnerId: owner}), nil)
		require.Error(t, err)

		ev := marshal(t, event.GetAllMessagesEvent{ChatId: "chat-1", OwnerId: owner})
		_, err = StreamGetMessagesHandler(failingLookup, auth)(ev, nil)
		require.ErrorIs(t, err, lookupErr)

		_, err = StreamGetMessagesHandler(foreignChat, auth)(ev, nil)
		require.Error(t, err)

		listErr := errors.New("list down")
		failingList := stubChatRepo{
			getChatFn:      ownChat.getChatFn,
			listMessagesFn: func(string, *uint64, *string) ([]domain.ChatMessage, string, error) { return nil, "", listErr },
		}
		_, err = StreamGetMessagesHandler(failingList, auth)(ev, nil)
		require.ErrorIs(t, err, listErr)

		out, err := StreamGetMessagesHandler(ownChat, auth)(ev, nil)
		require.NoError(t, err)
		require.Empty(t, out.(event.ChatMessagesResponse).Messages)
	})

	t.Run("GetMessage", func(t *testing.T) {
		_, err := StreamGetMessageHandler(ownChat, auth)([]byte("{"), nil)
		require.Error(t, err)

		_, err = StreamGetMessageHandler(ownChat, auth)(marshal(t, event.GetMessageEvent{Id: "msg-1"}), nil)
		require.Error(t, err)
		_, err = StreamGetMessageHandler(ownChat, auth)(marshal(t, event.GetMessageEvent{ChatId: "chat-1"}), nil)
		require.Error(t, err)

		ev := marshal(t, event.GetMessageEvent{ChatId: "chat-1", Id: "msg-1"})
		_, err = StreamGetMessageHandler(failingLookup, auth)(ev, nil)
		require.ErrorIs(t, err, lookupErr)

		_, err = StreamGetMessageHandler(foreignChat, auth)(ev, nil)
		require.Error(t, err)

		getErr := errors.New("message store down")
		failingMessage := stubChatRepo{
			getChatFn:    ownChat.getChatFn,
			getMessageFn: func(string, string) (domain.ChatMessage, error) { return domain.ChatMessage{}, getErr },
		}
		_, err = StreamGetMessageHandler(failingMessage, auth)(ev, nil)
		require.ErrorIs(t, err, getErr)
	})

	t.Run("GetUserChats", func(t *testing.T) {
		_, err := StreamGetUserChatsHandler(ownChat, auth)([]byte("{"), nil)
		require.Error(t, err)

		_, err = StreamGetUserChatsHandler(ownChat, auth)(marshal(t, event.GetAllChatsEvent{}), nil)
		require.Error(t, err)

		listErr := errors.New("list down")
		failingList := stubChatRepo{getUserChatsFn: func(string, *uint64, *string) ([]domain.Chat, string, error) {
			return nil, "", listErr
		}}
		_, err = StreamGetUserChatsHandler(failingList, auth)(marshal(t, event.GetAllChatsEvent{UserId: owner}), nil)
		require.ErrorIs(t, err, listErr)
	})
}

//nolint:all
package handler

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

type stubChatRepo struct {
	createChatFn    func(chatId *string, ownerId, otherUserId string) (domain.Chat, error)
	deleteChatFn    func(chatId string) error
	getUserChatsFn  func(userId string, limit *uint64, cursor *string) ([]domain.Chat, string, error)
	createMessageFn func(msg domain.ChatMessage) (domain.ChatMessage, error)
	listMessagesFn  func(chatId string, limit *uint64, cursor *string) ([]domain.ChatMessage, string, error)
	getMessageFn    func(chatId, id string) (domain.ChatMessage, error)
	deleteMessageFn func(chatId, id string) error
	getChatFn       func(chatId string) (domain.Chat, error)
}

func (s stubChatRepo) CreateChat(chatId *string, ownerId, otherUserId string) (domain.Chat, error) {
	if s.createChatFn != nil {
		return s.createChatFn(chatId, ownerId, otherUserId)
	}
	return domain.Chat{Id: valueOr(chatId, "chat-1"), OwnerId: ownerId, OtherUserId: otherUserId, CreatedAt: time.Now()}, nil
}

func (s stubChatRepo) DeleteChat(chatId string) error {
	if s.deleteChatFn != nil {
		return s.deleteChatFn(chatId)
	}
	return nil
}

func (s stubChatRepo) GetUserChats(userId string, limit *uint64, cursor *string) ([]domain.Chat, string, error) {
	if s.getUserChatsFn != nil {
		return s.getUserChatsFn(userId, limit, cursor)
	}
	return nil, "", nil
}

func (s stubChatRepo) CreateMessage(msg domain.ChatMessage) (domain.ChatMessage, error) {
	if s.createMessageFn != nil {
		return s.createMessageFn(msg)
	}
	msg.Id = "msg-1"
	return msg, nil
}

func (s stubChatRepo) ListMessages(chatId string, limit *uint64, cursor *string) ([]domain.ChatMessage, string, error) {
	if s.listMessagesFn != nil {
		return s.listMessagesFn(chatId, limit, cursor)
	}
	return nil, "", nil
}

func (s stubChatRepo) GetMessage(chatId, id string) (domain.ChatMessage, error) {
	if s.getMessageFn != nil {
		return s.getMessageFn(chatId, id)
	}
	return domain.ChatMessage{Id: id, ChatId: chatId}, nil
}

func (s stubChatRepo) DeleteMessage(chatId, id string) error {
	if s.deleteMessageFn != nil {
		return s.deleteMessageFn(chatId, id)
	}
	return nil
}

func (s stubChatRepo) GetChat(chatId string) (domain.Chat, error) {
	if s.getChatFn != nil {
		return s.getChatFn(chatId)
	}
	return domain.Chat{Id: chatId, OwnerId: "owner-1", OtherUserId: "other-1"}, nil
}

type stubUserRepo struct {
	getByNodeIdFn func(nodeID string) (domain.User, error)
	getFn         func(userId string) (domain.User, error)
}

func (s stubUserRepo) GetByNodeID(nodeID string) (domain.User, error) {
	if s.getByNodeIdFn != nil {
		return s.getByNodeIdFn(nodeID)
	}
	return domain.User{}, nil
}

func (s stubUserRepo) Get(userId string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return domain.User{Id: userId, NodeId: "node-2"}, nil
}

type stubStreamer struct {
	genericStreamFn func(nodeId string, path stream.WarpRoute, data any) ([]byte, error)
	nodeInfo        warpnet.NodeInfo
}

func (s stubStreamer) GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
	if s.genericStreamFn != nil {
		return s.genericStreamFn(nodeId, path, data)
	}
	return nil, nil
}

func (s stubStreamer) NodeInfo() warpnet.NodeInfo { return s.nodeInfo }

type stubAuth struct{ owner domain.Owner }

func (s stubAuth) GetOwner() domain.Owner { return s.owner }

func marshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func valueOr(v *string, fallback string) string {
	if v == nil {
		return fallback
	}
	return *v
}

func TestStreamCreateChatHandler(t *testing.T) {
	owner := "owner-1"
	other := "other-1"
	chatID := "chat-1"
	chat := domain.Chat{Id: chatID, OwnerId: owner, OtherUserId: other, CreatedAt: time.Now()}

	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamCreateChatHandler(stubChatRepo{}, stubUserRepo{}, stubStreamer{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty ids", func(t *testing.T) {
		_, err := StreamCreateChatHandler(stubChatRepo{}, stubUserRepo{}, stubStreamer{})(marshal(t, event.NewChatEvent{}), nil)
		if err == nil || err.Error() != "owner ID or other user ID is empty" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("create chat error", func(t *testing.T) {
		repoErr := errors.New("db failed")
		_, err := StreamCreateChatHandler(stubChatRepo{createChatFn: func(chatId *string, ownerId, otherUserId string) (domain.Chat, error) {
			return domain.Chat{}, repoErr
		}}, stubUserRepo{}, stubStreamer{})(marshal(t, event.NewChatEvent{OwnerId: owner, OtherUserId: other, ChatId: &chatID}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got: %v", err)
		}
	})

	t.Run("self chat", func(t *testing.T) {
		resp, err := StreamCreateChatHandler(stubChatRepo{}, stubUserRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})(marshal(t, event.NewChatEvent{OwnerId: owner, OtherUserId: owner, ChatId: &chatID}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.ChatCreatedResponse).OwnerId != owner {
			t.Fatalf("unexpected response: %#v", resp)
		}
	})

	t.Run("chat created by other user", func(t *testing.T) {
		resp, err := StreamCreateChatHandler(stubChatRepo{createChatFn: func(chatId *string, ownerId, otherUserId string) (domain.Chat, error) {
			return chat, nil
		}}, stubUserRepo{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: "another-owner"}})(marshal(t, event.NewChatEvent{OwnerId: owner, OtherUserId: other, ChatId: &chatID}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.ChatCreatedResponse).Id != chatID {
			t.Fatalf("unexpected response: %#v", resp)
		}
	})

	t.Run("local owner user not found", func(t *testing.T) {
		resp, err := StreamCreateChatHandler(stubChatRepo{}, stubUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})(marshal(t, event.NewChatEvent{OwnerId: owner, OtherUserId: other, ChatId: &chatID}), nil)
		if err != nil || resp == nil {
			t.Fatalf("unexpected: resp=%v err=%v", resp, err)
		}
	})

	t.Run("generic stream error branches", func(t *testing.T) {
		tests := []error{warpnet.ErrNodeIsOffline, node.ErrSelfRequest, errors.New("broken")}
		for _, streamErr := range tests {
			resp, err := StreamCreateChatHandler(stubChatRepo{}, stubUserRepo{}, stubStreamer{
				nodeInfo: warpnet.NodeInfo{OwnerId: owner},
				genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
					return nil, streamErr
				},
			})(marshal(t, event.NewChatEvent{OwnerId: owner, OtherUserId: other, ChatId: &chatID}), nil)
			if err != nil || resp == nil {
				t.Fatalf("streamErr=%v resp=%v err=%v", streamErr, resp, err)
			}
		}
	})

	t.Run("remote response contains error payload", func(t *testing.T) {
		respErr, _ := json.Marshal(event.ResponseError{Code: 500, Message: "oops"})
		resp, err := StreamCreateChatHandler(stubChatRepo{}, stubUserRepo{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return respErr, nil
			},
		})(marshal(t, event.NewChatEvent{OwnerId: owner, OtherUserId: other, ChatId: &chatID}), nil)
		if err != nil || resp == nil {
			t.Fatalf("unexpected: resp=%v err=%v", resp, err)
		}
	})
}

func TestChatAuthHandlers(t *testing.T) {
	owner := "owner-1"
	chatID := "chat-1"

	t.Run("get single chat", func(t *testing.T) {
		h := StreamGetUserChatHandler(stubChatRepo{getChatFn: func(chatId string) (domain.Chat, error) {
			return domain.Chat{Id: chatId, OwnerId: owner, OtherUserId: "other"}, nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h(marshal(t, event.GetChatEvent{ChatId: chatID}), nil)
		if err != nil || resp.(event.GetChatResponse).Id != chatID {
			t.Fatalf("unexpected: resp=%v err=%v", resp, err)
		}
	})

	t.Run("get single chat unauthorized", func(t *testing.T) {
		h := StreamGetUserChatHandler(stubChatRepo{}, stubAuth{owner: domain.Owner{UserId: "intruder"}})
		_, err := h(marshal(t, event.GetChatEvent{ChatId: chatID}), nil)
		if err == nil || err.Error() != "not authorized for this chat" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("delete chat unauthorized", func(t *testing.T) {
		h := StreamDeleteChatHandler(stubChatRepo{}, stubAuth{owner: domain.Owner{UserId: "intruder"}})
		_, err := h(marshal(t, event.DeleteChatEvent{ChatId: chatID}), nil)
		if err == nil || err.Error() != "not authorized for this chat" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("delete chat success", func(t *testing.T) {
		deleted := false
		h := StreamDeleteChatHandler(stubChatRepo{deleteChatFn: func(chatId string) error {
			deleted = true
			return nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h(marshal(t, event.DeleteChatEvent{ChatId: chatID}), nil)
		if err != nil || resp != event.Accepted || !deleted {
			t.Fatalf("unexpected: resp=%v err=%v deleted=%v", resp, err, deleted)
		}
	})
}

func TestStreamGetUserChatsHandler(t *testing.T) {
	owner := "owner-1"
	h := StreamGetUserChatsHandler(stubChatRepo{getUserChatsFn: func(userId string, limit *uint64, cursor *string) ([]domain.Chat, string, error) {
		return []domain.Chat{{Id: "c1", OwnerId: owner, OtherUserId: "u2"}}, "next", nil
	}}, stubAuth{owner: domain.Owner{UserId: owner}})

	_, err := h([]byte("{"), nil)
	if err == nil || !strings.Contains(err.Error(), "get chats: unmarshal") {
		t.Fatalf("expected wrapped unmarshal error: %v", err)
	}

	_, err = h(marshal(t, event.GetAllChatsEvent{}), nil)
	if err == nil || err.Error() != "empty user ID" {
		t.Fatalf("unexpected err: %v", err)
	}

	_, err = h(marshal(t, event.GetAllChatsEvent{UserId: "other"}), nil)
	if err == nil || err.Error() != "not owner's chats" {
		t.Fatalf("unexpected err: %v", err)
	}

	errHandler := StreamGetUserChatsHandler(stubChatRepo{getUserChatsFn: func(userId string, limit *uint64, cursor *string) ([]domain.Chat, string, error) {
		return nil, "", errors.New("db")
	}}, stubAuth{owner: domain.Owner{UserId: owner}})

	_, err = errHandler(marshal(t, event.GetAllChatsEvent{UserId: owner}), nil)
	if err == nil || !strings.Contains(err.Error(), "get chats: fetch from db") {
		t.Fatalf("unexpected err: %v", err)
	}

	emptyHandler := StreamGetUserChatsHandler(stubChatRepo{getUserChatsFn: func(userId string, limit *uint64, cursor *string) ([]domain.Chat, string, error) {
		return nil, "next", nil
	}}, stubAuth{owner: domain.Owner{UserId: owner}})

	resp, err := emptyHandler(marshal(t, event.GetAllChatsEvent{UserId: owner}), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(resp.(event.ChatsResponse).Chats) != 0 {
		t.Fatalf("expected empty chats: %#v", resp)
	}

	resp, err = h(marshal(t, event.GetAllChatsEvent{UserId: owner}), nil)
	if err != nil || len(resp.(event.ChatsResponse).Chats) != 1 {
		t.Fatalf("unexpected resp=%#v err=%v", resp, err)
	}
}

func TestStreamNewMessageHandler(t *testing.T) {
	owner := "owner-1"
	receiver := "receiver-1"
	chatID := "owner-1:receiver-1"

	makeHandler := func(repo stubChatRepo, users stubUserRepo, streamer stubStreamer) warpnet.WarpHandlerFunc {
		streamer.nodeInfo.OwnerId = owner
		return StreamNewMessageHandler(repo, users, streamer)
	}

	bad := []event.NewMessageEvent{{}, {ChatId: "abc", Text: "x", SenderId: owner, ReceiverId: receiver}, {ChatId: chatID, SenderId: owner, ReceiverId: receiver}}
	for _, ev := range bad {
		_, err := makeHandler(stubChatRepo{}, stubUserRepo{}, stubStreamer{})(marshal(t, ev), nil)
		if err == nil {
			t.Fatalf("expected invalid message params for %#v", ev)
		}
	}

	_, err := makeHandler(stubChatRepo{}, stubUserRepo{}, stubStreamer{})(marshal(t, event.NewMessageEvent{ChatId: chatID, Text: "ok", SenderId: "", ReceiverId: receiver}), nil)
	if err == nil || err.Error() != "sender and receiver parameters are invalid" {
		t.Fatalf("unexpected err: %v", err)
	}

	_, err = makeHandler(stubChatRepo{}, stubUserRepo{}, stubStreamer{})(marshal(t, event.NewMessageEvent{ChatId: chatID, Text: strings.Repeat("a", messageLimit+1), SenderId: owner, ReceiverId: receiver}), nil)
	if err == nil || err.Error() != "message is too long" {
		t.Fatalf("unexpected err: %v", err)
	}

	_, err = makeHandler(stubChatRepo{}, stubUserRepo{}, stubStreamer{})(marshal(t, event.NewMessageEvent{ChatId: chatID, Text: "ok", SenderId: "u1", ReceiverId: "u2"}), nil)
	if err == nil || err.Error() != "not authorized to send message to this chat" {
		t.Fatalf("unexpected err: %v", err)
	}

	repoErr := errors.New("repo")
	_, err = makeHandler(stubChatRepo{getChatFn: func(chatId string) (domain.Chat, error) { return domain.Chat{}, repoErr }}, stubUserRepo{}, stubStreamer{})(marshal(t, event.NewMessageEvent{ChatId: chatID, Text: "ok", SenderId: owner, ReceiverId: receiver}), nil)
	if !errors.Is(err, repoErr) {
		t.Fatalf("expected repo error: %v", err)
	}

	_, err = makeHandler(stubChatRepo{getChatFn: func(chatId string) (domain.Chat, error) {
		return domain.Chat{}, database.ErrChatNotFound
	}, createChatFn: func(chatId *string, ownerId, otherUserId string) (domain.Chat, error) {
		return domain.Chat{}, repoErr
	}}, stubUserRepo{}, stubStreamer{})(marshal(t, event.NewMessageEvent{ChatId: chatID, Text: "ok", SenderId: receiver, ReceiverId: owner}), nil)
	if !errors.Is(err, repoErr) {
		t.Fatalf("expected create chat error: %v", err)
	}

	_, err = makeHandler(stubChatRepo{getChatFn: func(chatId string) (domain.Chat, error) {
		return domain.Chat{Id: chatID, OwnerId: "u3", OtherUserId: "u4"}, nil
	}}, stubUserRepo{}, stubStreamer{})(marshal(t, event.NewMessageEvent{ChatId: chatID, Text: "ok", SenderId: owner, ReceiverId: receiver}), nil)
	if err == nil || err.Error() != "not authorized for this chat" {
		t.Fatalf("unexpected err: %v", err)
	}

	_, err = makeHandler(stubChatRepo{getChatFn: func(chatId string) (domain.Chat, error) {
		return domain.Chat{Id: chatID, OwnerId: owner, OtherUserId: receiver}, nil
	}, createMessageFn: func(msg domain.ChatMessage) (domain.ChatMessage, error) { return domain.ChatMessage{}, repoErr }}, stubUserRepo{}, stubStreamer{})(marshal(t, event.NewMessageEvent{ChatId: chatID, Text: "ok", SenderId: owner, ReceiverId: receiver}), nil)
	if !errors.Is(err, repoErr) {
		t.Fatalf("expected create msg error: %v", err)
	}

	resp, err := makeHandler(stubChatRepo{getChatFn: func(chatId string) (domain.Chat, error) {
		return domain.Chat{Id: chatID, OwnerId: owner, OtherUserId: owner}, nil
	}}, stubUserRepo{}, stubStreamer{})(marshal(t, event.NewMessageEvent{ChatId: chatID, Text: "self", SenderId: owner, ReceiverId: owner}), nil)
	if err != nil || resp.(event.NewMessageResponse).Status != "" {
		t.Fatalf("unexpected self chat result: %#v err=%v", resp, err)
	}

	resp, err = makeHandler(stubChatRepo{getChatFn: func(chatId string) (domain.Chat, error) {
		return domain.Chat{}, database.ErrChatNotFound
	}, createChatFn: func(chatId *string, ownerId, otherUserId string) (domain.Chat, error) {
		return domain.Chat{Id: chatID, OwnerId: receiver, OtherUserId: owner}, nil
	}}, stubUserRepo{}, stubStreamer{})(marshal(t, event.NewMessageEvent{ChatId: chatID, Text: "incoming", SenderId: receiver, ReceiverId: owner}), nil)
	if err != nil || resp.(event.NewMessageResponse).Text != "incoming" {
		t.Fatalf("unexpected incoming result: %#v err=%v", resp, err)
	}

	resp, err = makeHandler(stubChatRepo{getChatFn: func(chatId string) (domain.Chat, error) {
		return domain.Chat{Id: chatID, OwnerId: owner, OtherUserId: receiver}, nil
	}}, stubUserRepo{getFn: func(userId string) (domain.User, error) {
		return domain.User{}, database.ErrUserNotFound
	}}, stubStreamer{})(marshal(t, event.NewMessageEvent{ChatId: chatID, Text: "ok", SenderId: owner, ReceiverId: receiver}), nil)
	if err != nil || resp.(event.NewMessageResponse).Status != "" {
		t.Fatalf("unexpected user not found branch: %#v err=%v", resp, err)
	}

	_, err = makeHandler(stubChatRepo{getChatFn: func(chatId string) (domain.Chat, error) {
		return domain.Chat{Id: chatID, OwnerId: owner, OtherUserId: receiver}, nil
	}}, stubUserRepo{getFn: func(userId string) (domain.User, error) { return domain.User{}, repoErr }}, stubStreamer{})(marshal(t, event.NewMessageEvent{ChatId: chatID, Text: "ok", SenderId: owner, ReceiverId: receiver}), nil)
	if !errors.Is(err, repoErr) {
		t.Fatalf("expected user repo error: %v", err)
	}

	resp, err = makeHandler(stubChatRepo{getChatFn: func(chatId string) (domain.Chat, error) {
		return domain.Chat{Id: chatID, OwnerId: owner, OtherUserId: receiver}, nil
	}}, stubUserRepo{}, stubStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
		return nil, warpnet.ErrNodeIsOffline
	}})(marshal(t, event.NewMessageEvent{ChatId: chatID, Text: "ok", SenderId: owner, ReceiverId: receiver}), nil)
	if err != nil || resp.(event.NewMessageResponse).Status != "undelivered" {
		t.Fatalf("expected undelivered offline: %#v err=%v", resp, err)
	}

	_, err = makeHandler(stubChatRepo{getChatFn: func(chatId string) (domain.Chat, error) {
		return domain.Chat{Id: chatID, OwnerId: owner, OtherUserId: receiver}, nil
	}}, stubUserRepo{}, stubStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
		return nil, repoErr
	}})(marshal(t, event.NewMessageEvent{ChatId: chatID, Text: "ok", SenderId: owner, ReceiverId: receiver}), nil)
	if !errors.Is(err, repoErr) {
		t.Fatalf("expected stream error: %v", err)
	}

	remoteErr, _ := json.Marshal(event.ResponseError{Message: "remote failed", Code: 400})
	resp, err = makeHandler(stubChatRepo{getChatFn: func(chatId string) (domain.Chat, error) {
		return domain.Chat{Id: chatID, OwnerId: owner, OtherUserId: receiver}, nil
	}}, stubUserRepo{}, stubStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
		return remoteErr, nil
	}})(marshal(t, event.NewMessageEvent{ChatId: chatID, Text: "ok", SenderId: owner, ReceiverId: receiver}), nil)
	if err != nil || resp.(event.NewMessageResponse).Status != "undelivered" {
		t.Fatalf("expected undelivered due to remote payload err: %#v err=%v", resp, err)
	}

	resp, err = makeHandler(stubChatRepo{getChatFn: func(chatId string) (domain.Chat, error) {
		return domain.Chat{Id: chatID, OwnerId: owner, OtherUserId: receiver}, nil
	}}, stubUserRepo{}, stubStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
		return []byte("{}"), nil
	}})(marshal(t, event.NewMessageEvent{ChatId: chatID, Text: "ok", SenderId: owner, ReceiverId: receiver}), nil)
	if err != nil || resp.(event.NewMessageResponse).Status != "" {
		t.Fatalf("expected delivered result: %#v err=%v", resp, err)
	}
}

func TestMessageReadDeleteHandlers(t *testing.T) {
	owner := "owner-1"
	chatID := "chat-1"
	msgID := "msg-1"
	auth := stubAuth{owner: domain.Owner{UserId: owner}}
	repo := stubChatRepo{getChatFn: func(chatId string) (domain.Chat, error) {
		return domain.Chat{Id: chatId, OwnerId: owner, OtherUserId: "other"}, nil
	}}

	t.Run("delete message success", func(t *testing.T) {
		h := StreamDeleteMessageHandler(repo, auth)
		resp, err := h(marshal(t, event.DeleteMessageEvent{ChatId: chatID, Id: msgID}), nil)
		if err != nil || resp != event.Accepted {
			t.Fatalf("unexpected: resp=%v err=%v", resp, err)
		}
	})

	t.Run("get messages empty and non-empty", func(t *testing.T) {
		hEmpty := StreamGetMessagesHandler(stubChatRepo{getChatFn: repo.getChatFn, listMessagesFn: func(chatId string, limit *uint64, cursor *string) ([]domain.ChatMessage, string, error) {
			return nil, "next", nil
		}}, auth)
		resp, err := hEmpty(marshal(t, event.GetAllMessagesEvent{ChatId: chatID}), nil)
		if err != nil || len(resp.(event.ChatMessagesResponse).Messages) != 0 {
			t.Fatalf("unexpected empty response: %#v err=%v", resp, err)
		}

		hFilled := StreamGetMessagesHandler(stubChatRepo{getChatFn: repo.getChatFn, listMessagesFn: func(chatId string, limit *uint64, cursor *string) ([]domain.ChatMessage, string, error) {
			return []domain.ChatMessage{{Id: msgID, ChatId: chatID}}, "", nil
		}}, auth)
		resp, err = hFilled(marshal(t, event.GetAllMessagesEvent{ChatId: chatID}), nil)
		if err != nil || len(resp.(event.ChatMessagesResponse).Messages) != 1 {
			t.Fatalf("unexpected filled response: %#v err=%v", resp, err)
		}
	})

	t.Run("get one message success", func(t *testing.T) {
		h := StreamGetMessageHandler(stubChatRepo{getChatFn: repo.getChatFn, getMessageFn: func(chatId, id string) (domain.ChatMessage, error) {
			return domain.ChatMessage{Id: id, ChatId: chatId}, nil
		}}, auth)
		resp, err := h(marshal(t, event.GetMessageEvent{ChatId: chatID, Id: msgID}), nil)
		if err != nil || resp.(event.ChatMessageResponse).Id != msgID {
			t.Fatalf("unexpected response: %#v err=%v", resp, err)
		}
	})

	t.Run("authorization checks", func(t *testing.T) {
		intruderAuth := stubAuth{owner: domain.Owner{UserId: "intruder"}}
		if _, err := StreamDeleteMessageHandler(repo, intruderAuth)(marshal(t, event.DeleteMessageEvent{ChatId: chatID, Id: msgID}), nil); err == nil {
			t.Fatal("expected auth error for delete")
		}
		if _, err := StreamGetMessagesHandler(repo, intruderAuth)(marshal(t, event.GetAllMessagesEvent{ChatId: chatID}), nil); err == nil {
			t.Fatal("expected auth error for list")
		}
		if _, err := StreamGetMessageHandler(repo, intruderAuth)(marshal(t, event.GetMessageEvent{ChatId: chatID, Id: msgID}), nil); err == nil {
			t.Fatal("expected auth error for get")
		}
	})
}

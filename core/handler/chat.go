/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
	"strings"
	"time"
)

type ChatAuthStorer interface {
	GetOwner() domain.Owner
}

type ChatStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

type ChatUserFetcher interface {
	GetByNodeID(nodeID string) (user domain.User, err error)
	Get(userId string) (user domain.User, err error)
}

type ChatStorer interface {
	CreateChat(chatId *string, ownerId, otherUserId string) (domain.Chat, error)
	DeleteChat(chatId string) error
	GetUserChats(userId string, limit *uint64, cursor *string) ([]domain.Chat, string, error)
	CreateMessage(msg domain.ChatMessage) (domain.ChatMessage, error)
	ListMessages(chatId string, limit *uint64, cursor *string) ([]domain.ChatMessage, string, error)
	GetMessage(chatId, id string) (domain.ChatMessage, error)
	DeleteMessage(chatId, id string) error
	GetChat(chatId string) (chat domain.Chat, err error)
}

// Handler for creating a new chat
func StreamCreateChatHandler(
	repo ChatStorer,
	userRepo ChatUserFetcher,
	streamer ChatStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.NewChatEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.OwnerId == "" || ev.OtherUserId == "" {
			return nil, warpnet.WarpError("owner ID or other user ID is empty")
		}

		otherUser, err := userRepo.Get(ev.OtherUserId)
		if err != nil {
			return nil, err
		}

		if otherUser.IsOffline {
			return nil, warpnet.ErrUserIsOffline // TODO
		}

		ownerChat, err := repo.CreateChat(ev.ChatId, ev.OwnerId, ev.OtherUserId)
		if err != nil {
			return nil, err
		}

		if ev.OwnerId == ev.OtherUserId { // self chat
			return event.ChatCreatedResponse(ownerChat), nil
		}
		if ev.OwnerId != streamer.NodeInfo().OwnerId { // other user created chat
			log.Infoln("new chat!")
			return event.ChatCreatedResponse(ownerChat), nil
		}

		otherChatData, err := streamer.GenericStream(
			otherUser.NodeId,
			event.PUBLIC_POST_CHAT,
			domain.Chat{
				CreatedAt:   ownerChat.CreatedAt,
				Id:          ownerChat.Id,
				OtherUserId: ownerChat.OtherUserId,
				OwnerId:     ownerChat.OwnerId,
			},
		)
		if err != nil && !errors.Is(err, node.ErrSelfRequest) {
			_ = repo.DeleteChat(ownerChat.Id)
			return nil, err
		}

		var possibleError event.ErrorResponse
		if _ = json.Unmarshal(otherChatData, &possibleError); possibleError.Message != "" {
			_ = repo.DeleteChat(ownerChat.Id)
			return nil, fmt.Errorf("unmarshal other chat error response: %s", possibleError.Message)
		}

		return event.ChatCreatedResponse(ownerChat), err
	}
}

func StreamGetUserChatHandler(repo ChatStorer, authRepo ChatAuthStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetChatEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		if ev.ChatId == "" {
			return nil, warpnet.WarpError("empty chat ID")
		}

		chat, err := repo.GetChat(ev.ChatId)
		if err != nil {
			return nil, err
		}
		ownerId := authRepo.GetOwner().UserId
		if chat.OwnerId != ownerId && chat.OtherUserId != ownerId {
			return nil, warpnet.WarpError("not authorized for this chat")
		}
		return event.GetChatResponse(chat), nil
	}
}

func StreamDeleteChatHandler(repo ChatStorer, authRepo ChatAuthStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.DeleteChatEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.ChatId == "" {
			return nil, warpnet.WarpError("chat ID is empty")
		}

		chat, err := repo.GetChat(ev.ChatId)
		if err != nil {
			return nil, err
		}
		ownerId := authRepo.GetOwner().UserId
		if chat.OwnerId != ownerId && chat.OtherUserId != ownerId {
			return nil, warpnet.WarpError("not authorized for this chat")
		}

		return event.Accepted, repo.DeleteChat(ev.ChatId)
	}
}

type OwnerChatsStorer interface {
	GetOwner() domain.Owner
}

func StreamGetUserChatsHandler(repo ChatStorer, authRepo OwnerChatsStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetAllChatsEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, fmt.Errorf("get chats: unmarshal: %w", err)
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty user ID")
		}

		owner := authRepo.GetOwner()
		if owner.UserId != ev.UserId {
			return nil, warpnet.WarpError("not owner's chats")
		}

		chats, cursor, err := repo.GetUserChats(ev.UserId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, fmt.Errorf("get chats: fetch from db: %w", err)
		}
		if len(chats) == 0 {
			return event.ChatsResponse{
				Chats:  []domain.Chat{},
				Cursor: cursor,
				UserId: ev.UserId,
			}, nil
		}
		return event.ChatsResponse{
			UserId: ev.UserId,
			Chats:  chats,
			Cursor: cursor,
		}, nil
	}
}

// Handler for sending a new message
func StreamSendMessageHandler(repo ChatStorer, userRepo ChatUserFetcher, streamer ChatStreamer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.NewMessageEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.ChatId == "" || !strings.Contains(ev.ChatId, ":") || ev.Text == "" {
			return nil, warpnet.WarpError("message parameters are invalid")
		}
		if ev.SenderId == "" || ev.ReceiverId == "" {
			return nil, warpnet.WarpError("sender and receiver parameters are invalid")
		}
		if len(ev.Text) > 5000 {
			return nil, warpnet.WarpError("message is too long")
		}

		ownerId := streamer.NodeInfo().OwnerId

		if ev.SenderId != ownerId && ev.ReceiverId != ownerId {
			return nil, warpnet.WarpError("not authorized to send message to this chat")
		}

		isSelfChat := ev.SenderId == ev.ReceiverId
		isOwnerReceiver := ownerId == ev.ReceiverId

		chat, err := repo.GetChat(ev.ChatId)
		if err != nil {
			return nil, err
		}

		if chat.OwnerId != ownerId && chat.OtherUserId != ownerId {
			return nil, warpnet.WarpError("not authorized for this chat")
		}

		now := time.Now()
		msg := domain.ChatMessage{
			ChatId:     ev.ChatId,
			SenderId:   ev.SenderId,
			ReceiverId: ev.ReceiverId,
			Text:       ev.Text,
			CreatedAt:  now,
		}

		msg, err = repo.CreateMessage(msg)
		if err != nil {
			return nil, err
		}

		if isSelfChat {
			return event.NewMessageResponse(msg), nil
		}
		if isOwnerReceiver { // the other user sent a message
			log.Infoln("received new message!")
			return event.NewMessageResponse(msg), nil
		}

		otherUser, err := userRepo.Get(ev.ReceiverId)
		if err != nil {
			return nil, err
		}

		otherMsgData, err := streamer.GenericStream(
			otherUser.NodeId,
			event.PUBLIC_POST_MESSAGE,
			event.NewMessageEvent(domain.ChatMessage{
				ChatId:     ev.ChatId,
				SenderId:   ownerId,
				ReceiverId: ev.ReceiverId,
				Text:       ev.Text,
				CreatedAt:  now,
			}),
		)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			log.Warnf("chat message send to offline: %s", otherUser.NodeId)
			msg.Status = "undelivered"
			return event.NewMessageResponse(msg), err
		}
		if err != nil {
			return nil, err
		}

		var possibleError event.ErrorResponse
		if _ = json.Unmarshal(otherMsgData, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other message error response: %s", possibleError.Message)
			msg.Status = "undelivered"
		}

		return event.NewMessageResponse(msg), err
	}
}

// Handler for deleting a message
func StreamDeleteMessageHandler(repo ChatStorer, authRepo OwnerChatsStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.DeleteMessageEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.ChatId == "" || ev.Id == "" {
			return nil, warpnet.WarpError("chat ID, user ID, or message ID cannot be blank")
		}
		chat, err := repo.GetChat(ev.ChatId)
		if err != nil {
			return nil, err
		}
		ownerId := authRepo.GetOwner().UserId
		if chat.OwnerId != ownerId && chat.OtherUserId != ownerId {
			return nil, warpnet.WarpError("not authorized for this chat")
		}

		return event.Accepted, repo.DeleteMessage(ev.ChatId, ev.Id)
	}
}

// Handler for getting messages in a chat
func StreamGetMessagesHandler(repo ChatStorer, authRepo OwnerChatsStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetAllMessagesEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.ChatId == "" {
			return nil, warpnet.WarpError("chat ID cannot be blank")
		}

		chat, err := repo.GetChat(ev.ChatId)
		if err != nil {
			return nil, err
		}

		ownerId := authRepo.GetOwner().UserId
		if chat.OwnerId != ownerId && chat.OtherUserId != ownerId {
			return nil, warpnet.WarpError("not authorized for this chat")
		}

		messages, cursor, err := repo.ListMessages(ev.ChatId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}

		if len(messages) == 0 {
			return event.ChatMessagesResponse{
				ChatId:   ev.ChatId,
				Cursor:   cursor,
				Messages: []domain.ChatMessage{},
			}, nil
		}

		return event.ChatMessagesResponse{
			ChatId:   ev.ChatId,
			Messages: messages,
			Cursor:   cursor,
		}, nil
	}
}

// StreamGetMessageHandler for retrieving a specific message
func StreamGetMessageHandler(repo ChatStorer, authRepo OwnerChatsStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetMessageEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.ChatId == "" || ev.Id == "" {
			return nil, warpnet.WarpError("chat ID, user ID, or message ID cannot be blank")
		}

		chat, err := repo.GetChat(ev.ChatId)
		if err != nil {
			return nil, err
		}

		ownerId := authRepo.GetOwner().UserId
		if chat.OwnerId != ownerId && chat.OtherUserId != ownerId {
			return nil, warpnet.WarpError("not authorized for this chat")
		}

		msg, err := repo.GetMessage(ev.ChatId, ev.Id)
		if err != nil {
			return nil, err
		}

		return event.ChatMessageResponse(msg), nil
	}
}

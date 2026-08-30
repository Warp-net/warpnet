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
	"github.com/Warp-net/warpnet/core/authorship"
	"github.com/Warp-net/warpnet/core/mastodon"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const (
	messageLimit      = 5000
	mediaKeyLimit     = 128
	maxMessageImages  = 4
	statusUndelivered = "undelivered"
)

type ChatAuthStorer interface {
	GetOwner() domain.Owner
}

type ChatStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
	PairedDeviceIDs() []string
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

		otherUser, otherUserErr := userRepo.Get(ev.OtherUserId)
		if otherUser.Network == mastodon.Network {
			return nil, mastodon.ErrNotSupported
		}

		initiator, _ := userRepo.Get(ev.OwnerId)
		if err := authorship.VerifyAuthor(streamer, s, initiator.NodeId); err != nil {
			return nil, err
		}

		ownNodeInfo := streamer.NodeInfo()
		isSelfChat := ev.OwnerId == ev.OtherUserId
		isOtherUserChat := ev.OwnerId != ownNodeInfo.OwnerId

		ownerChat, err := repo.CreateChat(ev.ChatId, ev.OwnerId, ev.OtherUserId)
		if err != nil {
			return nil, err
		}

		if isSelfChat {
			return event.ChatCreatedResponse(ownerChat), nil
		}

		if isOtherUserChat {
			log.Infoln("new chat!")
			return event.ChatCreatedResponse(ownerChat), nil
		}

		if errors.Is(otherUserErr, database.ErrUserNotFound) {
			return event.ChatCreatedResponse(ownerChat), nil
		}
		if otherUserErr != nil {
			return nil, otherUserErr
		}

		if ownNodeInfo.ID.String() == otherUser.NodeId {
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
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return event.ChatCreatedResponse(ownerChat), nil
		}
		if errors.Is(err, node.ErrSelfRequest) {
			return event.ChatCreatedResponse(ownerChat), nil
		}
		if err != nil {
			log.Errorf("create chat: stream: %v", err)
			return event.ChatCreatedResponse(ownerChat), nil
		}

		var possibleError event.ResponseError
		if _ = json.Unmarshal(otherChatData, &possibleError); possibleError.Code != 0 {
			log.Errorf("create chat: unmarshal other reply response: %s", possibleError.Message)
		}

		return event.ChatCreatedResponse(ownerChat), nil
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
		isMeParticipating := chat.OwnerId == ownerId || chat.OtherUserId == ownerId
		if !isMeParticipating {
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
		isMeParticipating := chat.OwnerId == ownerId || chat.OtherUserId == ownerId
		if !isMeParticipating {
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

// mediaKey normalizes an attachment key: an absent one and an empty one mean
// the same thing, and an oversized one is rejected outright.
func mediaKey(key *string) (*string, bool) {
	if key == nil || *key == "" {
		return nil, true
	}
	if len(*key) > mediaKeyLimit {
		return nil, false
	}
	return key, true
}

// mediaKeys drops the empty entries a client may pad the list with and refuses
// a list that is oversized in either dimension.
func mediaKeys(keys []string) ([]string, bool) {
	if len(keys) > maxMessageImages {
		return nil, false
	}
	var kept []string
	for _, key := range keys {
		if key == "" {
			continue
		}
		if len(key) > mediaKeyLimit {
			return nil, false
		}
		kept = append(kept, key)
	}
	return kept, true
}

type ChatNotifier interface {
	Add(not domain.Notification) error
}

// StreamNewMessageHandler is for sending a new message
func StreamNewMessageHandler(repo ChatStorer, userRepo ChatUserFetcher, notifyRepo ChatNotifier, streamer ChatStreamer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.NewMessageEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.ChatId == "" || !strings.Contains(ev.ChatId, ":") {
			return nil, warpnet.WarpError("message parameters are invalid")
		}

		if ev.SenderId == "" || ev.ReceiverId == "" {
			return nil, warpnet.WarpError("sender and receiver parameters are invalid")
		}

		ownNodeInfo := streamer.NodeInfo()
		ownerId := ownNodeInfo.OwnerId

		isMeParticipating := ev.SenderId == ownerId || ev.ReceiverId == ownerId
		if !isMeParticipating {
			return nil, warpnet.WarpError("not authorized to send message to this chat")
		}

		sender, _ := userRepo.Get(ev.SenderId)
		if err := authorship.VerifyAuthor(streamer, s, sender.NodeId); err != nil {
			return nil, err
		}

		imageKeys, areImageKeysValid := mediaKeys(ev.ImageKeys)
		videoKey, isVideoKeyValid := mediaKey(ev.VideoKey)
		if !areImageKeysValid || !isVideoKeyValid {
			return nil, warpnet.WarpError("message attachment key is invalid")
		}
		if ev.Text == "" && len(imageKeys) == 0 && videoKey == nil {
			return nil, warpnet.WarpError("message parameters are invalid")
		}
		if utf8.RuneCountInString(ev.Text) > messageLimit {
			return nil, warpnet.WarpError("message is too long")
		}

		chat, err := repo.GetChat(ev.ChatId)
		if err != nil && !errors.Is(err, database.ErrChatNotFound) {
			return nil, err
		}

		isOwnerReceiver := ownerId == ev.ReceiverId // if message isn't associated with a chat
		if isOwnerReceiver && errors.Is(err, database.ErrChatNotFound) {
			chat, err = repo.CreateChat(&ev.ChatId, ev.SenderId, ownerId)
			if err != nil {
				return nil, err
			}
		}

		isMeParticipating = chat.OwnerId == ownerId || chat.OtherUserId == ownerId
		if !isMeParticipating {
			return nil, warpnet.WarpError("not authorized for this chat")
		}

		isSelfChat := ev.SenderId == ev.ReceiverId

		var (
			otherUser    domain.User
			otherUserErr error
		)
		if !isSelfChat && !isOwnerReceiver {
			otherUser, otherUserErr = userRepo.Get(ev.ReceiverId)
			if otherUser.Network == mastodon.Network {
				return nil, mastodon.ErrNotSupported
			}
		}

		now := time.Now()
		msg := domain.ChatMessage{
			Id:         ev.Id,
			ChatId:     ev.ChatId,
			SenderId:   ev.SenderId,
			ReceiverId: ev.ReceiverId,
			Text:       ev.Text,
			ImageKeys:  imageKeys,
			VideoKey:   videoKey,
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
			if err := notifyRepo.Add(domain.Notification{
				Type:        domain.NotificationMessageType,
				Text:        sender.Username + " sent you a message",
				RecepientId: ownerId,
				ActorId:     ev.SenderId,
			}); err != nil {
				log.Errorf("chat message: adding notification: %v", err)
			}
			return event.NewMessageResponse(msg), nil
		}

		if errors.Is(otherUserErr, database.ErrUserNotFound) {
			return msg, nil
		}
		if otherUserErr != nil {
			log.Errorf("chat message: resolve receiver %s: %v", ev.ReceiverId, otherUserErr)
			msg.Status = statusUndelivered
			return msg, nil
		}

		if ownNodeInfo.ID.String() == otherUser.NodeId {
			return msg, nil
		}
		otherMsgData, err := streamer.GenericStream(
			otherUser.NodeId,
			event.PUBLIC_POST_MESSAGE,
			domain.ChatMessage{
				Id:         msg.Id,
				ChatId:     ev.ChatId,
				SenderId:   ownerId,
				ReceiverId: ev.ReceiverId,
				Text:       ev.Text,
				ImageKeys:  imageKeys,
				VideoKey:   videoKey,
				CreatedAt:  now,
			},
		)
		if err != nil {
			log.Warnf("chat message not delivered to node %s: %v", otherUser.NodeId, err)
			msg.Status = statusUndelivered
			return msg, nil
		}

		var possibleError event.ResponseError
		if _ = json.Unmarshal(otherMsgData, &possibleError); possibleError.Code != 0 {
			log.Errorf("unmarshal other message error response: %s", possibleError.Message)
			msg.Status = statusUndelivered
		}

		return msg, err
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
		isMeParticipating := chat.OwnerId == ownerId || chat.OtherUserId == ownerId
		if !isMeParticipating {
			return nil, warpnet.WarpError("not authorized for this chat")
		}

		return event.Accepted, repo.DeleteMessage(ev.ChatId, ev.Id)
	}
}

// Handler for getting messages in a chat
func StreamGetMessagesHandler(repo ChatStorer, _ OwnerChatsStorer) warpnet.WarpHandlerFunc {
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

		ownerId := ev.OwnerId
		isMeParticipating := chat.OwnerId == ownerId || chat.OtherUserId == ownerId
		if !isMeParticipating {
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
		isMeParticipating := chat.OwnerId == ownerId || chat.OtherUserId == ownerId
		if !isMeParticipating {
			return nil, warpnet.WarpError("not authorized for this chat")
		}

		msg, err := repo.GetMessage(ev.ChatId, ev.Id)
		if err != nil {
			return nil, err
		}

		return event.ChatMessageResponse(msg), nil
	}
}

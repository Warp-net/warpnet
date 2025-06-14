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

package database

import (
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/database/storage"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/oklog/ulid/v2"
	"sync"
	"time"
)

var (
	ErrChatNotFound    = errors.New("chat not found")
	ErrMessageNotFound = errors.New("message not found")
)

const (
	ChatNamespace     = "/CHATS"
	MessageNamespace  = "/MESSAGES"
	NonceSubNamespace = "NONCE"
)

type ChatStorer interface {
	NewTxn() (storage.WarpTransactioner, error)
}

type ChatRepo struct {
	db ChatStorer
	mx *sync.Mutex
}

func NewChatRepo(db ChatStorer) *ChatRepo {
	return &ChatRepo{db: db, mx: new(sync.Mutex)}
}

func (repo *ChatRepo) CreateChat(chatId *string, ownerId, otherUserId string) (chat domain.Chat, err error) {
	if ownerId == "" || otherUserId == "" {
		return domain.Chat{}, errors.New("user ID or other user ID is empty")
	}

	repo.mx.Lock()
	defer repo.mx.Unlock()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return domain.Chat{}, err
	}
	defer txn.Rollback()

	if chatId == nil || *chatId == "" {
		chatId = new(string)
		*chatId = repo.composeChatId(ownerId, otherUserId)
	}
	fixedUserChatKey := storage.NewPrefixBuilder(ChatNamespace).
		AddRootID(*chatId).
		AddRange(storage.FixedRangeKey).
		Build()

	sortableUserChatKey := storage.NewPrefixBuilder(ChatNamespace).
		AddRootID(*chatId).
		AddReversedTimestamp(time.Now()).
		Build()

	chat.Id = *chatId

	// check if already exist
	bt, err := txn.Get(sortableUserChatKey)
	if err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return chat, err
	}
	if err == nil {
		if err = json.JSON.Unmarshal(bt, &chat); err != nil {
			return chat, err
		}
		return chat, txn.Commit()
	}

	// create new chat
	chat = domain.Chat{
		CreatedAt:   time.Now(),
		Id:          *chatId,
		OtherUserId: otherUserId,
		UpdatedAt:   time.Now(),
		OwnerId:     ownerId,
	}

	bt, err = json.JSON.Marshal(chat)
	if err != nil {
		return chat, err
	}
	err = txn.Set(fixedUserChatKey, sortableUserChatKey.Bytes())
	if err != nil {
		return chat, err
	}
	err = txn.Set(sortableUserChatKey, bt)
	if err != nil {
		return chat, err
	}
	return chat, txn.Commit()
}

func (repo *ChatRepo) DeleteChat(chatId string) error {
	if chatId == "" {
		return errors.New("chat ID is empty")
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	fixedUserChatKey := storage.NewPrefixBuilder(ChatNamespace).
		AddRootID(chatId).
		AddRange(storage.FixedRangeKey).
		Build()

	sortableKey, err := txn.Get(fixedUserChatKey)
	if err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return err
	}
	if len(sortableKey) == 0 {
		return ErrChatNotFound
	}
	if err := txn.Delete(fixedUserChatKey); err != nil {
		return err
	}
	if err := txn.Delete(storage.DatabaseKey(sortableKey)); err != nil {
		return err
	}

	return txn.Commit()
}

func (repo *ChatRepo) GetChat(chatId string) (chat domain.Chat, err error) {
	if chatId == "" {
		return chat, errors.New("chat ID is empty")
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return chat, err
	}
	defer txn.Rollback()

	fixedUserChatKey := storage.NewPrefixBuilder(ChatNamespace).
		AddRootID(chatId).
		AddRange(storage.FixedRangeKey).
		Build()

	sortableKey, err := txn.Get(fixedUserChatKey)
	if err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return chat, err
	}
	if len(sortableKey) == 0 {
		return chat, ErrChatNotFound
	}
	bt, err := txn.Get(storage.DatabaseKey(sortableKey))
	if err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return chat, err
	}

	if err = json.JSON.Unmarshal(bt, &chat); err != nil {
		return chat, err
	}
	return chat, txn.Commit()
}

func (repo *ChatRepo) GetUserChats(userId string, limit *uint64, cursor *string) ([]domain.Chat, string, error) {
	if userId == "" {
		return []domain.Chat{}, "", errors.New("ID cannot be blank")
	}

	prefix := storage.NewPrefixBuilder(ChatNamespace).Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return []domain.Chat{}, "", err
	}
	defer txn.Rollback()

	items, cur, err := txn.List(prefix, limit, cursor)
	if err != nil {
		return []domain.Chat{}, cur, err
	}

	if err := txn.Commit(); err != nil {
		return []domain.Chat{}, cur, err
	}

	chats := make([]domain.Chat, 0, len(items))
	for _, item := range items {
		var chat domain.Chat
		err = json.JSON.Unmarshal(item.Value, &chat)
		if err != nil {
			err = fmt.Errorf(
				"failed to unmarshal chat: key: %s, value: %s, message: %w",
				item.Key, item.Value, err,
			)
			return chats, cur, err
		}
		chats = append(chats, chat)
	}

	return chats, cur, nil
}

func (repo *ChatRepo) CreateMessage(msg domain.ChatMessage) (domain.ChatMessage, error) {
	if msg == (domain.ChatMessage{}) {
		return msg, errors.New("empty message")
	}
	if msg.ChatId == "" {
		return msg, errors.New("chat ID is empty")
	}

	repo.mx.Lock()
	defer repo.mx.Unlock()

	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	if msg.Id == "" {
		msg.Id = ulid.Make().String()
	}

	fixedKey := storage.NewPrefixBuilder(MessageNamespace).
		AddRootID(msg.ChatId).
		AddRange(storage.FixedRangeKey).
		AddParentId(msg.Id).
		Build()

	sortableKey := storage.NewPrefixBuilder(MessageNamespace).
		AddRootID(msg.ChatId).
		AddReversedTimestamp(msg.CreatedAt).
		AddParentId(msg.Id).
		Build()

	data, err := json.JSON.Marshal(msg)
	if err != nil {
		return msg, fmt.Errorf("message: marshal: %w", err)
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return msg, err
	}
	defer txn.Rollback()

	if err = txn.Set(fixedKey, sortableKey.Bytes()); err != nil {
		return msg, err
	}
	err = txn.Set(sortableKey, data)
	if err != nil {
		return msg, err
	}
	return msg, txn.Commit()
}

func (repo *ChatRepo) ListMessages(chatId string, limit *uint64, cursor *string) ([]domain.ChatMessage, string, error) {
	if chatId == "" {
		return nil, "", errors.New("chat ID cannot be blank")
	}

	prefix := storage.NewPrefixBuilder(MessageNamespace).
		AddRootID(chatId).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return nil, "", err
	}
	defer txn.Rollback()

	items, cur, err := txn.List(prefix, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	if err := txn.Commit(); err != nil {
		return nil, cur, err
	}

	chatsMsgs := make([]domain.ChatMessage, 0, len(items))
	for _, item := range items {
		var chatMsg domain.ChatMessage
		err = json.JSON.Unmarshal(item.Value, &chatMsg)
		if err != nil {
			return nil, "", err
		}
		chatsMsgs = append(chatsMsgs, chatMsg)
	}

	return chatsMsgs, cur, nil
}

func (repo *ChatRepo) GetMessage(chatId, id string) (m domain.ChatMessage, err error) {
	if chatId == "" || id == "" {
		return m, errors.New("chatId or id cannot be blank")
	}
	fixedKey := storage.NewPrefixBuilder(MessageNamespace).
		AddRootID(chatId).
		AddRange(storage.FixedRangeKey).
		AddParentId(id).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return m, err
	}
	defer txn.Rollback()

	sortableKey, err := txn.Get(fixedKey)
	if err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return m, err
	}
	if len(sortableKey) == 0 {
		return m, ErrMessageNotFound
	}

	data, err := txn.Get(storage.DatabaseKey(sortableKey))
	if err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return m, err
	}

	if err := txn.Commit(); err != nil {
		return m, err
	}

	err = json.JSON.Unmarshal(data, &m)
	if err != nil {
		return m, err
	}
	return m, nil
}

func (repo *ChatRepo) DeleteMessage(chatId, id string) error {
	fixedKey := storage.NewPrefixBuilder(MessageNamespace).
		AddRootID(chatId).
		AddRange(storage.FixedRangeKey).
		AddParentId(id).
		Build()

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	sortableKey, err := txn.Get(fixedKey)
	if err != nil && !errors.Is(err, storage.ErrKeyNotFound) {
		return err
	}
	if len(sortableKey) == 0 {
		return ErrMessageNotFound
	}
	if err = txn.Delete(fixedKey); err != nil {
		return err
	}
	if err = txn.Delete(storage.DatabaseKey(sortableKey)); err != nil {
		return err
	}

	return txn.Commit()
}

// TODO access this approach
// ULID consist of 26 symbols: first 10 symbols contain timestamp, last ones - random
func (repo *ChatRepo) composeChatId(ownerId, otherUserId string) string {
	randomPartOwnerId := ownerId[14:]
	randomPartOtherId := otherUserId[14:]
	if randomPartOwnerId > randomPartOtherId {
		randomPartOwnerId, randomPartOtherId = randomPartOtherId, randomPartOwnerId
	}
	return fmt.Sprintf("%s:%s", randomPartOwnerId, randomPartOtherId)
}

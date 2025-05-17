/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU General Public License for more details.

 You should have received a copy of the GNU General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

package database

import (
	"crypto/rand"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/database/storage"
	"github.com/Warp-net/warpnet/domain"
	"github.com/oklog/ulid/v2"
)

const testUserID = "01BX5ZZKBKACTAV9WEVGEMTEST"

func setupChatRepo() (*ChatRepo, func(), error) {
	var err error
	db, err := storage.New(".", true, "")
	if err != nil {
		return nil, nil, err
	}
	dbCloseFunc := db.Close

	authRepo := NewAuthRepo(db)

	err = authRepo.Authenticate(rand.Text(), rand.Text())
	if err != nil {
		return nil, dbCloseFunc, err
	}

	return NewChatRepo(db), dbCloseFunc, nil
}

func TestCreateAndGetChat(t *testing.T) {
	defer goleak.VerifyNone(t)

	ownerID := testUserID
	otherID := ulid.Make().String()

	repo, closeF, err := setupChatRepo()
	assert.NoError(t, err)
	defer closeF()

	chat, err := repo.CreateChat(nil, ownerID, otherID)
	assert.NoError(t, err)

	fetched, err := repo.GetChat(chat.Id)
	assert.NoError(t, err)
	assert.Equal(t, chat.Id, fetched.Id)
	assert.Equal(t, chat.OwnerId, fetched.OwnerId)
	assert.Equal(t, chat.OtherUserId, fetched.OtherUserId)

	closeWriter()

}

func TestDeleteChat(t *testing.T) {
	defer goleak.VerifyNone(t)

	ownerID := testUserID
	otherID := ulid.Make().String()

	repo, closeF, err := setupChatRepo()
	assert.NoError(t, err)
	defer closeF()

	chat, err := repo.CreateChat(nil, ownerID, otherID)
	assert.NoError(t, err)
	assert.NotEmpty(t, chat.Id)

	err = repo.DeleteChat(chat.Id)
	assert.NoError(t, err)

	deleted, err := repo.GetChat(chat.Id)
	assert.Error(t, err)
	assert.Empty(t, deleted.Id)
	closeWriter()

}

func TestGetUserChats(t *testing.T) {
	defer goleak.VerifyNone(t)

	userID := testUserID

	repo, closeF, err := setupChatRepo()
	assert.NoError(t, err)
	defer closeF()

	for i := 0; i < 3; i++ {
		other := ulid.Make().String()
		_, err := repo.CreateChat(nil, userID, other)
		assert.NoError(t, err)
	}

	limit := uint64(10)
	chats, cursor, err := repo.GetUserChats(userID, &limit, nil)
	assert.NoError(t, err)
	assert.Len(t, chats, 3)
	assert.Equal(t, cursor, "end")

	closeWriter()

}

func TestCreateAndGetMessage(t *testing.T) {
	defer goleak.VerifyNone(t)

	ownerID := testUserID
	otherID := ulid.Make().String()

	repo, closeF, err := setupChatRepo()
	assert.NoError(t, err)
	defer closeF()

	chat, _ := repo.CreateChat(nil, ownerID, otherID)

	msg := domain.ChatMessage{
		ChatId: chat.Id,
		Text:   "hello",
	}

	created, err := repo.CreateMessage(msg)
	assert.NoError(t, err)

	got, err := repo.GetMessage(chat.Id, created.Id)
	assert.NoError(t, err)
	assert.Equal(t, msg.Text, got.Text)

	closeWriter()

}

func TestListMessages(t *testing.T) {
	defer goleak.VerifyNone(t)

	ownerID := testUserID
	otherID := ulid.Make().String()

	repo, closeF, err := setupChatRepo()
	assert.NoError(t, err)
	defer closeF()

	chat, err := repo.CreateChat(nil, ownerID, otherID)
	assert.NoError(t, err)
	assert.NotEmpty(t, chat.Id)

	for i := 0; i < 5; i++ {
		msg := domain.ChatMessage{
			ChatId:    chat.Id,
			Text:      "msg",
			CreatedAt: time.Now().Add(-time.Duration(i) * time.Second),
		}
		_, err := repo.CreateMessage(msg)
		assert.NoError(t, err)
	}

	limit := uint64(10)
	msgs, cursor, err := repo.ListMessages(chat.Id, &limit, nil)
	assert.NoError(t, err)
	assert.Len(t, msgs, 5)
	assert.Equal(t, cursor, "end")

	closeWriter()

}

func TestDeleteMessage(t *testing.T) {
	defer goleak.VerifyNone(t)

	ownerID := testUserID
	otherID := ulid.Make().String()

	repo, closeF, err := setupChatRepo()
	assert.NoError(t, err)
	defer closeF()

	chat, err := repo.CreateChat(nil, ownerID, otherID)
	assert.NoError(t, err)
	assert.NotEmpty(t, chat.Id)

	msg := domain.ChatMessage{
		ChatId: chat.Id,
		Text:   "to delete",
	}
	created, _ := repo.CreateMessage(msg)

	err = repo.DeleteMessage(chat.Id, created.Id)
	assert.NoError(t, err)

	got, err := repo.GetMessage(chat.Id, created.Id)
	assert.Error(t, err)
	assert.Empty(t, got.Text)

	closeWriter()

}

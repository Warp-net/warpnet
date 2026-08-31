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

//nolint:all
package database

import (
	"testing"

	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

func TestChatRepoErrorPaths(t *testing.T) {
	// composeChatId slices the id at byte 14, so the participants must be ULIDs.
	ownerId, otherId := ulid.Make().String(), ulid.Make().String()
	chatId := NewChatRepo(nil).composeChatId(ownerId, otherId)

	seedChat := func(t *testing.T, s *faultStore) {
		_, err := NewChatRepo(s).CreateChat(nil, ownerId, otherId)
		require.NoError(t, err)
	}
	message := func() domain.ChatMessage {
		return domain.ChatMessage{
			ChatId:     chatId,
			Id:         "msg-1",
			SenderId:   ownerId,
			ReceiverId: otherId,
			Text:       "hi",
		}
	}
	seedMessage := func(t *testing.T, s *faultStore) {
		seedChat(t, s)
		_, err := NewChatRepo(s).CreateMessage(message())
		require.NoError(t, err)
	}

	runFaultCases(t, []faultCase{
		{
			name: "CreateChat",
			run: func(s *faultStore) error {
				_, err := NewChatRepo(s).CreateChat(nil, ownerId, otherId)
				return err
			},
			ops: []faultOp{op("Get"), op("Set"), opN("Set", 2), op("Commit")},
		},
		{
			name: "CreateChatExisting",
			seed: seedChat,
			run: func(s *faultStore) error {
				_, err := NewChatRepo(s).CreateChat(&chatId, ownerId, otherId)
				return err
			},
			ops: []faultOp{op("Get"), opN("Get", 2), op("Commit")},
		},
		{
			name: "DeleteChat",
			seed: seedChat,
			run:  func(s *faultStore) error { return NewChatRepo(s).DeleteChat(chatId) },
			ops:  []faultOp{op("Get"), op("Delete"), opN("Delete", 2), op("Commit")},
		},
		{
			name: "GetChat",
			seed: seedChat,
			run: func(s *faultStore) error {
				_, err := NewChatRepo(s).GetChat(chatId)
				return err
			},
			ops: []faultOp{op("Get"), opN("Get", 2), op("Commit")},
		},
		{
			name: "GetUserChats",
			seed: seedChat,
			run: func(s *faultStore) error {
				_, _, err := NewChatRepo(s).GetUserChats(ownerId, nil, nil)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
		{
			name: "CreateMessage",
			seed: seedChat,
			run: func(s *faultStore) error {
				_, err := NewChatRepo(s).CreateMessage(message())
				return err
			},
			ops: []faultOp{op("Get"), op("Set"), opN("Set", 2), opN("Get", 2), opN("Get", 3), op("Commit")},
		},
		{
			name: "CreateMessageExisting",
			seed: seedMessage,
			run: func(s *faultStore) error {
				_, err := NewChatRepo(s).CreateMessage(message())
				return err
			},
			ops: []faultOp{op("Get"), opN("Get", 2), op("Commit")},
		},
		{
			name: "ListMessages",
			seed: seedMessage,
			run: func(s *faultStore) error {
				_, _, err := NewChatRepo(s).ListMessages(chatId, nil, nil)
				return err
			},
			ops: []faultOp{op("List"), op("Commit")},
		},
		{
			name: "GetMessage",
			seed: seedMessage,
			run: func(s *faultStore) error {
				_, err := NewChatRepo(s).GetMessage(chatId, "msg-1")
				return err
			},
			ops: []faultOp{op("Get"), opN("Get", 2), op("Commit")},
		},
		{
			name: "DeleteMessage",
			seed: seedMessage,
			run:  func(s *faultStore) error { return NewChatRepo(s).DeleteMessage(chatId, "msg-1") },
			ops:  []faultOp{op("Get"), op("Delete"), opN("Delete", 2), op("Commit")},
		},
	})

	t.Run("Validation", func(t *testing.T) {
		repo := NewChatRepo(newFaultStore(t))

		_, err := repo.CreateChat(nil, "", otherId)
		require.Error(t, err)
		_, err = repo.CreateChat(nil, ownerId, "")
		require.Error(t, err)
		require.Error(t, repo.DeleteChat(""))
		_, err = repo.GetChat("")
		require.Error(t, err)
		_, _, err = repo.GetUserChats("", nil, nil)
		require.Error(t, err)

		_, err = repo.CreateMessage(domain.ChatMessage{})
		require.Error(t, err)
		_, err = repo.CreateMessage(domain.ChatMessage{Text: "no chat id"})
		require.Error(t, err)

		_, _, err = repo.ListMessages("", nil, nil)
		require.Error(t, err)
		_, err = repo.GetMessage("", "msg-1")
		require.Error(t, err)
		_, err = repo.GetMessage(chatId, "")
		require.Error(t, err)
	})

	t.Run("NotFound", func(t *testing.T) {
		repo := NewChatRepo(newFaultStore(t))
		_, err := repo.GetChat("absent-chat")
		require.ErrorIs(t, err, ErrChatNotFound)
		require.ErrorIs(t, repo.DeleteChat("absent-chat"), ErrChatNotFound)

		_, err = repo.GetMessage(chatId, "absent-msg")
		require.ErrorIs(t, err, ErrMessageNotFound)
		require.ErrorIs(t, repo.DeleteMessage(chatId, "absent-msg"), ErrMessageNotFound)
	})

	t.Run("CorruptPayload", func(t *testing.T) {
		s := newFaultStore(t)
		seedMessage(t, s)

		sortableChat, err := s.db.Get(local_store.NewPrefixBuilder(ChatNamespace).
			AddRootID(chatId).
			AddRange(local_store.FixedRangeKey).
			Build())
		require.NoError(t, err)
		require.NoError(t, s.db.Set(local_store.DatabaseKey(sortableChat), []byte("{not-json")))

		repo := NewChatRepo(s)
		_, err = repo.GetChat(chatId)
		require.Error(t, err)
		_, _, err = repo.GetUserChats(ownerId, nil, nil)
		require.Error(t, err)
		// a corrupt chat record also fails the preview bump on the next message
		_, err = repo.CreateMessage(domain.ChatMessage{ChatId: chatId, Id: "msg-2", Text: "second"})
		require.Error(t, err)

		sortableMsg, err := s.db.Get(local_store.NewPrefixBuilder(MessageNamespace).
			AddRootID(chatId).
			AddRange(local_store.FixedRangeKey).
			AddParentId("msg-1").
			Build())
		require.NoError(t, err)
		require.NoError(t, s.db.Set(local_store.DatabaseKey(sortableMsg), []byte("{not-json")))

		_, err = repo.GetMessage(chatId, "msg-1")
		require.Error(t, err)
		_, _, err = repo.ListMessages(chatId, nil, nil)
		require.Error(t, err)
	})

	t.Run("MessageWithoutChatSkipsPreview", func(t *testing.T) {
		repo := NewChatRepo(newFaultStore(t))
		msg, err := repo.CreateMessage(domain.ChatMessage{ChatId: "orphan-chat", Id: "orphan-msg", Text: "hi"})
		require.NoError(t, err)
		require.Equal(t, "orphan-msg", msg.Id)
	})

	t.Run("AttachmentPreview", func(t *testing.T) {
		s := newFaultStore(t)
		seedChat(t, s)
		repo := NewChatRepo(s)

		_, err := repo.CreateMessage(domain.ChatMessage{
			ChatId:    chatId,
			Id:        "msg-image",
			ImageKeys: []string{"img-key"},
		})
		require.NoError(t, err)

		chat, err := repo.GetChat(chatId)
		require.NoError(t, err)
		require.Equal(t, "Attachment", chat.LastMessage)
	})

	t.Run("LongPreviewIsTruncated", func(t *testing.T) {
		s := newFaultStore(t)
		seedChat(t, s)
		repo := NewChatRepo(s)

		long := make([]rune, lastMessagePreviewLimit*2)
		for i := range long {
			long[i] = 'ы'
		}
		_, err := repo.CreateMessage(domain.ChatMessage{ChatId: chatId, Id: "msg-long", Text: string(long)})
		require.NoError(t, err)

		chat, err := repo.GetChat(chatId)
		require.NoError(t, err)
		require.Len(t, []rune(chat.LastMessage), lastMessagePreviewLimit)
	})

	t.Run("CreateMessageIsIdempotent", func(t *testing.T) {
		s := newFaultStore(t)
		seedChat(t, s)
		repo := NewChatRepo(s)

		first, err := repo.CreateMessage(message())
		require.NoError(t, err)
		second, err := repo.CreateMessage(message())
		require.NoError(t, err)
		require.Equal(t, first.Id, second.Id)

		msgs, _, err := repo.ListMessages(chatId, nil, nil)
		require.NoError(t, err)
		require.Len(t, msgs, 1)
	})

	t.Run("CreateChatIsIdempotent", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewChatRepo(s)

		first, err := repo.CreateChat(nil, ownerId, otherId)
		require.NoError(t, err)
		second, err := repo.CreateChat(nil, ownerId, otherId)
		require.NoError(t, err)
		require.Equal(t, first.Id, second.Id)
		require.Equal(t, first.CreatedAt.UnixNano(), second.CreatedAt.UnixNano())
	})
}

func TestAuthRepoErrorPaths(t *testing.T) {
	t.Run("NilRepo", func(t *testing.T) {
		var repo *AuthRepo
		require.ErrorIs(t, repo.Authenticate("user", "pass"), ErrNilAuthRepo)
		require.Nil(t, repo.PrivateKey())
		require.Panics(t, func() { repo.GetOwner() })
	})

	t.Run("NilStore", func(t *testing.T) {
		repo := NewAuthRepo(nil, "testnet")
		require.ErrorIs(t, repo.Authenticate("user", "pass"), local_store.ErrNotRunning)
	})

	t.Run("EmptyCredentials", func(t *testing.T) {
		repo := NewAuthRepo(newFaultStore(t), "testnet")
		require.Error(t, repo.Authenticate("", ""))
	})

	t.Run("PrivateKeyBeforeAuthPanics", func(t *testing.T) {
		repo := NewAuthRepo(newFaultStore(t), "testnet")
		require.Panics(t, func() { repo.PrivateKey() })
	})

	t.Run("SessionTokenIsStableAcrossReauth", func(t *testing.T) {
		repo := NewAuthRepo(newFaultStore(t), "testnet")
		require.NoError(t, repo.Authenticate("test", "test"))

		token := repo.SessionToken()
		require.NotEmpty(t, token)
		require.NotNil(t, repo.PrivateKey())

		require.NoError(t, repo.Authenticate("test", "test"))
		require.Equal(t, token, repo.SessionToken())
	})

	t.Run("OwnerRoundTrip", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewAuthRepo(s, "testnet")
		require.NoError(t, repo.Authenticate("test", "test"))

		require.Equal(t, domain.Owner{}, repo.GetOwner())

		owner, err := repo.SetOwner(domain.Owner{UserId: "owner-1", Username: "owner"})
		require.NoError(t, err)
		require.False(t, owner.CreatedAt.IsZero())
		require.Equal(t, "owner-1", repo.GetOwner().UserId)

		// a cold repo reads the owner back from storage and backfills the
		// redundant id
		cold := NewAuthRepo(s, "testnet")
		got := cold.GetOwner()
		require.Equal(t, "owner-1", got.UserId)
		require.Equal(t, "owner-1", got.RedundantUserID)
	})

	t.Run("SetOwnerStoreFails", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewAuthRepo(s, "testnet")
		require.NoError(t, repo.Authenticate("test", "test"))

		s.arm("db.Set", 1)
		_, err := repo.SetOwner(domain.Owner{UserId: "owner-1"})
		require.ErrorIs(t, err, errFault)
	})

	t.Run("GetOwnerStoreFailsPanics", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewAuthRepo(s, "testnet")
		require.NoError(t, repo.Authenticate("test", "test"))

		s.arm("db.Get", 1)
		require.Panics(t, func() { repo.GetOwner() })
	})

	t.Run("GetOwnerCorruptPayloadPanics", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewAuthRepo(s, "testnet")
		require.NoError(t, repo.Authenticate("test", "test"))

		ownerKey := local_store.NewPrefixBuilder(AuthRepoName).
			AddRootID(DefaultOwnerKey).
			Build()
		require.NoError(t, s.db.Set(ownerKey, []byte("{not-json")))

		require.Panics(t, func() { repo.GetOwner() })
	})

	t.Run("Logout", func(t *testing.T) {
		s := newFaultStore(t)
		repo := NewAuthRepo(s, "testnet")
		require.NoError(t, repo.Authenticate("test", "test"))

		repo.Logout()
		require.True(t, s.IsClosed())
	})
}

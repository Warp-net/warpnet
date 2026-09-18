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
	"encoding/hex"
	"time"

	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/security"
)

// Chat attachments live in their own namespace, keyed by the chat rather than
// by a user. The public media routes read /MEDIA and cannot reach a blob
// stored here at all, so a private picture is out of their reach by layout
// instead of by a permission check.
const (
	ChatMediaRepoName = "/CHATMEDIA"
)

var ErrChatMediaRepoNotInit = local_store.DBError("chat media repo is not initialized")

type ChatMediaStorer interface {
	Set(key local_store.DatabaseKey, value []byte) error
	Get(key local_store.DatabaseKey) ([]byte, error)
	SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error
}

type ChatMediaRepo struct {
	db ChatMediaStorer
}

func NewChatMediaRepo(db ChatMediaStorer) *ChatMediaRepo {
	return &ChatMediaRepo{db: db}
}

func (repo *ChatMediaRepo) mediaKey(subNamespace, chatId, key string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(ChatMediaRepoName).
		AddRootID(subNamespace).
		AddParentId(chatId).
		AddId(key).
		Build()
}

func (repo *ChatMediaRepo) GetChatImage(chatId, key string) (domain.Base64Image, error) {
	if repo == nil {
		return "", ErrChatMediaRepoNotInit
	}
	if chatId == "" || key == "" {
		return "", ErrMediaNotFound
	}

	data, err := repo.db.Get(repo.mediaKey(ImageSubNamespace, chatId, key))
	if local_store.IsNotFoundError(err) {
		return "", ErrMediaNotFound
	}
	return domain.Base64Image(data), err
}

func (repo *ChatMediaRepo) SetChatImage(chatId string, img domain.Base64Image) (_ domain.ImageKey, err error) {
	if repo == nil {
		return "", ErrChatMediaRepoNotInit
	}
	if chatId == "" {
		return "", local_store.DBError("empty chat id for chat image set")
	}
	if len(img) == 0 {
		return "", local_store.DBError("no data for chat image set")
	}

	key := hex.EncodeToString(security.ConvertToSHA256([]byte(img)))
	return domain.ImageKey(key), repo.db.Set(repo.mediaKey(ImageSubNamespace, chatId, key), []byte(img))
}

func (repo *ChatMediaRepo) SetForeignChatImageWithTTL(chatId, key string, img domain.Base64Image) error {
	if repo == nil {
		return ErrChatMediaRepoNotInit
	}
	if chatId == "" || key == "" {
		return local_store.DBError("empty chat id or key for chat image set")
	}
	if len(img) == 0 {
		return local_store.DBError("no data for chat image set")
	}

	return repo.db.SetWithTTL(repo.mediaKey(ImageSubNamespace, chatId, key), []byte(img), chatMediaTTL)
}

func (repo *ChatMediaRepo) GetChatVideo(chatId, key string) (domain.Base64Video, error) {
	if repo == nil {
		return "", ErrChatMediaRepoNotInit
	}
	if chatId == "" || key == "" {
		return "", ErrMediaNotFound
	}

	data, err := repo.db.Get(repo.mediaKey(VideoSubNamespace, chatId, key))
	if local_store.IsNotFoundError(err) {
		return "", ErrMediaNotFound
	}
	return domain.Base64Video(data), err
}

func (repo *ChatMediaRepo) SetChatVideo(chatId string, video domain.Base64Video) (_ domain.VideoKey, err error) {
	if repo == nil {
		return "", ErrChatMediaRepoNotInit
	}
	if chatId == "" {
		return "", local_store.DBError("empty chat id for chat video set")
	}
	if len(video) == 0 {
		return "", local_store.DBError("no data for chat video set")
	}

	key := hex.EncodeToString(security.ConvertToSHA256([]byte(video)))
	return domain.VideoKey(key), repo.db.Set(repo.mediaKey(VideoSubNamespace, chatId, key), []byte(video))
}

func (repo *ChatMediaRepo) SetForeignChatVideoWithTTL(chatId, key string, video domain.Base64Video) error {
	if repo == nil {
		return ErrChatMediaRepoNotInit
	}
	if chatId == "" || key == "" {
		return local_store.DBError("empty chat id or key for chat video set")
	}
	if len(video) == 0 {
		return local_store.DBError("no data for chat video set")
	}

	return repo.db.SetWithTTL(repo.mediaKey(VideoSubNamespace, chatId, key), []byte(video), chatMediaTTL)
}

const chatMediaTTL = time.Hour * 24 * 7

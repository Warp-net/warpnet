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

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/security"
)

const (
	MediaRepoName     = "/MEDIA"
	ImageSubNamespace = "IMAGES"
	VideoSubNamespace = "VIDEOS"
)

var (
	ErrMediaNotFound    = local_store.DBError("media not found")
	ErrMediaRepoNotInit = local_store.DBError("media repo is not initialized")
)

type MediaStorer interface {
	Set(key local_store.DatabaseKey, value []byte) error
	Get(key local_store.DatabaseKey) ([]byte, error)
	SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error
}

type MediaRepo struct {
	db MediaStorer
}

type (
	Base64Image string
	ImageKey    string
)

func NewMediaRepo(db MediaStorer) *MediaRepo {
	return &MediaRepo{db: db}
}

func (repo *MediaRepo) GetImage(userId, key string) (Base64Image, error) {
	if repo == nil {
		return "", ErrMediaRepoNotInit
	}
	if key == "" || userId == "" {
		return "", ErrMediaNotFound
	}

	mediaKey := local_store.NewPrefixBuilder(MediaRepoName).
		AddRootID(ImageSubNamespace).
		AddParentId(userId).
		AddId(key).
		Build()

	data, err := repo.db.Get(mediaKey)
	if local_store.IsNotFoundError(err) {
		return "", ErrMediaNotFound
	}

	return Base64Image(data), err
}

func (repo *MediaRepo) SetImage(userId string, img Base64Image) (_ ImageKey, err error) {
	if repo == nil {
		return "", ErrMediaRepoNotInit
	}
	if len(img) == 0 || len(userId) == 0 {
		return "", local_store.DBError("no data for image set")
	}
	h := security.ConvertToSHA256([]byte(img))
	key := hex.EncodeToString(h)

	mediaKey := local_store.NewPrefixBuilder(MediaRepoName).
		AddRootID(ImageSubNamespace).
		AddParentId(userId).
		AddId(key).
		Build()

	return ImageKey(key), repo.db.Set(mediaKey, []byte(img))
}

func (repo *MediaRepo) SetForeignImageWithTTL(userId, key string, img Base64Image) error {
	if repo == nil {
		return ErrMediaRepoNotInit
	}
	if len(img) == 0 || len(userId) == 0 {
		return local_store.DBError("no data for image set provided")
	}
	if key == "" {
		return local_store.DBError("no key for image set provided")
	}

	mediaKey := local_store.NewPrefixBuilder(MediaRepoName).
		AddRootID(ImageSubNamespace).
		AddParentId(userId).
		AddId(key).
		Build()

	week := time.Hour * 24 * 7
	return repo.db.SetWithTTL(mediaKey, []byte(img), week)
}

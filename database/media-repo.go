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
	"encoding/hex"
	"errors"
	"github.com/Warp-net/warpnet/database/storage"
	"github.com/Warp-net/warpnet/security"
	"time"
)

const (
	MediaRepoName     = "/MEDIA"
	ImageSubNamespace = "IMAGES"
	VideoSubNamespace = "VIDEOS"
)

var (
	ErrMediaNotFound    = errors.New("media not found")
	ErrMediaRepoNotInit = errors.New("media repo is not initialized")
)

type MediaStorer interface {
	Set(key storage.DatabaseKey, value []byte) error
	Get(key storage.DatabaseKey) ([]byte, error)
	SetWithTTL(key storage.DatabaseKey, value []byte, ttl time.Duration) error
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

	mediaKey := storage.NewPrefixBuilder(MediaRepoName).
		AddRootID(ImageSubNamespace).
		AddParentId(userId).
		AddId(key).
		Build()

	data, err := repo.db.Get(mediaKey)
	if errors.Is(err, storage.ErrKeyNotFound) {
		return "", ErrMediaNotFound
	}

	return Base64Image(data), err
}

func (repo *MediaRepo) SetImage(userId string, img Base64Image) (_ ImageKey, err error) {
	if repo == nil {
		return "", ErrMediaRepoNotInit
	}
	if len(img) == 0 || len(userId) == 0 {
		return "", errors.New("no data for image set")
	}
	h := security.ConvertToSHA256([]byte(img))
	key := hex.EncodeToString(h)

	mediaKey := storage.NewPrefixBuilder(MediaRepoName).
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
		return errors.New("no data for image set provided")
	}
	if key == "" {
		return errors.New("no key for image set provided")
	}

	mediaKey := storage.NewPrefixBuilder(MediaRepoName).
		AddRootID(ImageSubNamespace).
		AddParentId(userId).
		AddId(key).
		Build()

	week := time.Hour * 24 * 7
	return repo.db.SetWithTTL(mediaKey, []byte(img), week)
}

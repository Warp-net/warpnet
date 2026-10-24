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
	"github.com/Warp-net/warpnet/domain"
	"time"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
)

const (
	MediaRepoName          = "/MEDIA"
	ChatMediaRepoName      = "/CHATMEDIA"
	SponsoredMediaRepoName = "/SPONSOREDMEDIA"
	ImageSubNamespace      = "IMAGES"
	CopySubNamespace       = "COPIES"

	foreignMediaTTL = time.Hour * 24 * 7
)

var (
	ErrMediaNotFound    = local_store.DBError("media not found")
	ErrMediaRepoNotInit = local_store.DBError("media repo is not initialized")
)

type ImageStorer interface {
	Set(key local_store.DatabaseKey, value []byte) error
	Get(key local_store.DatabaseKey) ([]byte, error)
	SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error
	NewReadTxn() (local_store.WarpTransactioner, error)
}

type ImageRepo struct {
	db     ImageStorer
	prefix string
}

func NewImageRepo(db ImageStorer) *ImageRepo {
	return &ImageRepo{db: db, prefix: MediaRepoName}
}

func NewChatImageRepo(db ImageStorer) *ImageRepo {
	return &ImageRepo{db: db, prefix: ChatMediaRepoName}
}

func NewSponsoredImageRepo(db ImageStorer) *ImageRepo {
	return &ImageRepo{db: db, prefix: SponsoredMediaRepoName}
}

func (repo *ImageRepo) GetImage(userId, key string) (domain.Base64Image, error) {
	if repo == nil {
		return "", ErrMediaRepoNotInit
	}
	if key == "" || userId == "" {
		return "", ErrMediaNotFound
	}

	mediaKey := local_store.NewPrefixBuilder(repo.prefix).
		AddRootID(ImageSubNamespace).
		AddParentId(userId).
		AddId(key).
		Build()

	data, err := repo.db.Get(mediaKey)
	if local_store.IsNotFoundError(err) {
		return "", ErrMediaNotFound
	}

	return domain.Base64Image(data), err
}

func (repo *ImageRepo) SetImage(userId string, img domain.Base64Image) (_ domain.ImageKey, err error) {
	if repo == nil {
		return "", ErrMediaRepoNotInit
	}
	if len(img) == 0 || len(userId) == 0 {
		return "", local_store.DBError("no data for image set")
	}
	h := security.ConvertToSHA256([]byte(img))
	key := hex.EncodeToString(h)

	mediaKey := local_store.NewPrefixBuilder(repo.prefix).
		AddRootID(ImageSubNamespace).
		AddParentId(userId).
		AddId(key).
		Build()

	return domain.ImageKey(key), repo.db.Set(mediaKey, []byte(img))
}

func (repo *ImageRepo) SetForeignImageWithTTL(userId, key string, img domain.Base64Image) error {
	if repo == nil {
		return ErrMediaRepoNotInit
	}
	if len(img) == 0 || len(userId) == 0 {
		return local_store.DBError("no data for image set provided")
	}
	if key == "" {
		return local_store.DBError("no key for image set provided")
	}

	mediaKey := local_store.NewPrefixBuilder(repo.prefix).
		AddRootID(ImageSubNamespace).
		AddParentId(userId).
		AddId(key).
		Build()

	return repo.db.SetWithTTL(mediaKey, []byte(img), foreignMediaTTL)
}

func (repo *ImageRepo) GetCopy(userId, key string) (domain.MediaCopy, error) {
	if repo == nil {
		return domain.MediaCopy{}, ErrMediaRepoNotInit
	}
	return getCopy(repo.db, repo.prefix, userId, key)
}

func (repo *ImageRepo) SetCopy(userId, key string, c domain.MediaCopy) error {
	if repo == nil {
		return ErrMediaRepoNotInit
	}
	if userId == "" || key == "" || c.OriginalKey == "" || c.BuyerId == "" {
		return local_store.DBError("no data for copy set")
	}

	data, err := json.Marshal(c)
	if err != nil {
		return err
	}

	mediaKey := local_store.NewPrefixBuilder(repo.prefix).
		AddRootID(CopySubNamespace).
		AddParentId(userId).
		AddId(key).
		Build()

	return repo.db.Set(mediaKey, data)
}

func (repo *ImageRepo) ListCopies(userId, originalKey string) ([]domain.MediaCopy, error) {
	if repo == nil {
		return nil, ErrMediaRepoNotInit
	}
	if userId == "" || originalKey == "" {
		return nil, local_store.DBError("no data for copies list")
	}

	txn, err := repo.db.NewReadTxn()
	if err != nil {
		return nil, err
	}
	defer txn.Rollback()

	var keys []local_store.DatabaseKey
	prefix := local_store.NewPrefixBuilder(repo.prefix).
		AddRootID(CopySubNamespace).
		AddParentId(userId).
		Build() + local_store.Delimeter
	err = txn.IterateKeys(prefix, func(key string) error {
		keys = append(keys, local_store.DatabaseKey(key))
		return nil
	})
	if err != nil || len(keys) == 0 {
		return nil, err
	}

	items, err := txn.BatchGet(keys...)
	if err != nil {
		return nil, err
	}
	var copies []domain.MediaCopy
	for _, item := range items {
		var c domain.MediaCopy
		if err := json.Unmarshal(item.Value, &c); err != nil {
			return nil, err
		}
		if c.OriginalKey == originalKey {
			copies = append(copies, c)
		}
	}
	return copies, nil
}

type copyGetter interface {
	Get(key local_store.DatabaseKey) ([]byte, error)
}

func getCopy(db copyGetter, prefix, userId, key string) (domain.MediaCopy, error) {
	if key == "" || userId == "" {
		return domain.MediaCopy{}, ErrMediaNotFound
	}

	mediaKey := local_store.NewPrefixBuilder(prefix).
		AddRootID(CopySubNamespace).
		AddParentId(userId).
		AddId(key).
		Build()

	data, err := db.Get(mediaKey)
	if local_store.IsNotFoundError(err) {
		return domain.MediaCopy{}, ErrMediaNotFound
	}
	if err != nil {
		return domain.MediaCopy{}, err
	}

	var c domain.MediaCopy
	if err := json.Unmarshal(data, &c); err != nil {
		return domain.MediaCopy{}, err
	}
	return c, nil
}

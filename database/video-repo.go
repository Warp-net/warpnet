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
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
)

const (
	VideoSubNamespace    = "VIDEOS"
	ChunkSubNamespace    = "CHUNKS"
	ManifestSubNamespace = "MANIFEST"
)

type VideoStorer interface {
	Get(key local_store.DatabaseKey) ([]byte, error)
	SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error
	NewTxn() (local_store.WarpTransactioner, error)
}

type VideoRepo struct {
	db     VideoStorer
	prefix string
}

func NewVideoRepo(db VideoStorer) *VideoRepo {
	return &VideoRepo{db: db, prefix: MediaRepoName}
}

func NewChatVideoRepo(db VideoStorer) *VideoRepo {
	return &VideoRepo{db: db, prefix: ChatMediaRepoName}
}

func NewSponsoredVideoRepo(db VideoStorer) *VideoRepo {
	return &VideoRepo{db: db, prefix: SponsoredMediaRepoName}
}

func (repo *VideoRepo) SetVideo(userId string, video domain.Base64Video) (_ domain.VideoKey, err error) {
	if repo == nil {
		return "", ErrMediaRepoNotInit
	}
	if len(video) == 0 || len(userId) == 0 {
		return "", local_store.DBError("no data for video set")
	}

	manifest, raw, err := splitVideo(video)
	if err != nil {
		return "", err
	}
	key := manifest.Key()
	return domain.VideoKey(key), repo.storeVideo(userId, key, manifest, raw, local_store.PermanentTTL)
}

func (repo *VideoRepo) SetForeignVideoWithTTL(userId, key string, video domain.Base64Video) error {
	if repo == nil {
		return ErrMediaRepoNotInit
	}
	if len(video) == 0 || len(userId) == 0 {
		return local_store.DBError("no data for video set provided")
	}
	if key == "" {
		return local_store.DBError("no key for video set provided")
	}

	manifest, raw, err := splitVideo(video)
	if err != nil {
		return err
	}
	return repo.storeVideo(userId, key, manifest, raw, foreignMediaTTL)
}

func (repo *VideoRepo) GetVideo(userId, key string) (domain.Base64Video, error) {
	manifest, err := repo.GetManifest(userId, key)
	if err != nil {
		return "", err
	}

	prefix := "data:" + manifest.Mime + ";base64,"
	var file strings.Builder
	file.Grow(len(prefix) + base64.StdEncoding.EncodedLen(int(manifest.Size)))
	file.WriteString(prefix)

	encoder := base64.NewEncoder(base64.StdEncoding, &file)
	for idx := range manifest.Hashes {
		chunk, err := repo.GetChunk(userId, key, idx)
		if err != nil {
			return "", err
		}
		_, _ = encoder.Write(chunk)
	}
	_ = encoder.Close()
	return domain.Base64Video(file.String()), nil
}

func (repo *VideoRepo) GetManifest(userId, key string) (domain.VideoManifest, error) {
	if repo == nil {
		return domain.VideoManifest{}, ErrMediaRepoNotInit
	}
	if key == "" || userId == "" {
		return domain.VideoManifest{}, ErrMediaNotFound
	}

	data, err := repo.db.Get(repo.manifestKey(userId, key))
	if local_store.IsNotFoundError(err) {
		return domain.VideoManifest{}, ErrMediaNotFound
	}
	if err != nil {
		return domain.VideoManifest{}, err
	}

	var manifest domain.VideoManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return domain.VideoManifest{}, err
	}
	return manifest, nil
}

func (repo *VideoRepo) GetChunk(userId, key string, idx int) ([]byte, error) {
	if repo == nil {
		return nil, ErrMediaRepoNotInit
	}
	if key == "" || userId == "" || idx < 0 {
		return nil, ErrMediaNotFound
	}

	chunk, err := repo.db.Get(repo.chunkKey(userId, key, idx))
	if local_store.IsNotFoundError(err) {
		return nil, ErrMediaNotFound
	}
	return chunk, err
}

func (repo *VideoRepo) SetForeignManifestWithTTL(userId, key string, manifest domain.VideoManifest) error {
	if repo == nil {
		return ErrMediaRepoNotInit
	}
	if userId == "" || key == "" || len(manifest.Hashes) == 0 {
		return local_store.DBError("no data for video manifest set provided")
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return repo.db.SetWithTTL(repo.manifestKey(userId, key), data, foreignMediaTTL)
}

func (repo *VideoRepo) SetForeignChunkWithTTL(userId, key string, idx int, chunk []byte) error {
	if repo == nil {
		return ErrMediaRepoNotInit
	}
	if userId == "" || key == "" || idx < 0 || len(chunk) == 0 {
		return local_store.DBError("no data for video chunk set provided")
	}
	return repo.db.SetWithTTL(repo.chunkKey(userId, key, idx), chunk, foreignMediaTTL)
}

func (repo *VideoRepo) GetCopy(userId, key string) (domain.MediaCopy, error) {
	if repo == nil {
		return domain.MediaCopy{}, ErrMediaRepoNotInit
	}
	return getCopy(repo.db, repo.prefix, userId, key)
}

func (repo *VideoRepo) storeVideo(
	userId, key string, manifest domain.VideoManifest, raw []byte, ttl time.Duration,
) error {
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}

	txn, err := repo.db.NewTxn()
	if err != nil {
		return err
	}
	defer txn.Rollback()

	for idx := range manifest.Hashes {
		start := int64(idx) * manifest.ChunkSize
		chunk := raw[start : start+manifest.ChunkLen(idx)]
		if err := txn.SetWithTTL(repo.chunkKey(userId, key, idx), chunk, ttl); err != nil {
			return err
		}
	}
	if err := txn.SetWithTTL(repo.manifestKey(userId, key), data, ttl); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *VideoRepo) manifestKey(userId, key string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(repo.prefix).
		AddSubPrefix(VideoSubNamespace).
		AddRootID(ManifestSubNamespace).
		AddParentId(userId).
		AddId(key).
		Build()
}

func (repo *VideoRepo) chunkKey(userId, key string, idx int) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(repo.prefix).
		AddSubPrefix(VideoSubNamespace).
		AddRootID(ChunkSubNamespace).
		AddParentId(userId).
		AddId(key).
		AddId(strconv.Itoa(idx)).
		Build()
}

func splitVideo(video domain.Base64Video) (domain.VideoManifest, []byte, error) {
	header, raw, err := media_meta.SplitDataURI(string(video))
	if err != nil {
		return domain.VideoManifest{}, nil, err
	}
	if len(raw) == 0 {
		return domain.VideoManifest{}, nil, local_store.DBError("no data for video set")
	}

	mime, _, _ := strings.Cut(strings.TrimPrefix(header, "data:"), ";")
	return media_meta.BuildVideoManifest(raw, mime), raw, nil
}

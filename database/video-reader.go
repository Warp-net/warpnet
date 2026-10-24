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
	"bufio"
	"errors"
	"io"
	"time"

	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	log "github.com/sirupsen/logrus"
)

const chunkReadTimeout = 30 * time.Second

type VideoFetcher interface {
	FetchVideoChunks(author domain.User, key string, offset int64) (warpnet.WarpStream, error)
	FetchVideo(author domain.User, key string) (domain.Base64Video, error)
}

type VideoReader struct {
	repo         *VideoRepo
	fetcher      VideoFetcher
	author       domain.User
	key          string
	offset       int64
	manifest     domain.VideoManifest
	remote       warpnet.WarpStream
	remoteReader *bufio.Reader
	nextChunk    int
}

func (repo *VideoRepo) OpenForeignVideo(
	fetcher VideoFetcher, author domain.User, key string, offset int64,
) (*VideoReader, error) {
	if repo == nil {
		return nil, ErrMediaRepoNotInit
	}
	r := &VideoReader{repo: repo, fetcher: fetcher, author: author, key: key, offset: max(offset, 0)}

	manifest, err := repo.GetManifest(author.Id, key)
	if err == nil {
		r.manifest = manifest
		return r, nil
	}
	if err := r.openRemote(r.offset); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *VideoReader) WriteVideo(s warpnet.WarpStream) error {
	if err := media_meta.WriteVideoManifest(s, r.manifest); err != nil {
		return err
	}
	if r.offset >= r.manifest.Size {
		return nil
	}

	for idx := int(r.offset / r.manifest.ChunkSize); idx < len(r.manifest.Hashes); idx++ {
		chunk, err := r.readChunk(idx)
		if err != nil {
			return err
		}
		if err := writeWithDeadline(s, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (r *VideoReader) Close() {
	if r.remote == nil {
		return
	}
	_ = r.remote.Reset()
	r.remote, r.remoteReader = nil, nil
}

func (r *VideoReader) readChunk(idx int) ([]byte, error) {
	if r.remote != nil && r.nextChunk == idx {
		return r.readRemoteChunk(idx)
	}

	chunk, err := r.repo.getChunk(r.author.Id, r.key, idx)
	if err == nil {
		return chunk, nil
	}
	if err := r.openRemote(int64(idx) * r.manifest.ChunkSize); err != nil {
		return nil, err
	}
	if r.remote == nil {
		return r.repo.getChunk(r.author.Id, r.key, idx)
	}
	return r.readRemoteChunk(idx)
}

func (r *VideoReader) openRemote(offset int64) error {
	r.Close()

	s, err := r.fetcher.FetchVideoChunks(r.author, r.key, offset)
	if errors.Is(err, warpnet.ErrProtocolNotSupported) {
		return r.fetchVideo()
	}
	if err != nil {
		return err
	}

	_ = s.SetReadDeadline(time.Now().Add(chunkReadTimeout))
	data := bufio.NewReaderSize(s, media_meta.MaxManifestSize)
	manifest, err := media_meta.ReadVideoManifest(data)
	if errors.Is(err, warpnet.ErrProtocolNotSupported) {
		_ = s.Reset()
		return r.fetchVideo()
	}
	if err != nil {
		_ = s.Reset()
		return err
	}

	if len(r.manifest.Hashes) == 0 {
		if err := media_meta.VerifyManifest(r.author, r.key, manifest); err != nil {
			_ = s.Reset()
			return err
		}
		r.manifest = manifest
		if err := r.repo.setForeignManifest(r.author.Id, r.key, manifest); err != nil {
			log.Errorf("video: storing manifest of %s: %v", r.key, err)
		}
	}
	r.remote, r.remoteReader, r.nextChunk = s, data, int(offset/r.manifest.ChunkSize)
	return nil
}

func (r *VideoReader) readRemoteChunk(idx int) ([]byte, error) {
	chunk := make([]byte, r.manifest.ChunkLen(idx))
	_ = r.remote.SetReadDeadline(time.Now().Add(chunkReadTimeout))
	if _, err := io.ReadFull(r.remoteReader, chunk); err != nil {
		r.Close()
		return nil, err
	}
	if err := media_meta.VerifyChunk(r.manifest, idx, chunk); err != nil {
		r.Close()
		return nil, err
	}

	r.nextChunk++
	if err := r.repo.setForeignChunk(r.author.Id, r.key, idx, chunk); err != nil {
		log.Errorf("video: storing chunk %d of %s: %v", idx, r.key, err)
	}
	return chunk, nil
}

func (r *VideoReader) fetchVideo() error {
	video, err := r.fetcher.FetchVideo(r.author, r.key)
	if err != nil {
		return err
	}
	if video == "" {
		return media_meta.ErrVideoUnavailable
	}
	if err := media_meta.VerifyForeignVideo(r.author, r.key, string(video)); err != nil {
		return err
	}
	if err := r.repo.SetForeignVideoWithTTL(r.author.Id, r.key, video); err != nil {
		return err
	}

	manifest, err := r.repo.GetManifest(r.author.Id, r.key)
	if err != nil {
		return err
	}
	r.manifest = manifest
	return nil
}

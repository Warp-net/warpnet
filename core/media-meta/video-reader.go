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

package media_meta

import (
	"bufio"
	"errors"
	"io"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const chunkReadTimeout = 30 * time.Second

type VideoStreamer interface {
	OpenStream(nodeId string, path stream.WarpRoute, data any) (warpnet.WarpStream, error)
	GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error)
}

type VideoCache interface {
	GetManifest(userId, key string) (domain.VideoManifest, error)
	GetChunk(userId, key string, idx int) ([]byte, error)
	SetForeignManifestWithTTL(userId, key string, manifest domain.VideoManifest) error
	SetForeignChunkWithTTL(userId, key string, idx int, chunk []byte) error
	SetForeignVideoWithTTL(userId, key string, video domain.Base64Video) error
}

type VideoReader struct {
	Streamer  VideoStreamer
	Cache     VideoCache
	Author    domain.User
	Key       string
	RangePath stream.WarpRoute
	FilePath  stream.WarpRoute
}

func (r VideoReader) ReadRange(offset, length int64) (domain.VideoManifest, []byte, error) {
	manifest, err := r.Cache.GetManifest(r.Author.Id, r.Key)
	if err != nil && r.Streamer == nil {
		return domain.VideoManifest{}, nil, err
	}
	if err != nil {
		return r.readRemote(domain.VideoManifest{}, offset, length)
	}

	end := min(offset+length, manifest.Size)
	if offset < 0 || offset >= end {
		return manifest, nil, nil
	}

	first := offset / manifest.ChunkSize
	var chunks []byte
	for idx := int(first); idx <= int((end-1)/manifest.ChunkSize); idx++ {
		chunk, err := r.Cache.GetChunk(r.Author.Id, r.Key, idx)
		if err != nil && r.Streamer != nil {
			chunkStart := int64(idx) * manifest.ChunkSize
			_, rest, err := r.readRemote(manifest, chunkStart, end-chunkStart)
			if err != nil {
				return manifest, nil, err
			}
			chunks = append(chunks, rest...)
			break
		}
		if err != nil {
			return manifest, nil, err
		}
		chunks = append(chunks, chunk...)
	}

	start := offset - first*manifest.ChunkSize
	return manifest, chunks[start : start+end-offset], nil
}

func (r VideoReader) readRemote(known domain.VideoManifest, offset, length int64) (domain.VideoManifest, []byte, error) {
	s, err := r.Streamer.OpenStream(r.Author.NodeId, r.RangePath, event.GetVideoRangeEvent{
		UserId: r.Author.Id,
		Key:    r.Key,
		Offset: offset,
		Length: length,
	})
	if errors.Is(err, warpnet.ErrProtocolNotSupported) {
		return r.readFile(offset, length)
	}
	if err != nil {
		return known, nil, err
	}

	_ = s.SetReadDeadline(time.Now().Add(chunkReadTimeout))
	data := bufio.NewReaderSize(s, MaxManifestSize)
	header, err := ReadVideoManifest(data)
	if errors.Is(err, warpnet.ErrProtocolNotSupported) {
		_ = s.Reset()
		return r.readFile(offset, length)
	}
	if err != nil {
		_ = s.Reset()
		return known, nil, err
	}

	manifest := known
	if len(manifest.Hashes) == 0 {
		if err := VerifyManifest(r.Author, r.Key, header); err != nil {
			_ = s.Reset()
			return known, nil, err
		}
		manifest = header
		if err := r.Cache.SetForeignManifestWithTTL(r.Author.Id, r.Key, manifest); err != nil {
			log.Errorf("media meta: storing manifest of %s: %v", r.Key, err)
		}
	}

	chunks, err := r.readChunks(data, s, manifest, offset, length)
	if err != nil {
		_ = s.Reset()
		return manifest, nil, err
	}
	_ = s.Close()
	return manifest, chunks, nil
}

func (r VideoReader) readChunks(
	data io.Reader, s warpnet.WarpStream, manifest domain.VideoManifest, offset, length int64,
) ([]byte, error) {
	end := min(offset+length, manifest.Size)
	if offset < 0 || offset >= end {
		return nil, nil
	}

	first := offset / manifest.ChunkSize
	var chunks []byte
	for idx := int(first); idx <= int((end-1)/manifest.ChunkSize); idx++ {
		chunk := make([]byte, manifest.ChunkLen(idx))
		_ = s.SetReadDeadline(time.Now().Add(chunkReadTimeout))
		if _, err := io.ReadFull(data, chunk); err != nil {
			return nil, err
		}
		if err := VerifyChunk(manifest, idx, chunk); err != nil {
			return nil, err
		}
		if err := r.Cache.SetForeignChunkWithTTL(r.Author.Id, r.Key, idx, chunk); err != nil {
			log.Errorf("media meta: storing chunk %d of %s: %v", idx, r.Key, err)
		}
		chunks = append(chunks, chunk...)
	}

	start := offset - first*manifest.ChunkSize
	return chunks[start : start+end-offset], nil
}

func (r VideoReader) readFile(offset, length int64) (domain.VideoManifest, []byte, error) {
	resp, err := r.Streamer.GenericStream(r.Author.NodeId, r.FilePath, event.GetVideoEvent{UserId: r.Author.Id, Key: r.Key})
	if err != nil {
		return domain.VideoManifest{}, nil, err
	}

	var videoResp event.GetVideoResponse
	if err := json.Unmarshal(resp, &videoResp); err != nil {
		return domain.VideoManifest{}, nil, err
	}
	if videoResp.File == "" {
		return domain.VideoManifest{}, nil, ErrVideoUnavailable
	}
	if err := VerifyForeignVideo(r.Author, r.Key, videoResp.File); err != nil {
		return domain.VideoManifest{}, nil, err
	}
	if err := r.Cache.SetForeignVideoWithTTL(r.Author.Id, r.Key, domain.Base64Video(videoResp.File)); err != nil {
		return domain.VideoManifest{}, nil, err
	}

	cached := r
	cached.Streamer = nil
	return cached.ReadRange(offset, length)
}

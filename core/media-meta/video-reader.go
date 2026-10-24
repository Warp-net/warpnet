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

type VideoSource struct {
	VideoReader

	manifest domain.VideoManifest
	offset   int64
	remote   warpnet.WarpStream
	data     *bufio.Reader
	next     int
}

func (r VideoReader) Open(offset int64) (*VideoSource, error) {
	video := &VideoSource{VideoReader: r, offset: max(offset, 0)}

	manifest, err := r.Cache.GetManifest(r.Author.Id, r.Key)
	if err != nil && r.Streamer != nil {
		manifest, err = video.openRemote(video.offset)
	}
	if err != nil {
		video.Close()
		return nil, err
	}
	video.manifest = manifest
	return video, nil
}

func (v *VideoSource) WriteTo(s warpnet.WarpStream) error {
	header, err := json.Marshal(v.manifest)
	if err != nil {
		return err
	}
	if err := writeWithDeadline(s, append(header, '\n')); err != nil {
		return err
	}

	for idx := int(v.offset / v.manifest.ChunkSize); idx < len(v.manifest.Hashes); idx++ {
		chunk, err := v.loadChunk(idx)
		if err != nil {
			return err
		}
		if err := writeWithDeadline(s, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (v *VideoSource) Close() {
	if v.remote == nil {
		return
	}
	_ = v.remote.Reset()
	v.remote, v.data = nil, nil
}

func (v *VideoSource) loadChunk(idx int) ([]byte, error) {
	if v.remote != nil && v.next == idx {
		return v.readRemoteChunk(idx)
	}

	chunk, err := v.Cache.GetChunk(v.Author.Id, v.Key, idx)
	if err == nil || v.Streamer == nil {
		return chunk, err
	}
	if _, err := v.openRemote(int64(idx) * v.manifest.ChunkSize); err != nil {
		return nil, err
	}
	if v.remote == nil {
		return v.Cache.GetChunk(v.Author.Id, v.Key, idx)
	}
	return v.readRemoteChunk(idx)
}

func (v *VideoSource) openRemote(offset int64) (domain.VideoManifest, error) {
	v.Close()

	s, err := v.Streamer.OpenStream(v.Author.NodeId, v.RangePath, event.GetVideoRangeEvent{
		UserId: v.Author.Id,
		Key:    v.Key,
		Offset: offset,
	})
	if errors.Is(err, warpnet.ErrProtocolNotSupported) {
		return v.fetchFile()
	}
	if err != nil {
		return v.manifest, err
	}

	_ = s.SetReadDeadline(time.Now().Add(chunkReadTimeout))
	data := bufio.NewReaderSize(s, MaxManifestSize)
	header, err := ReadVideoManifest(data)
	if errors.Is(err, warpnet.ErrProtocolNotSupported) {
		_ = s.Reset()
		return v.fetchFile()
	}
	if err != nil {
		_ = s.Reset()
		return v.manifest, err
	}

	manifest := v.manifest
	if len(manifest.Hashes) == 0 {
		if err := VerifyManifest(v.Author, v.Key, header); err != nil {
			_ = s.Reset()
			return v.manifest, err
		}
		manifest = header
		if err := v.Cache.SetForeignManifestWithTTL(v.Author.Id, v.Key, manifest); err != nil {
			log.Errorf("media meta: storing manifest of %s: %v", v.Key, err)
		}
	}

	v.manifest = manifest
	v.remote, v.data, v.next = s, data, int(offset/manifest.ChunkSize)
	return manifest, nil
}

func (v *VideoSource) readRemoteChunk(idx int) ([]byte, error) {
	chunk := make([]byte, v.manifest.ChunkLen(idx))
	_ = v.remote.SetReadDeadline(time.Now().Add(chunkReadTimeout))
	if _, err := io.ReadFull(v.data, chunk); err != nil {
		v.Close()
		return nil, err
	}
	if err := VerifyChunk(v.manifest, idx, chunk); err != nil {
		v.Close()
		return nil, err
	}

	v.next++
	if err := v.Cache.SetForeignChunkWithTTL(v.Author.Id, v.Key, idx, chunk); err != nil {
		log.Errorf("media meta: storing chunk %d of %s: %v", idx, v.Key, err)
	}
	return chunk, nil
}

func (v *VideoSource) fetchFile() (domain.VideoManifest, error) {
	resp, err := v.Streamer.GenericStream(v.Author.NodeId, v.FilePath, event.GetVideoEvent{UserId: v.Author.Id, Key: v.Key})
	if err != nil {
		return v.manifest, err
	}

	var videoResp event.GetVideoResponse
	if err := json.Unmarshal(resp, &videoResp); err != nil {
		return v.manifest, err
	}
	if videoResp.File == "" {
		return v.manifest, ErrVideoUnavailable
	}
	if err := VerifyForeignVideo(v.Author, v.Key, videoResp.File); err != nil {
		return v.manifest, err
	}
	if err := v.Cache.SetForeignVideoWithTTL(v.Author.Id, v.Key, domain.Base64Video(videoResp.File)); err != nil {
		return v.manifest, err
	}

	manifest, err := v.Cache.GetManifest(v.Author.Id, v.Key)
	if err != nil {
		return v.manifest, err
	}
	v.manifest = manifest
	return manifest, nil
}

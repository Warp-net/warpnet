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
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
)

const (
	VideoChunkSize  = 1 << 20
	MaxManifestSize = 64 << 10

	manifestSignDomain = "warpnet/video-manifest/v0"
	chunkWriteTimeout  = 30 * time.Second

	ErrForgedManifest   warpnet.WarpError = "video manifest does not match the key or its signer"
	ErrForgedChunk      warpnet.WarpError = "video chunk does not match its manifest"
	ErrVideoUnavailable warpnet.WarpError = "video is not available"
)

type VideoChunkFetcher interface {
	GetChunk(userId, key string, idx int) ([]byte, error)
}

func BuildVideoManifest(video []byte, mime string) domain.VideoManifest {
	manifest := domain.VideoManifest{Size: int64(len(video)), Mime: mime, ChunkSize: VideoChunkSize}
	for start := 0; start < len(video); start += VideoChunkSize {
		end := min(start+VideoChunkSize, len(video))
		manifest.Hashes = append(manifest.Hashes, security.ConvertToSHA256(video[start:end]))
	}
	return manifest
}

func (m Metadata) SignManifest(key string, manifest domain.VideoManifest) (domain.VideoManifest, error) {
	if err := m.validate(); err != nil {
		return manifest, err
	}
	manifest.NodeId, manifest.OwnerId = m.NodeId, m.OwnerId
	manifest.Signature = security.Sign(m.PrivKey, manifestSigningBytes(key, m.NodeId, m.OwnerId))
	return manifest, nil
}

func VerifyManifest(u domain.User, key string, manifest domain.VideoManifest) error {
	if !isWholeManifest(manifest) || manifest.Key() != key {
		return ErrForgedManifest
	}
	if manifest.NodeId == "" || manifest.NodeId != u.NodeId || manifest.OwnerId != u.Id {
		return ErrForgedManifest
	}

	pubKey := warpnet.FromIDToPubKey(warpnet.FromStringToPeerID(manifest.NodeId))
	if len(pubKey) == 0 {
		return ErrForgedManifest
	}
	body := manifestSigningBytes(key, manifest.NodeId, manifest.OwnerId)
	if err := security.VerifySignature(pubKey, body, manifest.Signature); err != nil {
		return ErrForgedManifest
	}
	return nil
}

func VerifyChunk(manifest domain.VideoManifest, idx int, chunk []byte) error {
	if idx < 0 || idx >= len(manifest.Hashes) || int64(len(chunk)) != manifest.ChunkLen(idx) {
		return ErrForgedChunk
	}
	if !bytes.Equal(security.ConvertToSHA256(chunk), manifest.Hashes[idx]) {
		return ErrForgedChunk
	}
	return nil
}

func WriteVideoRange(
	s warpnet.WarpStream,
	chunks VideoChunkFetcher,
	userId, key string,
	manifest domain.VideoManifest,
	offset, length int64,
) error {
	header, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := writeWithDeadline(s, append(header, '\n')); err != nil {
		return err
	}
	if offset < 0 || offset >= manifest.Size || manifest.ChunkSize <= 0 {
		return nil
	}

	first := int(offset / manifest.ChunkSize)
	last := len(manifest.Hashes) - 1
	if length > 0 {
		last = min(last, int((offset+length-1)/manifest.ChunkSize))
	}
	for idx := first; idx <= last; idx++ {
		chunk, err := chunks.GetChunk(userId, key, idx)
		if err != nil {
			return err
		}
		if err := writeWithDeadline(s, chunk); err != nil {
			return err
		}
	}
	return nil
}

func ReadVideoManifest(r *bufio.Reader) (domain.VideoManifest, error) {
	var manifest domain.VideoManifest

	line, err := r.ReadSlice('\n')
	if err != nil && (!errors.Is(err, io.EOF) || len(line) == 0) {
		return manifest, err
	}
	if err := json.Unmarshal(line, &manifest); err != nil {
		return manifest, err
	}
	if len(manifest.Hashes) == 0 {
		return manifest, ErrVideoUnavailable
	}
	return manifest, nil
}

func isWholeManifest(manifest domain.VideoManifest) bool {
	if manifest.ChunkSize != VideoChunkSize || manifest.Size <= 0 || !strings.HasPrefix(manifest.Mime, "video/") {
		return false
	}
	if int64(len(manifest.Hashes)) != (manifest.Size+manifest.ChunkSize-1)/manifest.ChunkSize {
		return false
	}
	for _, chunkHash := range manifest.Hashes {
		if len(chunkHash) != sha256.Size {
			return false
		}
	}
	return true
}

func manifestSigningBytes(key, nodeId, ownerId string) []byte {
	return []byte(strings.Join([]string{manifestSignDomain, key, nodeId, ownerId}, "\x00"))
}

func writeWithDeadline(s warpnet.WarpStream, data []byte) error {
	_ = s.SetWriteDeadline(time.Now().Add(chunkWriteTimeout))
	_, err := s.Write(data)
	return err
}

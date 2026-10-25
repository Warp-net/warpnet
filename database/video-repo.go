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
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
)

const (
	VideoSubNamespace    = "VIDEOS"
	chunkSubNamespace    = "CHUNKS"
	manifestSubNamespace = "MANIFEST"

	videoChunkSize   = 1 << 20
	maxManifestSize  = 64 << 10
	chunkReadTimeout = 30 * time.Second
)

var (
	ErrForgedVideo = local_store.DBError("video does not match its key")
	ErrInvalidSeek = local_store.DBError("invalid video offset")
)

type VideoStorer interface {
	Get(key local_store.DatabaseKey) ([]byte, error)
	SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error
	NewTxn() (local_store.WarpTransactioner, error)
}

type VideoFetcher interface {
	FetchVideoChunks(author domain.User, key string, offset int64) (warpnet.WarpStream, error)
	FetchVideo(author domain.User, key string) (domain.Base64Video, error)
}

type VideoRepo struct {
	db     VideoStorer
	prefix string
}

type VideoStream struct {
	repo     *VideoRepo
	fetcher  VideoFetcher
	author   domain.User
	key      string
	manifest domain.VideoManifest
	offset   int64

	chunk    []byte
	chunkIdx int

	remote       warpnet.WarpStream
	remoteReader *bufio.Reader
	nextChunk    int
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
	return domain.VideoKey(key), repo.setVideo(userId, key, manifest, raw, local_store.PermanentTTL)
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
	return repo.setVideo(userId, key, manifest, raw, foreignMediaTTL)
}

func (repo *VideoRepo) GetVideo(userId, key string) (domain.Base64Video, error) {
	video, err := repo.GetVideoStream(userId, key)
	if err != nil {
		return "", err
	}
	defer func() { _ = video.Close() }()

	prefix := "data:" + video.manifest.Mime + ";base64,"
	var file strings.Builder
	file.Grow(len(prefix) + base64.StdEncoding.EncodedLen(int(video.manifest.Size)))
	file.WriteString(prefix)

	encoder := base64.NewEncoder(base64.StdEncoding, &file)
	if _, err := video.WriteTo(encoder); err != nil {
		return "", err
	}
	_ = encoder.Close()
	return domain.Base64Video(file.String()), nil
}

func (repo *VideoRepo) GetVideoStream(userId, key string) (*VideoStream, error) {
	if repo == nil {
		return nil, ErrMediaRepoNotInit
	}
	manifest, err := repo.getManifest(userId, key)
	if err != nil {
		return nil, err
	}
	return &VideoStream{repo: repo, author: domain.User{Id: userId}, key: key, manifest: manifest, chunkIdx: -1}, nil
}

func (repo *VideoRepo) GetForeignVideoStream(
	fetcher VideoFetcher, author domain.User, key string,
) (*VideoStream, error) {
	if repo == nil {
		return nil, ErrMediaRepoNotInit
	}
	if key == "" || author.Id == "" {
		return nil, ErrMediaNotFound
	}
	return &VideoStream{repo: repo, fetcher: fetcher, author: author, key: key, chunkIdx: -1}, nil
}

func (repo *VideoRepo) GetCopy(userId, key string) (domain.MediaCopy, error) {
	if repo == nil {
		return domain.MediaCopy{}, ErrMediaRepoNotInit
	}
	return getCopy(repo.db, repo.prefix, userId, key)
}

func (v *VideoStream) Manifest() (domain.VideoManifest, error) {
	if len(v.manifest.Hashes) > 0 {
		return v.manifest, nil
	}
	manifest, err := v.repo.getManifest(v.author.Id, v.key)
	if err == nil {
		v.manifest = manifest
		return manifest, nil
	}
	if v.fetcher == nil {
		return domain.VideoManifest{}, err
	}
	if err := v.openRemote(int(v.offset / videoChunkSize)); err != nil {
		return domain.VideoManifest{}, err
	}
	return v.manifest, nil
}

func (v *VideoStream) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += v.offset
	case io.SeekEnd:
		manifest, err := v.Manifest()
		if err != nil {
			return v.offset, err
		}
		offset += manifest.Size
	default:
		return v.offset, ErrInvalidSeek
	}
	if offset < 0 {
		return v.offset, ErrInvalidSeek
	}
	v.offset = offset
	return offset, nil
}

func (v *VideoStream) Read(p []byte) (int, error) {
	data, err := v.peekChunk()
	if err != nil {
		return 0, err
	}
	n := copy(p, data)
	v.offset += int64(n)
	return n, nil
}

func (v *VideoStream) WriteTo(w io.Writer) (int64, error) {
	var written int64
	for {
		data, err := v.peekChunk()
		if errors.Is(err, io.EOF) {
			return written, nil
		}
		if err != nil {
			return written, err
		}

		n, err := w.Write(data)
		written += int64(n)
		v.offset += int64(n)
		if err != nil {
			return written, err
		}
	}
}

func (v *VideoStream) Close() error {
	v.closeRemote()
	v.chunk, v.chunkIdx = nil, -1
	return nil
}

func (v *VideoStream) peekChunk() ([]byte, error) {
	manifest, err := v.Manifest()
	if err != nil {
		return nil, err
	}
	if v.offset >= manifest.Size {
		return nil, io.EOF
	}

	idx := int(v.offset / manifest.ChunkSize)
	chunk, err := v.loadChunk(idx)
	if err != nil {
		return nil, err
	}
	return chunk[v.offset-int64(idx)*manifest.ChunkSize:], nil
}

func (v *VideoStream) loadChunk(idx int) ([]byte, error) {
	if idx == v.chunkIdx {
		return v.chunk, nil
	}
	chunk, err := v.readChunk(idx)
	if err != nil {
		return nil, err
	}
	v.chunk, v.chunkIdx = chunk, idx
	return chunk, nil
}

func (v *VideoStream) readChunk(idx int) ([]byte, error) {
	if v.remote != nil && v.nextChunk == idx {
		return v.readRemoteChunk(idx)
	}

	chunk, err := v.repo.getChunk(v.author.Id, v.key, idx)
	if err == nil || v.fetcher == nil {
		return chunk, err
	}
	if err := v.openRemote(idx); err != nil {
		return nil, err
	}
	if v.remote == nil {
		return v.repo.getChunk(v.author.Id, v.key, idx)
	}
	return v.readRemoteChunk(idx)
}

func (v *VideoStream) openRemote(idx int) error {
	v.closeRemote()

	s, err := v.fetcher.FetchVideoChunks(v.author, v.key, int64(idx)*videoChunkSize)
	if errors.Is(err, warpnet.ErrProtocolNotSupported) {
		return v.fetchVideo()
	}
	if err != nil {
		return err
	}

	_ = s.SetReadDeadline(time.Now().Add(chunkReadTimeout))
	data := bufio.NewReaderSize(s, maxManifestSize)
	manifest, err := readManifest(data)
	if errors.Is(err, warpnet.ErrProtocolNotSupported) {
		_ = s.Reset()
		return v.fetchVideo()
	}
	if err != nil {
		_ = s.Reset()
		return err
	}

	if len(v.manifest.Hashes) == 0 {
		if !isValidManifest(manifest, v.key) {
			_ = s.Reset()
			return ErrForgedVideo
		}
		v.manifest = manifest
		if err := v.repo.setForeignManifest(v.author.Id, v.key, manifest); err != nil {
			log.Errorf("video: storing manifest of %s: %v", v.key, err)
		}
	}
	v.remote, v.remoteReader, v.nextChunk = s, data, idx
	return nil
}

func (v *VideoStream) readRemoteChunk(idx int) ([]byte, error) {
	chunk := make([]byte, v.manifest.ChunkLen(idx))
	_ = v.remote.SetReadDeadline(time.Now().Add(chunkReadTimeout))
	if _, err := io.ReadFull(v.remoteReader, chunk); err != nil {
		v.closeRemote()
		return nil, err
	}
	if !bytes.Equal(security.ConvertToSHA256(chunk), v.manifest.Hashes[idx]) {
		v.closeRemote()
		return nil, ErrForgedVideo
	}

	v.nextChunk++
	if err := v.repo.setForeignChunk(v.author.Id, v.key, idx, chunk); err != nil {
		log.Errorf("video: storing chunk %d of %s: %v", idx, v.key, err)
	}
	return chunk, nil
}

func (v *VideoStream) fetchVideo() error {
	video, err := v.fetcher.FetchVideo(v.author, v.key)
	if err != nil {
		return err
	}
	if video == "" {
		return ErrMediaNotFound
	}
	if err := media_meta.VerifyForeignVideo(v.author, v.key, string(video)); err != nil {
		return err
	}
	if err := v.repo.SetForeignVideoWithTTL(v.author.Id, v.key, video); err != nil {
		return err
	}

	manifest, err := v.repo.getManifest(v.author.Id, v.key)
	if err != nil {
		return err
	}
	v.manifest = manifest
	return nil
}

func (v *VideoStream) closeRemote() {
	if v.remote == nil {
		return
	}
	_ = v.remote.Reset()
	v.remote, v.remoteReader = nil, nil
}

func (repo *VideoRepo) getManifest(userId, key string) (domain.VideoManifest, error) {
	if key == "" || userId == "" {
		return domain.VideoManifest{}, ErrMediaNotFound
	}

	data, err := repo.db.Get(repo.buildManifestKey(userId, key))
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

func (repo *VideoRepo) getChunk(userId, key string, idx int) ([]byte, error) {
	if key == "" || userId == "" || idx < 0 {
		return nil, ErrMediaNotFound
	}

	chunk, err := repo.db.Get(repo.buildChunkKey(userId, key, idx))
	if local_store.IsNotFoundError(err) {
		return nil, ErrMediaNotFound
	}
	return chunk, err
}

func (repo *VideoRepo) setForeignManifest(userId, key string, manifest domain.VideoManifest) error {
	if userId == "" || key == "" || len(manifest.Hashes) == 0 {
		return local_store.DBError("no data for video manifest set provided")
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return repo.db.SetWithTTL(repo.buildManifestKey(userId, key), data, foreignMediaTTL)
}

func (repo *VideoRepo) setForeignChunk(userId, key string, idx int, chunk []byte) error {
	if userId == "" || key == "" || idx < 0 || len(chunk) == 0 {
		return local_store.DBError("no data for video chunk set provided")
	}
	return repo.db.SetWithTTL(repo.buildChunkKey(userId, key, idx), chunk, foreignMediaTTL)
}

func (repo *VideoRepo) setVideo(
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
		if err := txn.SetWithTTL(repo.buildChunkKey(userId, key, idx), chunk, ttl); err != nil {
			return err
		}
	}
	if err := txn.SetWithTTL(repo.buildManifestKey(userId, key), data, ttl); err != nil {
		return err
	}
	return txn.Commit()
}

func (repo *VideoRepo) buildManifestKey(userId, key string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(repo.prefix).
		AddSubPrefix(VideoSubNamespace).
		AddRootID(manifestSubNamespace).
		AddParentId(userId).
		AddId(key).
		Build()
}

func (repo *VideoRepo) buildChunkKey(userId, key string, idx int) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(repo.prefix).
		AddSubPrefix(VideoSubNamespace).
		AddRootID(chunkSubNamespace).
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
	return buildManifest(raw, mime), raw, nil
}

func buildManifest(video []byte, mime string) domain.VideoManifest {
	manifest := domain.VideoManifest{Size: int64(len(video)), Mime: mime, ChunkSize: videoChunkSize}
	for start := 0; start < len(video); start += videoChunkSize {
		end := min(start+videoChunkSize, len(video))
		manifest.Hashes = append(manifest.Hashes, security.ConvertToSHA256(video[start:end]))
	}
	return manifest
}

func readManifest(r *bufio.Reader) (domain.VideoManifest, error) {
	var manifest domain.VideoManifest

	line, err := r.ReadSlice('\n')
	if err != nil && (!errors.Is(err, io.EOF) || len(line) == 0) {
		return manifest, err
	}
	if err := json.Unmarshal(line, &manifest); err != nil {
		return manifest, err
	}
	if len(manifest.Hashes) == 0 {
		return manifest, ErrMediaNotFound
	}
	return manifest, nil
}

func isValidManifest(manifest domain.VideoManifest, key string) bool {
	if manifest.ChunkSize != videoChunkSize || manifest.Size <= 0 || !strings.HasPrefix(manifest.Mime, "video/") {
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
	return manifest.Key() == key
}

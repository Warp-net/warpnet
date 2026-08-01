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

package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/docker/go-units"
	log "github.com/sirupsen/logrus"
)

const (
	maxVideoSize = units.MiB * 36

	ErrTooLargeVideo    warpnet.WarpError = "video is too large: 36 MiB is the maximum"
	ErrEmptyVideoKey    warpnet.WarpError = "empty video key"
	ErrNoVideoProvided  warpnet.WarpError = "a video must be provided"
	ErrUnsupportedVideo warpnet.WarpError = "unsupported video format: only MP4 and MOV (QuickTime) files are accepted"
)

const (
	boxHeaderSize = 8
	boxUUIDSize   = 16
)

var warpnetMetaUUID = [16]byte{
	0x77, 0x61, 0x72, 0x70, 0x6e, 0x65, 0x74, 0x00, // "warpnet\0"
	0x6d, 0x65, 0x74, 0x61, 0x00, 0x00, 0x00, 0x01, // "meta\0\0\0\1"
}

var acceptedVideoPrefixes = map[string]string{
	"video/mp4":       "data:video/mp4;base64,",
	"video/quicktime": "data:video/quicktime;base64,",
	"video/x-m4v":     "data:video/x-m4v;base64,",
}

type VideoStorer interface {
	GetVideo(userId, key string) (database.Base64Video, error)
	SetVideo(userId string, video database.Base64Video) (_ database.VideoKey, err error)
	SetForeignVideoWithTTL(userId, key string, video database.Base64Video) error
}

func StreamUploadVideoHandler(
	info MediaNodeInformer,
	mediaRepo VideoStorer,
	userRepo MediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UploadVideoEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, err
		}
		if ev.Video == "" {
			return nil, ErrNoVideoProvided
		}

		encryptedMeta, ownerUser, err := buildEncryptedMediaMeta(info, userRepo)
		if err != nil {
			return nil, err
		}

		key, err := processAndStoreVideo(ev.Video, encryptedMeta, ownerUser.Id, mediaRepo)
		if err != nil {
			return nil, fmt.Errorf("upload: video: %w", err)
		}

		return event.UploadVideoResponse{Key: key}, nil
	}
}

func processAndStoreVideo(
	file string,
	encryptedMeta []byte,
	userId string,
	mediaRepo VideoStorer,
) (string, error) {
	parts := strings.SplitN(file, ",", 2) //nolint:mnd
	if len(parts) != 2 {                  //nolint:mnd
		return "", ErrInvalidBase64Signature
	}

	prefix, ok := videoDataPrefix(parts[0])
	if !ok {
		return "", ErrUnsupportedVideo
	}

	videoBytes, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("base64 decoding: %w", err)
	}

	if len(videoBytes) > maxVideoSize {
		return "", ErrTooLargeVideo
	}

	if !isISOBaseMediaFile(videoBytes) {
		return "", ErrUnsupportedVideo
	}

	amendedVideo, err := amendVideoMetadata(videoBytes, encryptedMeta)
	if err != nil {
		return "", fmt.Errorf("meta data amending: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(amendedVideo)

	key, err := mediaRepo.SetVideo(userId, database.Base64Video(prefix+encoded))
	if err != nil {
		return "", fmt.Errorf("storing media: %w", err)
	}

	return string(key), nil
}

func StreamGetVideoHandler(
	streamer MediaStreamer,
	mediaRepo VideoStorer,
	userRepo MediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetVideoEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("get video: unmarshalling event: %w", err)
		}
		if ev.Key == "" {
			return nil, fmt.Errorf("get video: %w", ErrEmptyVideoKey)
		}

		ownNodeInfo := streamer.NodeInfo()
		ownerId := ownNodeInfo.OwnerId
		if ev.UserId == "" {
			ev.UserId = ownerId
		}

		isOwnVideoRequest := ownerId == ev.UserId

		if isOwnVideoRequest {
			video, err := mediaRepo.GetVideo(ev.UserId, ev.Key)
			if errors.Is(err, database.ErrMediaNotFound) || video == "" {
				log.Warnf("get video: key not found: %s", ev.Key)
				return event.GetVideoResponse{File: ""}, nil
			}
			if err != nil {
				return nil, fmt.Errorf("get video: fetching media: %w", err)
			}
			return newVideoResponse(video, ev.Deferred), nil
		}

		u, err := userRepo.Get(ev.UserId)
		if errors.Is(err, database.ErrUserNotFound) {
			video, _ := mediaRepo.GetVideo(ev.UserId, ev.Key)
			return newVideoResponse(video, ev.Deferred), nil
		}
		if err != nil {
			return nil, fmt.Errorf("get video: fetching user: %w", err)
		}

		isOwnAlias := ownNodeInfo.ID.String() == u.NodeId
		if isOwnAlias {
			return event.GetVideoResponse{File: ""}, nil
		}

		if cached, cErr := mediaRepo.GetVideo(ev.UserId, ev.Key); cErr == nil && cached != "" {
			return newVideoResponse(cached, ev.Deferred), nil
		}

		if ev.Deferred {
			return event.GetVideoResponse{File: "", Deferred: true}, nil
		}

		resp, err := streamer.GenericStream(u.NodeId, event.PUBLIC_GET_VIDEO, ev)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return event.GetVideoResponse{File: ""}, nil
		}
		if err != nil {
			return nil, err
		}

		var videoResp event.GetVideoResponse
		if err := json.Unmarshal(resp, &videoResp); err != nil {
			return nil, fmt.Errorf("get video: unmarshalling response: %w", err)
		}

		if videoResp.File != "" {
			if err := mediaRepo.SetForeignVideoWithTTL(
				u.Id, ev.Key, database.Base64Video(videoResp.File),
			); err != nil {
				log.Errorf("get video: storing foreign video: %v", err)
			}
		}

		return videoResp, nil
	}
}

func newVideoResponse(video database.Base64Video, deferred bool) event.GetVideoResponse {
	if deferred {
		return event.GetVideoResponse{
			File:     "",
			Size:     int64(len(video)),
			Deferred: true,
		}
	}
	return event.GetVideoResponse{File: string(video), Size: int64(len(video))}
}

func videoDataPrefix(header string) (string, bool) {
	header = strings.TrimPrefix(header, "data:")
	mime, params, _ := strings.Cut(header, ";")
	if params != "base64" {
		return "", false
	}
	prefix, ok := acceptedVideoPrefixes[strings.ToLower(strings.TrimSpace(mime))]
	return prefix, ok
}

func isISOBaseMediaFile(b []byte) bool {
	for offset := 0; offset+boxHeaderSize <= len(b); {
		size := int(binary.BigEndian.Uint32(b[offset : offset+4]))
		boxType := string(b[offset+4 : offset+boxHeaderSize])

		if boxType == "ftyp" {
			return true
		}
		if boxType != "wide" && boxType != "free" && boxType != "skip" {
			return false
		}
		if size < boxHeaderSize {
			return false
		}
		offset += size
	}
	return false
}

func amendVideoMetadata(videoBytes, metadata []byte) ([]byte, error) {
	encodedMetadata := base64.StdEncoding.EncodeToString(metadata)

	boxSize := boxHeaderSize + boxUUIDSize + len(encodedMetadata)
	if boxSize > math.MaxUint32 {
		return nil, warpnet.WarpError("amend video meta: metadata box too large")
	}

	buf := bytes.NewBuffer(make([]byte, 0, len(videoBytes)+boxSize))
	buf.Write(videoBytes)

	header := make([]byte, boxHeaderSize)
	binary.BigEndian.PutUint32(header, uint32(boxSize)) //nolint:gosec
	copy(header[4:], "uuid")

	buf.Write(header)
	buf.Write(warpnetMetaUUID[:])
	buf.WriteString(encodedMetadata)

	return buf.Bytes(), nil
}

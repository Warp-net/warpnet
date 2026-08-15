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
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
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

var acceptedVideoPrefixes = map[string]string{
	"video/mp4":       "data:video/mp4;base64,",
	"video/quicktime": "data:video/quicktime;base64,",
	"video/x-m4v":     "data:video/x-m4v;base64,",
}

type VideoStorer interface {
	GetVideo(userId, key string) (domain.Base64Video, error)
	SetVideo(userId string, video domain.Base64Video) (_ domain.VideoKey, err error)
	SetForeignVideoWithTTL(userId, key string, video domain.Base64Video) error
}

type VideoNodeInformer interface {
	NodeInfo() warpnet.NodeInfo
}

type VideoUserFetcher interface {
	Get(userId string) (user domain.User, err error)
}

type VideoStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

func StreamUploadVideoHandler(
	info VideoNodeInformer,
	privKey ed25519.PrivateKey,
	mediaRepo VideoStorer,
	userRepo VideoUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UploadVideoEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, err
		}
		if ev.Video == "" {
			return nil, ErrNoVideoProvided
		}

		nodeInfo := info.NodeInfo()

		owner, err := userRepo.Get(nodeInfo.OwnerId)
		if err != nil {
			return nil, fmt.Errorf("upload: video: fetching owner: %w", err)
		}

		watermark, err := buildWatermark(nodeInfo, privKey, owner)
		if err != nil {
			return nil, err
		}

		video, err := watermarkUploadedVideo(ev.Video, watermark)
		if err != nil {
			return nil, fmt.Errorf("upload: video: %w", err)
		}

		key, err := mediaRepo.SetVideo(watermark.OwnerId, video)
		if err != nil {
			return nil, fmt.Errorf("upload: video: storing media: %w", err)
		}

		return event.UploadVideoResponse{Key: string(key)}, nil
	}
}

func StreamGetVideoHandler(
	streamer VideoStreamer,
	mediaRepo VideoStorer,
	userRepo VideoUserFetcher,
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

		if stored, cErr := mediaRepo.GetVideo(ev.UserId, ev.Key); cErr == nil && stored != "" {
			return newVideoResponse(stored, ev.Deferred), nil
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

		if err := verifyForeignVideo(u, ev.Key, videoResp.File); err != nil {
			log.Warnf("get video: refused media of %s from node %s: %v", u.Id, u.NodeId, err)
			return event.GetVideoResponse{File: ""}, nil
		}

		if videoResp.File != "" {
			if err := mediaRepo.SetForeignVideoWithTTL(
				u.Id, ev.Key, domain.Base64Video(videoResp.File),
			); err != nil {
				log.Errorf("get video: storing foreign video: %v", err)
			}
		}

		return videoResp, nil
	}
}

func newVideoResponse(video domain.Base64Video, deferred bool) event.GetVideoResponse {
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

func verifyForeignVideo(u domain.User, key, file string) error {
	return verifyForeignMedia(u, key, file, media_meta.VerifyVideo)
}

func watermarkUploadedVideo(file string, watermark media_meta.Watermark) (domain.Base64Video, error) {
	header, videoBytes, err := splitDataURI(file)
	if err != nil {
		return "", err
	}

	prefix, ok := videoDataPrefix(header)
	if !ok {
		return "", ErrUnsupportedVideo
	}

	if len(videoBytes) > maxVideoSize {
		return "", ErrTooLargeVideo
	}

	if !media_meta.IsISOBaseMediaFile(videoBytes) {
		return "", ErrUnsupportedVideo
	}

	watermarked, err := watermarkRaw(videoBytes, watermark)
	if err != nil {
		return "", err
	}

	encoded := base64.StdEncoding.EncodeToString(watermarked)
	return domain.Base64Video(prefix + encoded), nil
}

func watermarkRaw(videoBytes []byte, watermark media_meta.Watermark) ([]byte, error) {
	raw, _, err := media_meta.SplitVideo(videoBytes)
	if err != nil {
		return nil, fmt.Errorf("meta data stripping: %w", err)
	}

	raw, err = media_meta.CloseOpenEndedBox(raw)
	if err != nil {
		return nil, fmt.Errorf("meta data stripping: %w", err)
	}

	watermarkBytes, err := watermark.Sign(security.ConvertToSHA256(raw))
	if err != nil {
		return nil, fmt.Errorf("meta data signing: %w", err)
	}

	watermarked, err := media_meta.EmbedInVideo(raw, watermarkBytes)
	if err != nil {
		return nil, fmt.Errorf("meta data amending: %w", err)
	}

	if err := media_meta.VerifyVideo(watermarked, watermark.NodeId, watermark.OwnerId); err != nil {
		return nil, fmt.Errorf("meta data self check: %w", err)
	}
	return watermarked, nil
}

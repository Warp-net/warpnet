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
	"errors"
	"fmt"

	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/docker/go-units"
	log "github.com/sirupsen/logrus"
)

const (
	maxChatVideoRangeLength = units.MiB * 4

	ErrEmptyChatImageKey warpnet.WarpError = "empty image key"
	ErrEmptyChatVideoKey warpnet.WarpError = "empty video key"
)

type ChatImageStorer interface {
	GetImage(userId, key string) (domain.Base64Image, error)
	SetForeignImageWithTTL(userId, key string, img domain.Base64Image) error
}

type ChatVideoStorer interface {
	GetVideo(userId, key string) (domain.Base64Video, error)
	SetForeignVideoWithTTL(userId, key string, video domain.Base64Video) error
}

type ChatVideoRangeStorer interface {
	GetManifest(userId, key string) (domain.VideoManifest, error)
	GetChunk(userId, key string, idx int) ([]byte, error)
}

type ChatVideoCacheStorer interface {
	GetManifest(userId, key string) (domain.VideoManifest, error)
	GetChunk(userId, key string, idx int) ([]byte, error)
	SetForeignManifestWithTTL(userId, key string, manifest domain.VideoManifest) error
	SetForeignChunkWithTTL(userId, key string, idx int, chunk []byte) error
	SetForeignVideoWithTTL(userId, key string, video domain.Base64Video) error
}

type ChatMediaNodeInformer interface {
	NodeInfo() warpnet.NodeInfo
}

type ChatMediaChatFetcher interface {
	IsParticipants(ownerId, otherUserId string) bool
}

type ChatMediaUserFetcher interface {
	Get(userId string) (user domain.User, err error)
	GetByNodeID(nodeID string) (user domain.User, err error)
}

type ChatMediaStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

type ChatVideoRangeStreamer interface {
	OpenStream(nodeId string, path stream.WarpRoute, data any) (warpnet.WarpStream, error)
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

func StreamGetChatImageHandler(
	streamer ChatMediaStreamer,
	mediaRepo ChatImageStorer,
	chatRepo ChatMediaChatFetcher,
	userRepo ChatMediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetImageEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("get chat image: unmarshalling event: %w", err)
		}
		if ev.Key == "" {
			return nil, fmt.Errorf("get chat image: %w", ErrEmptyChatImageKey)
		}

		ownNodeInfo := streamer.NodeInfo()
		ownerId := ownNodeInfo.OwnerId
		if ev.UserId == "" {
			ev.UserId = ownerId
		}

		if ev.UserId == ownerId {
			if !isChatMediaAllowed(s, chatRepo, userRepo, ownNodeInfo) {
				log.Warnf("get chat image: refused key %s", ev.Key)
				return event.GetImageResponse{File: ""}, nil
			}

			img, err := mediaRepo.GetImage(ownerId, ev.Key)
			if err != nil && !errors.Is(err, database.ErrMediaNotFound) {
				return nil, fmt.Errorf("get chat image: fetching media: %w", err)
			}
			return event.GetImageResponse{File: string(img)}, nil
		}

		isOwnRequest := warpnet.VerifyAuthorship(s, ownNodeInfo.ID.String()) == nil
		if !isOwnRequest {
			return event.GetImageResponse{File: ""}, nil
		}

		if stored, err := mediaRepo.GetImage(ev.UserId, ev.Key); err == nil && stored != "" {
			return event.GetImageResponse{File: string(stored)}, nil
		}

		u, err := userRepo.Get(ev.UserId)
		if err != nil {
			return nil, fmt.Errorf("get chat image: fetching user: %w", err)
		}
		if u.NodeId == "" || u.NodeId == ownNodeInfo.ID.String() {
			return event.GetImageResponse{File: ""}, nil
		}

		resp, err := streamer.GenericStream(u.NodeId, event.PUBLIC_GET_CHAT_IMAGE, ev)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return event.GetImageResponse{File: ""}, nil
		}
		if err != nil {
			return nil, err
		}

		var imgResp event.GetImageResponse
		if err := json.Unmarshal(resp, &imgResp); err != nil {
			return nil, fmt.Errorf("get chat image: unmarshalling response: %w", err)
		}

		if err := media_meta.VerifyForeignImage(u, ev.Key, imgResp.File); err != nil {
			log.Warnf("get chat image: refused media of %s from node %s: %v", u.Id, u.NodeId, err)
			return event.GetImageResponse{File: ""}, nil
		}

		if imgResp.File != "" {
			if err := mediaRepo.SetForeignImageWithTTL(
				u.Id, ev.Key, domain.Base64Image(imgResp.File),
			); err != nil {
				log.Errorf("get chat image: storing peer image: %v", err)
			}
		}

		return imgResp, nil
	}
}

func StreamGetChatVideoHandler(
	streamer ChatMediaStreamer,
	mediaRepo ChatVideoStorer,
	chatRepo ChatMediaChatFetcher,
	userRepo ChatMediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetVideoEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("get chat video: unmarshalling event: %w", err)
		}
		if ev.Key == "" {
			return nil, fmt.Errorf("get chat video: %w", ErrEmptyChatVideoKey)
		}

		ownNodeInfo := streamer.NodeInfo()
		ownerId := ownNodeInfo.OwnerId
		if ev.UserId == "" {
			ev.UserId = ownerId
		}

		if ev.UserId == ownerId {
			if !isChatMediaAllowed(s, chatRepo, userRepo, ownNodeInfo) {
				log.Warnf("get chat video: refused key %s", ev.Key)
				return event.GetVideoResponse{File: ""}, nil
			}

			video, err := mediaRepo.GetVideo(ownerId, ev.Key)
			if err != nil && !errors.Is(err, database.ErrMediaNotFound) {
				return nil, fmt.Errorf("get chat video: fetching media: %w", err)
			}
			return buildChatVideoResponse(video, ev.Deferred), nil
		}

		isOwnRequest := warpnet.VerifyAuthorship(s, ownNodeInfo.ID.String()) == nil
		if !isOwnRequest {
			return event.GetVideoResponse{File: ""}, nil
		}

		if stored, err := mediaRepo.GetVideo(ev.UserId, ev.Key); err == nil && stored != "" {
			return buildChatVideoResponse(stored, ev.Deferred), nil
		}

		u, err := userRepo.Get(ev.UserId)
		if err != nil {
			return nil, fmt.Errorf("get chat video: fetching user: %w", err)
		}
		if u.NodeId == "" || u.NodeId == ownNodeInfo.ID.String() {
			return event.GetVideoResponse{File: ""}, nil
		}
		if ev.Deferred {
			return event.GetVideoResponse{File: "", Deferred: true}, nil
		}

		resp, err := streamer.GenericStream(u.NodeId, event.PUBLIC_GET_CHAT_VIDEO, ev)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return event.GetVideoResponse{File: ""}, nil
		}
		if err != nil {
			return nil, err
		}

		var videoResp event.GetVideoResponse
		if err := json.Unmarshal(resp, &videoResp); err != nil {
			return nil, fmt.Errorf("get chat video: unmarshalling response: %w", err)
		}

		if err := media_meta.VerifyForeignVideo(u, ev.Key, videoResp.File); err != nil {
			log.Warnf("get chat video: refused media of %s from node %s: %v", u.Id, u.NodeId, err)
			return event.GetVideoResponse{File: ""}, nil
		}

		if videoResp.File != "" {
			if err := mediaRepo.SetForeignVideoWithTTL(
				u.Id, ev.Key, domain.Base64Video(videoResp.File),
			); err != nil {
				log.Errorf("get chat video: storing peer video: %v", err)
			}
		}

		return videoResp, nil
	}
}

func StreamGetChatVideoRangeHandler(
	info ChatMediaNodeInformer,
	privKey ed25519.PrivateKey,
	videoRepo ChatVideoRangeStorer,
	chatRepo ChatMediaChatFetcher,
	userRepo ChatMediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetVideoRangeEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("get chat video range: unmarshalling event: %w", err)
		}
		if ev.Key == "" {
			return nil, fmt.Errorf("get chat video range: %w", ErrEmptyChatVideoKey)
		}

		ownNodeInfo := info.NodeInfo()
		if ev.UserId == "" {
			ev.UserId = ownNodeInfo.OwnerId
		}
		if ev.UserId != ownNodeInfo.OwnerId {
			return domain.VideoManifest{}, nil
		}
		if !isChatMediaAllowed(s, chatRepo, userRepo, ownNodeInfo) {
			log.Warnf("get chat video range: refused key %s", ev.Key)
			return domain.VideoManifest{}, nil
		}

		manifest, err := videoRepo.GetManifest(ownNodeInfo.OwnerId, ev.Key)
		if errors.Is(err, database.ErrMediaNotFound) {
			return domain.VideoManifest{}, nil
		}
		if err != nil {
			return nil, fmt.Errorf("get chat video range: fetching manifest: %w", err)
		}

		signer := media_meta.Metadata{PrivKey: privKey, NodeId: ownNodeInfo.ID.String(), OwnerId: ownNodeInfo.OwnerId}
		manifest, err = signer.SignManifest(ev.Key, manifest)
		if err != nil {
			return nil, fmt.Errorf("get chat video range: signing manifest: %w", err)
		}

		err = media_meta.WriteVideoRange(s, videoRepo, ownNodeInfo.OwnerId, ev.Key, manifest, ev.Offset, ev.Length)
		if err != nil {
			log.Debugf("get chat video range: streaming %s stopped: %v", ev.Key, err)
			_ = s.Reset()
		}
		return []byte{}, nil
	}
}

func StreamPlayChatVideoHandler(
	streamer ChatVideoRangeStreamer,
	videoRepo ChatVideoCacheStorer,
	userRepo ChatMediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetVideoRangeEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("play chat video: unmarshalling event: %w", err)
		}
		if ev.Key == "" {
			return nil, fmt.Errorf("play chat video: %w", ErrEmptyChatVideoKey)
		}

		ownerId := streamer.NodeInfo().OwnerId
		if ev.UserId == "" {
			ev.UserId = ownerId
		}
		reader := media_meta.VideoReader{Cache: videoRepo, Author: domain.User{Id: ownerId}, Key: ev.Key}
		if ev.UserId != ownerId {
			u, err := userRepo.Get(ev.UserId)
			if errors.Is(err, database.ErrUserNotFound) {
				return event.GetVideoRangeResponse{}, nil
			}
			if err != nil {
				return nil, fmt.Errorf("play chat video: fetching user: %w", err)
			}
			reader = media_meta.VideoReader{
				Streamer:  streamer,
				Cache:     videoRepo,
				Author:    u,
				Key:       ev.Key,
				RangePath: event.PUBLIC_GET_CHAT_VIDEO_RANGE,
				FilePath:  event.PUBLIC_GET_CHAT_VIDEO,
			}
		}

		manifest, data, err := reader.ReadRange(ev.Offset, min(ev.Length, maxChatVideoRangeLength))
		if errors.Is(err, database.ErrMediaNotFound) || errors.Is(err, media_meta.ErrVideoUnavailable) ||
			errors.Is(err, warpnet.ErrNodeIsOffline) {
			return event.GetVideoRangeResponse{}, nil
		}
		if err != nil {
			log.Warnf("play chat video: refused video %s of %s: %v", ev.Key, ev.UserId, err)
			return event.GetVideoRangeResponse{}, nil
		}
		return event.GetVideoRangeResponse{Size: manifest.Size, Mime: manifest.Mime, Data: data}, nil
	}
}

func isChatMediaAllowed(
	s warpnet.WarpStream,
	chatRepo ChatMediaChatFetcher,
	userRepo ChatMediaUserFetcher,
	ownNodeInfo warpnet.NodeInfo,
) bool {
	if warpnet.VerifyAuthorship(s, ownNodeInfo.ID.String()) == nil {
		return true
	}
	if s == nil || s.Conn() == nil {
		return false
	}

	requester, err := userRepo.GetByNodeID(s.Conn().RemotePeer().String())
	if err != nil || requester.Id == "" {
		return false
	}
	return chatRepo.IsParticipants(ownNodeInfo.OwnerId, requester.Id)
}

func buildChatVideoResponse(video domain.Base64Video, deferred bool) event.GetVideoResponse {
	if deferred {
		return event.GetVideoResponse{
			File:     "",
			Size:     int64(len(video)),
			Deferred: true,
		}
	}
	return event.GetVideoResponse{File: string(video), Size: int64(len(video))}
}

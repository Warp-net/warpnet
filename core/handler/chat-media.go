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
	"errors"
	"fmt"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type ChatMediaStorer interface {
	GetImage(userId, key string) (domain.Base64Image, error)
	SetForeignImageWithTTL(userId, key string, img domain.Base64Image) error
	GetVideo(userId, key string) (domain.Base64Video, error)
	SetForeignVideoWithTTL(userId, key string, video domain.Base64Video) error
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

func StreamGetChatImageHandler(
	streamer ChatMediaStreamer,
	mediaRepo ChatMediaStorer,
	chatRepo ChatMediaChatFetcher,
	userRepo ChatMediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetImageEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("get chat image: unmarshalling event: %w", err)
		}
		if ev.Key == "" {
			return nil, fmt.Errorf("get chat image: %w", ErrEmptyImageKey)
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

		if !isOwnRequest(s, ownNodeInfo) {
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

		if err := verifyForeignImage(u, ev.Key, imgResp.File); err != nil {
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
	mediaRepo ChatMediaStorer,
	chatRepo ChatMediaChatFetcher,
	userRepo ChatMediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetVideoEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("get chat video: unmarshalling event: %w", err)
		}
		if ev.Key == "" {
			return nil, fmt.Errorf("get chat video: %w", ErrEmptyVideoKey)
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
			return newVideoResponse(video, ev.Deferred), nil
		}

		if !isOwnRequest(s, ownNodeInfo) {
			return event.GetVideoResponse{File: ""}, nil
		}

		if stored, err := mediaRepo.GetVideo(ev.UserId, ev.Key); err == nil && stored != "" {
			return newVideoResponse(stored, ev.Deferred), nil
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

		if err := verifyForeignVideo(u, ev.Key, videoResp.File); err != nil {
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

func isChatMediaAllowed(
	s warpnet.WarpStream,
	chatRepo ChatMediaChatFetcher,
	userRepo ChatMediaUserFetcher,
	ownNodeInfo warpnet.NodeInfo,
) bool {
	if isOwnRequest(s, ownNodeInfo) {
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

func isOwnRequest(s warpnet.WarpStream, ownNodeInfo warpnet.NodeInfo) bool {
	return warpnet.VerifyAuthorship(s, ownNodeInfo.ID.String()) == nil
}

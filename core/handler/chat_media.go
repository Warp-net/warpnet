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

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

/*

	Chat attachments never travel the public media routes. They are addressed by
	(chat id, content key) rather than by user id, stored in their own namespace,
	and served only to the two nodes that take part in the chat. A peer cannot
	name a blob without naming a conversation it belongs to, and the participant
	set is read from local state and checked against the peer the transport
	authenticated — never against anything the request claims about itself.

*/

const (
	ErrEmptyChatId       warpnet.WarpError = "chat id is empty"
	ErrForeignChat       warpnet.WarpError = "owner does not take part in this chat"
	ErrForeignChatMedia  warpnet.WarpError = "chat attachment is not shared with this peer"
	ErrNoChatMediaTarget warpnet.WarpError = "chat has no reachable counterpart"
)

// ChatMediaStorer is the media store scoped to /CHATMEDIA, where the root ID
// is a chat. It is the same storage as the public one, wired with the other
// prefix — see database.NewChatMediaRepo.
type ChatMediaStorer interface {
	GetImage(chatId, key string) (domain.Base64Image, error)
	SetImage(chatId string, img domain.Base64Image) (_ domain.ImageKey, err error)
	SetForeignImageWithTTL(chatId, key string, img domain.Base64Image) error
	GetVideo(chatId, key string) (domain.Base64Video, error)
	SetVideo(chatId string, video domain.Base64Video) (_ domain.VideoKey, err error)
	SetForeignVideoWithTTL(chatId, key string, video domain.Base64Video) error
}

type ChatMediaChatFetcher interface {
	GetChat(chatId string) (chat domain.Chat, err error)
}

type ChatMediaUserFetcher interface {
	Get(userId string) (user domain.User, err error)
}

type ChatMediaNodeInformer interface {
	NodeInfo() warpnet.NodeInfo
}

type ChatMediaStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

// authorizeChatMedia is the single gate every chat attachment passes through.
// It refuses a chat the owner is not in, and a peer that is neither the owner's
// own node (or a device paired to it) nor the node of the person on the other
// side of that chat. It returns that counterpart, since a fetch has nowhere
// else to go.
func authorizeChatMedia(
	s warpnet.WarpStream,
	chatRepo ChatMediaChatFetcher,
	userRepo ChatMediaUserFetcher,
	ownNodeInfo warpnet.NodeInfo,
	chatId string,
) (other domain.User, err error) {
	chat, err := chatRepo.GetChat(chatId)
	if err != nil {
		return other, err
	}

	ownerId := ownNodeInfo.OwnerId
	if chat.OwnerId != ownerId && chat.OtherUserId != ownerId {
		return other, ErrForeignChat
	}

	otherId := chat.OtherUserId
	if otherId == ownerId {
		otherId = chat.OwnerId
	}
	other, err = userRepo.Get(otherId)
	if err != nil && !errors.Is(err, database.ErrUserNotFound) {
		return other, err
	}

	if warpnet.VerifyAuthorship(s, ownNodeInfo.ID.String()) == nil {
		return other, nil
	}
	if other.NodeId != "" && warpnet.VerifyAuthorship(s, other.NodeId) == nil {
		return other, nil
	}
	return other, ErrForeignChatMedia
}

// isOwnRequest tells a call from the owner's own interface apart from one that
// arrived over the network. Only the former may make this node fetch from a
// peer; a remote peer gets what is already stored, or nothing.
func isOwnRequest(s warpnet.WarpStream, ownNodeInfo warpnet.NodeInfo) bool {
	return warpnet.VerifyAuthorship(s, ownNodeInfo.ID.String()) == nil
}

func StreamUploadChatImageHandler(
	info ChatMediaNodeInformer,
	privKey ed25519.PrivateKey,
	mediaRepo ChatMediaStorer,
	chatRepo ChatMediaChatFetcher,
	userRepo ChatMediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UploadChatImageEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("upload chat image: unmarshalling event: %w", err)
		}
		if ev.ChatId == "" {
			return nil, ErrEmptyChatId
		}

		images := [4]string{ev.Image1, ev.Image2, ev.Image3, ev.Image4}

		hasImages := false
		for _, img := range images {
			if img != "" {
				hasImages = true
				break
			}
		}
		if !hasImages {
			return nil, ErrNoImagesProvided
		}

		nodeInfo := info.NodeInfo()
		if _, err := authorizeChatMedia(s, chatRepo, userRepo, nodeInfo, string(ev.ChatId)); err != nil {
			log.Warnf("upload chat image: refused for chat %s: %v", ev.ChatId, err)
			return nil, ErrForeignChatMedia
		}

		owner, err := userRepo.Get(nodeInfo.OwnerId)
		if err != nil {
			return nil, fmt.Errorf("upload chat image: fetching owner: %w", err)
		}

		watermark, err := buildWatermark(nodeInfo, privKey, owner)
		if err != nil {
			return nil, err
		}

		var keys [4]string
		for i, file := range images {
			if file == "" {
				continue
			}

			img, err := watermarkUploadedImage(file, watermark)
			if err != nil {
				return nil, fmt.Errorf("upload chat image%d: %w", i+1, err)
			}

			key, err := mediaRepo.SetImage(string(ev.ChatId), img)
			if err != nil {
				return nil, fmt.Errorf("upload chat image%d: storing media: %w", i+1, err)
			}
			keys[i] = string(key)
		}

		return event.UploadImageResponse{
			Key1: keys[0],
			Key2: keys[1],
			Key3: keys[2],
			Key4: keys[3],
		}, nil
	}
}

func StreamGetChatImageHandler(
	streamer ChatMediaStreamer,
	mediaRepo ChatMediaStorer,
	chatRepo ChatMediaChatFetcher,
	userRepo ChatMediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetChatImageEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("get chat image: unmarshalling event: %w", err)
		}
		if ev.ChatId == "" {
			return nil, ErrEmptyChatId
		}
		if ev.Key == "" {
			return nil, fmt.Errorf("get chat image: %w", ErrEmptyImageKey)
		}

		ownNodeInfo := streamer.NodeInfo()
		other, err := authorizeChatMedia(s, chatRepo, userRepo, ownNodeInfo, string(ev.ChatId))
		if err != nil {
			log.Warnf("get chat image: refused key %s of chat %s: %v", ev.Key, ev.ChatId, err)
			return event.GetImageResponse{File: ""}, nil
		}

		img, err := mediaRepo.GetImage(string(ev.ChatId), ev.Key)
		if err != nil && !errors.Is(err, database.ErrMediaNotFound) {
			return nil, fmt.Errorf("get chat image: fetching media: %w", err)
		}
		if img != "" {
			return event.GetImageResponse{File: string(img)}, nil
		}

		if !isOwnRequest(s, ownNodeInfo) {
			return event.GetImageResponse{File: ""}, nil
		}
		if other.NodeId == "" || other.NodeId == ownNodeInfo.ID.String() {
			return event.GetImageResponse{File: ""}, nil
		}

		resp, err := streamer.GenericStream(other.NodeId, event.PUBLIC_GET_CHAT_IMAGE, ev)
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

		if err := verifyForeignImage(other, ev.Key, imgResp.File); err != nil {
			log.Warnf("get chat image: refused media of %s from node %s: %v", other.Id, other.NodeId, err)
			return event.GetImageResponse{File: ""}, nil
		}

		if imgResp.File != "" {
			if err := mediaRepo.SetForeignImageWithTTL(
				string(ev.ChatId), ev.Key, domain.Base64Image(imgResp.File),
			); err != nil {
				log.Errorf("get chat image: storing peer image: %v", err)
			}
		}

		return imgResp, nil
	}
}

func StreamUploadChatVideoHandler(
	info ChatMediaNodeInformer,
	privKey ed25519.PrivateKey,
	mediaRepo ChatMediaStorer,
	chatRepo ChatMediaChatFetcher,
	userRepo ChatMediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UploadChatVideoEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("upload chat video: unmarshalling event: %w", err)
		}
		if ev.ChatId == "" {
			return nil, ErrEmptyChatId
		}
		if ev.Video == "" {
			return nil, ErrNoVideoProvided
		}

		nodeInfo := info.NodeInfo()
		if _, err := authorizeChatMedia(s, chatRepo, userRepo, nodeInfo, string(ev.ChatId)); err != nil {
			log.Warnf("upload chat video: refused for chat %s: %v", ev.ChatId, err)
			return nil, ErrForeignChatMedia
		}

		owner, err := userRepo.Get(nodeInfo.OwnerId)
		if err != nil {
			return nil, fmt.Errorf("upload chat video: fetching owner: %w", err)
		}

		watermark, err := buildWatermark(nodeInfo, privKey, owner)
		if err != nil {
			return nil, err
		}

		video, err := watermarkUploadedVideo(ev.Video, watermark)
		if err != nil {
			return nil, fmt.Errorf("upload chat video: %w", err)
		}

		key, err := mediaRepo.SetVideo(string(ev.ChatId), video)
		if err != nil {
			return nil, fmt.Errorf("upload chat video: storing media: %w", err)
		}

		return event.UploadVideoResponse{Key: string(key)}, nil
	}
}

func StreamGetChatVideoHandler(
	streamer ChatMediaStreamer,
	mediaRepo ChatMediaStorer,
	chatRepo ChatMediaChatFetcher,
	userRepo ChatMediaUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(input []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetChatVideoEvent
		if err := json.Unmarshal(input, &ev); err != nil {
			return nil, fmt.Errorf("get chat video: unmarshalling event: %w", err)
		}
		if ev.ChatId == "" {
			return nil, ErrEmptyChatId
		}
		if ev.Key == "" {
			return nil, fmt.Errorf("get chat video: %w", ErrEmptyVideoKey)
		}

		ownNodeInfo := streamer.NodeInfo()
		other, err := authorizeChatMedia(s, chatRepo, userRepo, ownNodeInfo, string(ev.ChatId))
		if err != nil {
			log.Warnf("get chat video: refused key %s of chat %s: %v", ev.Key, ev.ChatId, err)
			return event.GetVideoResponse{File: ""}, nil
		}

		video, err := mediaRepo.GetVideo(string(ev.ChatId), ev.Key)
		if err != nil && !errors.Is(err, database.ErrMediaNotFound) {
			return nil, fmt.Errorf("get chat video: fetching media: %w", err)
		}
		if video != "" {
			return newVideoResponse(video, ev.Deferred), nil
		}

		if !isOwnRequest(s, ownNodeInfo) {
			return event.GetVideoResponse{File: ""}, nil
		}
		if other.NodeId == "" || other.NodeId == ownNodeInfo.ID.String() {
			return event.GetVideoResponse{File: ""}, nil
		}
		if ev.Deferred {
			return event.GetVideoResponse{File: "", Deferred: true}, nil
		}

		resp, err := streamer.GenericStream(other.NodeId, event.PUBLIC_GET_CHAT_VIDEO, ev)
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

		if err := verifyForeignVideo(other, ev.Key, videoResp.File); err != nil {
			log.Warnf("get chat video: refused media of %s from node %s: %v", other.Id, other.NodeId, err)
			return event.GetVideoResponse{File: ""}, nil
		}

		if videoResp.File != "" {
			if err := mediaRepo.SetForeignVideoWithTTL(
				string(ev.ChatId), ev.Key, domain.Base64Video(videoResp.File),
			); err != nil {
				log.Errorf("get chat video: storing peer video: %v", err)
			}
		}

		return videoResp, nil
	}
}

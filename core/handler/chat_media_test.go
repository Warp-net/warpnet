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

//nolint:all
package handler

import (
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/stretchr/testify/assert"
)

const (
	testChatId       = "aaa:bbb"
	testOtherUserID  = "other-user"
	chatAttachment   = "data:image/png;base64,PRIVATE"
	chatAttachmentId = "chat-attachment-key"
)

type chatMediaRepoDouble struct {
	images map[string]domain.Base64Image
	videos map[string]domain.Base64Video
	stored map[string]domain.Base64Image
}

func newChatMediaRepoDouble() *chatMediaRepoDouble {
	return &chatMediaRepoDouble{
		images: map[string]domain.Base64Image{},
		videos: map[string]domain.Base64Video{},
		stored: map[string]domain.Base64Image{},
	}
}

func (r *chatMediaRepoDouble) GetImage(chatId, key string) (domain.Base64Image, error) {
	img, ok := r.images[chatId+"/"+key]
	if !ok {
		return "", database.ErrMediaNotFound
	}
	return img, nil
}

func (r *chatMediaRepoDouble) SetImage(chatId string, img domain.Base64Image) (domain.ImageKey, error) {
	r.images[chatId+"/key"] = img
	return "key", nil
}

func (r *chatMediaRepoDouble) SetForeignImageWithTTL(chatId, key string, img domain.Base64Image) error {
	r.stored[chatId+"/"+key] = img
	return nil
}

func (r *chatMediaRepoDouble) GetVideo(chatId, key string) (domain.Base64Video, error) {
	v, ok := r.videos[chatId+"/"+key]
	if !ok {
		return "", database.ErrMediaNotFound
	}
	return v, nil
}

func (r *chatMediaRepoDouble) SetVideo(chatId string, video domain.Base64Video) (domain.VideoKey, error) {
	r.videos[chatId+"/key"] = video
	return "key", nil
}

func (r *chatMediaRepoDouble) SetForeignVideoWithTTL(chatId, key string, video domain.Base64Video) error {
	r.videos[chatId+"/"+key] = video
	return nil
}

type chatFetcherDouble struct {
	chats map[string]domain.Chat
}

func (d chatFetcherDouble) GetChat(chatId string) (domain.Chat, error) {
	chat, ok := d.chats[chatId]
	if !ok {
		return domain.Chat{}, database.ErrChatNotFound
	}
	return chat, nil
}

func ownChat() chatFetcherDouble {
	return chatFetcherDouble{chats: map[string]domain.Chat{
		testChatId: {Id: testChatId, OwnerId: ownerID, OtherUserId: testOtherUserID},
	}}
}

func chatUsers() mediaUserDouble {
	return mediaUserDouble{users: map[string]domain.User{
		ownerID:         {Id: ownerID, NodeId: selfNodeID},
		testOtherUserID: {Id: testOtherUserID, NodeId: remoteNodeID},
	}}
}

func streamFrom(remote warpnet.WarpPeerID) warpnet.WarpStream {
	_, peerStream := stream.NewLoopbackStream(
		warpnet.FromStringToPeerID(selfNodeID), remote, "/test/route/0.0.0",
	)
	return peerStream
}

func getChatImage(t *testing.T, h warpnet.WarpHandlerFunc, chatId string, remote warpnet.WarpPeerID) string {
	t.Helper()
	out, err := h(mustJSON(t, event.GetChatImageEvent{ChatId: chatId, Key: chatAttachmentId}), streamFrom(remote))
	assert.NoError(t, err)
	resp, ok := out.(event.GetImageResponse)
	assert.True(t, ok)
	return resp.File
}

// The gate is the whole point of these routes: a chat attachment must reach
// its two participants and nobody else, however the asker learned the key.
func TestGetChatImage_ParticipantsOnly(t *testing.T) {
	repo := newChatMediaRepoDouble()
	repo.images[testChatId+"/"+chatAttachmentId] = domain.Base64Image(chatAttachment)

	h := StreamGetChatImageHandler(&mediaStreamerDouble{}, repo, ownChat(), chatUsers())

	t.Run("owner's own interface gets the file", func(t *testing.T) {
		assert.Equal(t, chatAttachment, getChatImage(t, h, testChatId, warpnet.FromStringToPeerID(selfNodeID)))
	})

	t.Run("the other participant gets the file", func(t *testing.T) {
		assert.Equal(t, chatAttachment, getChatImage(t, h, testChatId, warpnet.FromStringToPeerID(remoteNodeID)))
	})

	t.Run("outsider that knows the key gets nothing", func(t *testing.T) {
		assert.Empty(t, getChatImage(t, h, testChatId, testSignerID))
	})

	t.Run("unknown chat gets nothing", func(t *testing.T) {
		assert.Empty(t, getChatImage(t, h, "zzz:yyy", warpnet.FromStringToPeerID(remoteNodeID)))
	})
}

// A chat the owner is not part of must be refused even to a peer that is in it,
// so a node cannot be used to relay someone else's conversation.
func TestGetChatImage_RefusesChatOwnerIsNotIn(t *testing.T) {
	repo := newChatMediaRepoDouble()
	repo.images[testChatId+"/"+chatAttachmentId] = domain.Base64Image(chatAttachment)

	foreign := chatFetcherDouble{chats: map[string]domain.Chat{
		testChatId: {Id: testChatId, OwnerId: "stranger-a", OtherUserId: testOtherUserID},
	}}
	h := StreamGetChatImageHandler(&mediaStreamerDouble{}, repo, foreign, chatUsers())

	assert.Empty(t, getChatImage(t, h, testChatId, warpnet.FromStringToPeerID(remoteNodeID)))
	assert.Empty(t, getChatImage(t, h, testChatId, warpnet.FromStringToPeerID(selfNodeID)))
}

// A remote participant asking for something this node does not hold must not
// make it go fetch on their behalf — only the owner's own interface may.
func TestGetChatImage_RemotePeerNeverTriggersFetch(t *testing.T) {
	streamer := &mediaStreamerDouble{}
	h := StreamGetChatImageHandler(streamer, newChatMediaRepoDouble(), ownChat(), chatUsers())

	assert.Empty(t, getChatImage(t, h, testChatId, warpnet.FromStringToPeerID(remoteNodeID)))
	assert.Empty(t, streamer.streamedTo, "a remote ask must not fan out to another node")
}

func TestGetChatImage_Validation(t *testing.T) {
	h := StreamGetChatImageHandler(&mediaStreamerDouble{}, newChatMediaRepoDouble(), ownChat(), chatUsers())

	t.Run("invalid payload", func(t *testing.T) {
		_, err := h([]byte("not json"), streamFrom(warpnet.FromStringToPeerID(selfNodeID)))
		assert.Error(t, err)
	})

	t.Run("empty chat id", func(t *testing.T) {
		_, err := h(
			mustJSON(t, event.GetChatImageEvent{Key: chatAttachmentId}),
			streamFrom(warpnet.FromStringToPeerID(selfNodeID)),
		)
		assert.ErrorIs(t, err, ErrEmptyChatId)
	})

	t.Run("empty key", func(t *testing.T) {
		_, err := h(
			mustJSON(t, event.GetChatImageEvent{ChatId: testChatId}),
			streamFrom(warpnet.FromStringToPeerID(selfNodeID)),
		)
		assert.ErrorIs(t, err, ErrEmptyImageKey)
	})
}

func TestUploadChatImage_RefusesForeignChat(t *testing.T) {
	foreign := chatFetcherDouble{chats: map[string]domain.Chat{
		testChatId: {Id: testChatId, OwnerId: "stranger-a", OtherUserId: "stranger-b"},
	}}
	h := StreamUploadChatImageHandler(
		n{}, testSignerKey, newChatMediaRepoDouble(), foreign, chatUsers(),
	)

	_, err := h(
		mustJSON(t, event.UploadChatImageEvent{ChatId: testChatId, Image1: testImagePNG}),
		streamFrom(testSignerID),
	)
	assert.ErrorIs(t, err, ErrForeignChatMedia)
}

func TestUploadChatImage_StoresUnderTheChat(t *testing.T) {
	repo := newChatMediaRepoDouble()
	h := StreamUploadChatImageHandler(n{}, testSignerKey, repo, ownChat(), chatUsers())

	out, err := h(
		mustJSON(t, event.UploadChatImageEvent{ChatId: testChatId, Image1: testImagePNG}),
		streamFrom(testSignerID),
	)
	assert.NoError(t, err)

	resp, ok := out.(event.UploadImageResponse)
	assert.True(t, ok)
	assert.NotEmpty(t, resp.Key1)
	assert.Contains(t, repo.images, testChatId+"/key", "chat attachment must land under its chat")
}

func TestUploadChatImage_Validation(t *testing.T) {
	h := StreamUploadChatImageHandler(
		n{}, testSignerKey, newChatMediaRepoDouble(), ownChat(), chatUsers(),
	)
	own := streamFrom(testSignerID)

	t.Run("empty chat id", func(t *testing.T) {
		_, err := h(mustJSON(t, event.UploadChatImageEvent{Image1: testImagePNG}), own)
		assert.ErrorIs(t, err, ErrEmptyChatId)
	})

	t.Run("no images", func(t *testing.T) {
		_, err := h(mustJSON(t, event.UploadChatImageEvent{ChatId: testChatId}), own)
		assert.ErrorIs(t, err, ErrNoImagesProvided)
	})
}

func TestGetChatVideo_ParticipantsOnly(t *testing.T) {
	repo := newChatMediaRepoDouble()
	repo.videos[testChatId+"/"+chatAttachmentId] = domain.Base64Video("data:video/mp4;base64,PRIVATE")

	h := StreamGetChatVideoHandler(&mediaStreamerDouble{}, repo, ownChat(), chatUsers())

	serve := func(remote warpnet.WarpPeerID) string {
		out, err := h(
			mustJSON(t, event.GetChatVideoEvent{ChatId: testChatId, Key: chatAttachmentId}),
			streamFrom(remote),
		)
		assert.NoError(t, err)
		resp, ok := out.(event.GetVideoResponse)
		assert.True(t, ok)
		return resp.File
	}

	assert.Equal(t, "data:video/mp4;base64,PRIVATE", serve(warpnet.FromStringToPeerID(remoteNodeID)))
	assert.Empty(t, serve(testSignerID))
}

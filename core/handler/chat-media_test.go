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
	testPartnerID    = "chat-partner"
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

func (r *chatMediaRepoDouble) GetImage(userId, key string) (domain.Base64Image, error) {
	img, ok := r.images[userId+"/"+key]
	if !ok {
		return "", database.ErrMediaNotFound
	}
	return img, nil
}

func (r *chatMediaRepoDouble) SetForeignImageWithTTL(userId, key string, img domain.Base64Image) error {
	r.stored[userId+"/"+key] = img
	return nil
}

func (r *chatMediaRepoDouble) GetVideo(userId, key string) (domain.Base64Video, error) {
	v, ok := r.videos[userId+"/"+key]
	if !ok {
		return "", database.ErrMediaNotFound
	}
	return v, nil
}

func (r *chatMediaRepoDouble) SetForeignVideoWithTTL(userId, key string, video domain.Base64Video) error {
	r.videos[userId+"/"+key] = video
	return nil
}

type chatFetcherDouble struct {
	chats []domain.Chat
	err   error
}

func (d chatFetcherDouble) GetUserChats(userId string, limit *uint64, cursor *string) ([]domain.Chat, string, error) {
	if d.err != nil {
		return nil, "", d.err
	}
	return d.chats, event.EndCursor, nil
}

func ownChat() chatFetcherDouble {
	return chatFetcherDouble{chats: []domain.Chat{
		{Id: "aaa:bbb", OwnerId: ownerID, OtherUserId: testPartnerID},
	}}
}

type chatUserDouble struct {
	byId   map[string]domain.User
	byNode map[string]domain.User
}

func (d chatUserDouble) Get(userId string) (domain.User, error) {
	u, ok := d.byId[userId]
	if !ok {
		return domain.User{}, database.ErrUserNotFound
	}
	return u, nil
}

func (d chatUserDouble) GetByNodeID(nodeID string) (domain.User, error) {
	u, ok := d.byNode[nodeID]
	if !ok {
		return domain.User{}, database.ErrUserNotFound
	}
	return u, nil
}

func chatUsers() chatUserDouble {
	partner := domain.User{Id: testPartnerID, NodeId: remoteNodeID}
	stranger := domain.User{Id: "stranger", NodeId: testSignerID.String()}
	return chatUserDouble{
		byId: map[string]domain.User{
			ownerID:       {Id: ownerID, NodeId: selfNodeID},
			testPartnerID: partner,
		},
		byNode: map[string]domain.User{
			remoteNodeID:          partner,
			testSignerID.String(): stranger,
		},
	}
}

func streamFrom(remote warpnet.WarpPeerID) warpnet.WarpStream {
	_, peerStream := stream.NewLoopbackStream(
		warpnet.FromStringToPeerID(selfNodeID), remote, "/test/route/0.0.0",
	)
	return peerStream
}

func getChatImage(t *testing.T, h warpnet.WarpHandlerFunc, userId string, remote warpnet.WarpPeerID) string {
	t.Helper()
	out, err := h(mustJSON(t, event.GetImageEvent{UserId: userId, Key: chatAttachmentId}), streamFrom(remote))
	assert.NoError(t, err)
	resp, ok := out.(event.GetImageResponse)
	assert.True(t, ok)
	return resp.File
}

func TestGetChatImage_ChatPartnersOnly(t *testing.T) {
	repo := newChatMediaRepoDouble()
	repo.images[ownerID+"/"+chatAttachmentId] = domain.Base64Image(chatAttachment)

	h := StreamGetChatImageHandler(&mediaStreamerDouble{}, repo, ownChat(), chatUsers())

	t.Run("owner's own interface gets the file", func(t *testing.T) {
		assert.Equal(t, chatAttachment, getChatImage(t, h, ownerID, warpnet.FromStringToPeerID(selfNodeID)))
	})

	t.Run("a peer the owner chats with gets the file", func(t *testing.T) {
		assert.Equal(t, chatAttachment, getChatImage(t, h, ownerID, warpnet.FromStringToPeerID(remoteNodeID)))
	})

	t.Run("outsider that knows the key gets nothing", func(t *testing.T) {
		assert.Empty(t, getChatImage(t, h, ownerID, testSignerID))
	})

	t.Run("a peer with no chat gets nothing", func(t *testing.T) {
		bare := StreamGetChatImageHandler(
			&mediaStreamerDouble{}, repo, chatFetcherDouble{}, chatUsers(),
		)
		assert.Empty(t, getChatImage(t, bare, ownerID, warpnet.FromStringToPeerID(remoteNodeID)))
	})
}

func TestGetChatImage_RemotePeerNeverTriggersFetch(t *testing.T) {
	streamer := &mediaStreamerDouble{}
	h := StreamGetChatImageHandler(streamer, newChatMediaRepoDouble(), ownChat(), chatUsers())

	assert.Empty(t, getChatImage(t, h, testPartnerID, warpnet.FromStringToPeerID(remoteNodeID)))
	assert.Empty(t, streamer.streamedTo, "a remote ask must not fan out to another node")
}

func TestGetChatImage_EmptyKey(t *testing.T) {
	h := StreamGetChatImageHandler(&mediaStreamerDouble{}, newChatMediaRepoDouble(), ownChat(), chatUsers())

	_, err := h(
		mustJSON(t, event.GetImageEvent{UserId: ownerID}),
		streamFrom(warpnet.FromStringToPeerID(selfNodeID)),
	)
	assert.ErrorIs(t, err, ErrEmptyImageKey)
}

func TestGetChatImage_InvalidPayload(t *testing.T) {
	h := StreamGetChatImageHandler(&mediaStreamerDouble{}, newChatMediaRepoDouble(), ownChat(), chatUsers())

	_, err := h([]byte("not json"), streamFrom(warpnet.FromStringToPeerID(selfNodeID)))
	assert.Error(t, err)
}

func TestGetChatVideo_ChatPartnersOnly(t *testing.T) {
	repo := newChatMediaRepoDouble()
	repo.videos[ownerID+"/"+chatAttachmentId] = domain.Base64Video("data:video/mp4;base64,PRIVATE")

	h := StreamGetChatVideoHandler(&mediaStreamerDouble{}, repo, ownChat(), chatUsers())

	serve := func(remote warpnet.WarpPeerID) string {
		out, err := h(
			mustJSON(t, event.GetVideoEvent{UserId: ownerID, Key: chatAttachmentId}),
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

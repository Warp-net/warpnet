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
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	hostileOwnerID    = "owner-id"
	hostileSelfNode   = "12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"
	hostileRemoteNode = "12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU"
)

// --- configurable doubles -------------------------------------------------

type mediaStreamerDouble struct {
	ownerId  string
	nodeId   string
	response []byte
	err      error

	streamedTo []string
}

func (m *mediaStreamerDouble) NodeInfo() warpnet.NodeInfo {
	id := m.nodeId
	if id == "" {
		id = hostileSelfNode
	}
	pid := warpnet.FromStringToPeerID(id)
	owner := m.ownerId
	if owner == "" {
		owner = hostileOwnerID
	}
	return warpnet.NodeInfo{OwnerId: owner, ID: pid}
}

func (m *mediaStreamerDouble) GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
	m.streamedTo = append(m.streamedTo, nodeId)
	return m.response, m.err
}

type imageRepoDouble struct {
	images     map[string]database.Base64Image
	getErr     error
	getPartial database.Base64Image

	foreignStored map[string]database.Base64Image
	foreignErr    error
}

func newImageRepoDouble() *imageRepoDouble {
	return &imageRepoDouble{
		images:        map[string]database.Base64Image{},
		foreignStored: map[string]database.Base64Image{},
	}
}

func (r *imageRepoDouble) GetImage(userId, key string) (database.Base64Image, error) {
	if r.getErr != nil {
		return r.getPartial, r.getErr
	}
	img, ok := r.images[userId+"/"+key]
	if !ok {
		return "", database.ErrMediaNotFound
	}
	return img, nil
}

func (r *imageRepoDouble) SetImage(userId string, img database.Base64Image) (database.ImageKey, error) {
	r.images[userId+"/key"] = img
	return "key", nil
}

func (r *imageRepoDouble) SetForeignImageWithTTL(userId, key string, img database.Base64Image) error {
	if r.foreignErr != nil {
		return r.foreignErr
	}
	r.foreignStored[userId+"/"+key] = img
	return nil
}

type videoRepoDouble struct {
	videos     map[string]database.Base64Video
	getErr     error
	getPartial database.Base64Video

	foreignStored map[string]database.Base64Video
}

func newVideoRepoDouble() *videoRepoDouble {
	return &videoRepoDouble{
		videos:        map[string]database.Base64Video{},
		foreignStored: map[string]database.Base64Video{},
	}
}

func (r *videoRepoDouble) GetVideo(userId, key string) (database.Base64Video, error) {
	if r.getErr != nil {
		return r.getPartial, r.getErr
	}
	v, ok := r.videos[userId+"/"+key]
	if !ok {
		return "", database.ErrMediaNotFound
	}
	return v, nil
}

func (r *videoRepoDouble) SetVideo(userId string, video database.Base64Video) (database.VideoKey, error) {
	r.videos[userId+"/key"] = video
	return "key", nil
}

func (r *videoRepoDouble) SetForeignVideoWithTTL(userId, key string, video database.Base64Video) error {
	r.foreignStored[userId+"/"+key] = video
	return nil
}

type mediaUserDouble struct {
	users map[string]domain.User
	err   error
}

func (d mediaUserDouble) Get(userId string) (domain.User, error) {
	if d.err != nil {
		return domain.User{}, d.err
	}
	u, ok := d.users[userId]
	if !ok {
		return domain.User{}, database.ErrUserNotFound
	}
	return u, nil
}

// --- images ---------------------------------------------------------------

func TestStreamGetImageHandler_Hostile(t *testing.T) {
	t.Run("malformed payload", func(t *testing.T) {
		h := StreamGetImageHandler(&mediaStreamerDouble{}, newImageRepoDouble(), mediaUserDouble{})
		_, err := h([]byte("{"), nil)
		assert.Error(t, err)
	})

	t.Run("empty key is rejected", func(t *testing.T) {
		h := StreamGetImageHandler(&mediaStreamerDouble{}, newImageRepoDouble(), mediaUserDouble{})
		_, err := h(mustJSON(t, event.GetImageEvent{UserId: hostileOwnerID}), nil)
		assert.ErrorIs(t, err, ErrEmptyImageKey)
	})

	// A missing avatar must render as a placeholder, not break the whole
	// profile view with an error.
	t.Run("own missing image answers empty, not an error", func(t *testing.T) {
		h := StreamGetImageHandler(&mediaStreamerDouble{}, newImageRepoDouble(), mediaUserDouble{})
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: hostileOwnerID, Key: "gone"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: ""}, out)
	})

	t.Run("own image is served from local storage", func(t *testing.T) {
		repo := newImageRepoDouble()
		repo.images[hostileOwnerID+"/avatar"] = "data:image/jpeg;base64,MINE"

		h := StreamGetImageHandler(&mediaStreamerDouble{}, repo, mediaUserDouble{})
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: hostileOwnerID, Key: "avatar"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: "data:image/jpeg;base64,MINE"}, out)
	})

	// An unset user id means "mine" — it must never resolve to a random peer.
	t.Run("missing user id defaults to this node's owner", func(t *testing.T) {
		repo := newImageRepoDouble()
		repo.images[hostileOwnerID+"/avatar"] = "data:image/jpeg;base64,MINE"

		streamer := &mediaStreamerDouble{}
		h := StreamGetImageHandler(streamer, repo, mediaUserDouble{})
		out, err := h(mustJSON(t, event.GetImageEvent{Key: "avatar"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: "data:image/jpeg;base64,MINE"}, out)
		assert.Empty(t, streamer.streamedTo, "an own-image read must not hit the network")
	})

	// An unreadable-but-empty payload degrades to a placeholder: an avatar is
	// never worth failing a whole profile render over.
	t.Run("empty payload with a storage error degrades to a placeholder", func(t *testing.T) {
		repo := newImageRepoDouble()
		repo.getErr = errors.New("disk on fire")

		h := StreamGetImageHandler(&mediaStreamerDouble{}, repo, mediaUserDouble{})
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: hostileOwnerID, Key: "avatar"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: ""}, out)
	})

	// A partially-read payload is different: the data is suspect, so the read
	// must fail loudly rather than hand back a corrupt image.
	t.Run("storage error with partial data surfaces", func(t *testing.T) {
		repo := newImageRepoDouble()
		repo.getErr = errors.New("disk on fire")
		repo.getPartial = "data:image/jpeg;base64,TRUNCATED"

		h := StreamGetImageHandler(&mediaStreamerDouble{}, repo, mediaUserDouble{})
		_, err := h(mustJSON(t, event.GetImageEvent{UserId: hostileOwnerID, Key: "avatar"}), nil)
		assert.Error(t, err)
	})

	t.Run("unknown remote user falls back to whatever is cached", func(t *testing.T) {
		repo := newImageRepoDouble()
		repo.images["stranger/avatar"] = "data:image/jpeg;base64,CACHED"

		streamer := &mediaStreamerDouble{}
		h := StreamGetImageHandler(streamer, repo, mediaUserDouble{})
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: "stranger", Key: "avatar"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: "data:image/jpeg;base64,CACHED"}, out)
		assert.Empty(t, streamer.streamedTo, "an unknown user has no node to ask")
	})

	t.Run("user lookup failure surfaces", func(t *testing.T) {
		h := StreamGetImageHandler(&mediaStreamerDouble{}, newImageRepoDouble(),
			mediaUserDouble{err: errors.New("db down")})
		_, err := h(mustJSON(t, event.GetImageEvent{UserId: "someone", Key: "avatar"}), nil)
		assert.Error(t, err)
	})

	// An alias of this very node must not be asked over the network — that is
	// a self-call that would deadlock or loop.
	t.Run("own alias is not streamed to", func(t *testing.T) {
		streamer := &mediaStreamerDouble{}
		users := mediaUserDouble{users: map[string]domain.User{
			"alias": {Id: "alias", NodeId: hostileSelfNode},
		}}

		h := StreamGetImageHandler(streamer, newImageRepoDouble(), users)
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: "alias", Key: "avatar"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: ""}, out)
		assert.Empty(t, streamer.streamedTo)
	})

	// A cached foreign avatar must win so a profile view doesn't cost a
	// round-trip on every render.
	t.Run("cached foreign image short-circuits the network", func(t *testing.T) {
		repo := newImageRepoDouble()
		repo.images["remote/avatar"] = "data:image/jpeg;base64,CACHED"

		streamer := &mediaStreamerDouble{}
		users := mediaUserDouble{users: map[string]domain.User{
			"remote": {Id: "remote", NodeId: hostileRemoteNode},
		}}

		h := StreamGetImageHandler(streamer, repo, users)
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: "remote", Key: "avatar"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: "data:image/jpeg;base64,CACHED"}, out)
		assert.Empty(t, streamer.streamedTo)
	})

	// An offline author means "no picture yet", not a failed profile load.
	t.Run("offline peer degrades to an empty image", func(t *testing.T) {
		streamer := &mediaStreamerDouble{err: warpnet.ErrNodeIsOffline}
		users := mediaUserDouble{users: map[string]domain.User{
			"remote": {Id: "remote", NodeId: hostileRemoteNode},
		}}

		h := StreamGetImageHandler(streamer, newImageRepoDouble(), users)
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: "remote", Key: "avatar"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: ""}, out)
	})

	t.Run("transport failure surfaces", func(t *testing.T) {
		streamer := &mediaStreamerDouble{err: errors.New("connection reset")}
		users := mediaUserDouble{users: map[string]domain.User{
			"remote": {Id: "remote", NodeId: hostileRemoteNode},
		}}

		h := StreamGetImageHandler(streamer, newImageRepoDouble(), users)
		_, err := h(mustJSON(t, event.GetImageEvent{UserId: "remote", Key: "avatar"}), nil)
		assert.Error(t, err)
	})

	t.Run("garbage from the peer is rejected", func(t *testing.T) {
		streamer := &mediaStreamerDouble{response: []byte("<html>nope</html>")}
		users := mediaUserDouble{users: map[string]domain.User{
			"remote": {Id: "remote", NodeId: hostileRemoteNode},
		}}

		h := StreamGetImageHandler(streamer, newImageRepoDouble(), users)
		_, err := h(mustJSON(t, event.GetImageEvent{UserId: "remote", Key: "avatar"}), nil)
		assert.Error(t, err)
	})

	t.Run("fetched foreign image is cached for next time", func(t *testing.T) {
		repo := newImageRepoDouble()
		streamer := &mediaStreamerDouble{
			response: mustJSON(t, event.GetImageResponse{File: "data:image/jpeg;base64,REMOTE"}),
		}
		users := mediaUserDouble{users: map[string]domain.User{
			"remote": {Id: "remote", NodeId: hostileRemoteNode},
		}}

		h := StreamGetImageHandler(streamer, repo, users)
		_, err := h(mustJSON(t, event.GetImageEvent{UserId: "remote", Key: "avatar"}), nil)
		require.NoError(t, err)

		assert.Equal(t, []string{hostileRemoteNode}, streamer.streamedTo)
		assert.Equal(t, database.Base64Image("data:image/jpeg;base64,REMOTE"),
			repo.foreignStored["remote/avatar"])
	})

	// A cache write failure must not lose the image the reader already asked for.
	t.Run("cache write failure still returns the image", func(t *testing.T) {
		repo := newImageRepoDouble()
		repo.foreignErr = errors.New("disk full")
		streamer := &mediaStreamerDouble{
			response: mustJSON(t, event.GetImageResponse{File: "data:image/jpeg;base64,REMOTE"}),
		}
		users := mediaUserDouble{users: map[string]domain.User{
			"remote": {Id: "remote", NodeId: hostileRemoteNode},
		}}

		h := StreamGetImageHandler(streamer, repo, users)
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: "remote", Key: "avatar"}), nil)
		require.NoError(t, err)
		assert.NotNil(t, out)
	})
}

// --- videos ---------------------------------------------------------------

func TestStreamGetVideoHandler_Hostile(t *testing.T) {
	remoteUsers := mediaUserDouble{users: map[string]domain.User{
		"remote": {Id: "remote", NodeId: hostileRemoteNode},
	}}

	t.Run("malformed payload", func(t *testing.T) {
		h := StreamGetVideoHandler(&mediaStreamerDouble{}, newVideoRepoDouble(), mediaUserDouble{})
		_, err := h([]byte("{"), nil)
		assert.Error(t, err)
	})

	t.Run("empty key is rejected", func(t *testing.T) {
		h := StreamGetVideoHandler(&mediaStreamerDouble{}, newVideoRepoDouble(), mediaUserDouble{})
		_, err := h(mustJSON(t, event.GetVideoEvent{UserId: hostileOwnerID}), nil)
		assert.ErrorIs(t, err, ErrEmptyVideoKey)
	})

	t.Run("own missing video answers empty", func(t *testing.T) {
		h := StreamGetVideoHandler(&mediaStreamerDouble{}, newVideoRepoDouble(), mediaUserDouble{})
		out, err := h(mustJSON(t, event.GetVideoEvent{UserId: hostileOwnerID, Key: "gone"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetVideoResponse{File: ""}, out)
	})

	t.Run("empty payload with a storage error degrades to a placeholder", func(t *testing.T) {
		repo := newVideoRepoDouble()
		repo.getErr = errors.New("disk on fire")

		h := StreamGetVideoHandler(&mediaStreamerDouble{}, repo, mediaUserDouble{})
		out, err := h(mustJSON(t, event.GetVideoEvent{UserId: hostileOwnerID, Key: "clip"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetVideoResponse{File: ""}, out)
	})

	t.Run("storage error with partial data surfaces", func(t *testing.T) {
		repo := newVideoRepoDouble()
		repo.getErr = errors.New("disk on fire")
		repo.getPartial = "data:video/mp4;base64,TRUNCATED"

		h := StreamGetVideoHandler(&mediaStreamerDouble{}, repo, mediaUserDouble{})
		_, err := h(mustJSON(t, event.GetVideoEvent{UserId: hostileOwnerID, Key: "clip"}), nil)
		assert.Error(t, err)
	})

	// Deferred reads are how the timeline avoids shipping megabytes per row:
	// the size must arrive so the player can lay out, but not the bytes.
	t.Run("deferred own video withholds bytes but reports size", func(t *testing.T) {
		repo := newVideoRepoDouble()
		repo.videos[hostileOwnerID+"/clip"] = "data:video/mp4;base64,PAYLOAD"

		h := StreamGetVideoHandler(&mediaStreamerDouble{}, repo, mediaUserDouble{})
		out, err := h(mustJSON(t, event.GetVideoEvent{UserId: hostileOwnerID, Key: "clip", Deferred: true}), nil)
		require.NoError(t, err)

		resp := out.(event.GetVideoResponse)
		assert.Empty(t, resp.File)
		assert.True(t, resp.Deferred)
		assert.Equal(t, int64(len("data:video/mp4;base64,PAYLOAD")), resp.Size)
	})

	t.Run("unknown user falls back to cache", func(t *testing.T) {
		repo := newVideoRepoDouble()
		repo.videos["stranger/clip"] = "data:video/mp4;base64,CACHED"

		streamer := &mediaStreamerDouble{}
		h := StreamGetVideoHandler(streamer, repo, mediaUserDouble{})
		out, err := h(mustJSON(t, event.GetVideoEvent{UserId: "stranger", Key: "clip"}), nil)
		require.NoError(t, err)
		assert.Equal(t, "data:video/mp4;base64,CACHED", out.(event.GetVideoResponse).File)
		assert.Empty(t, streamer.streamedTo)
	})

	t.Run("user lookup failure surfaces", func(t *testing.T) {
		h := StreamGetVideoHandler(&mediaStreamerDouble{}, newVideoRepoDouble(),
			mediaUserDouble{err: errors.New("db down")})
		_, err := h(mustJSON(t, event.GetVideoEvent{UserId: "someone", Key: "clip"}), nil)
		assert.Error(t, err)
	})

	t.Run("own alias is not streamed to", func(t *testing.T) {
		streamer := &mediaStreamerDouble{}
		users := mediaUserDouble{users: map[string]domain.User{
			"alias": {Id: "alias", NodeId: hostileSelfNode},
		}}

		h := StreamGetVideoHandler(streamer, newVideoRepoDouble(), users)
		out, err := h(mustJSON(t, event.GetVideoEvent{UserId: "alias", Key: "clip"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetVideoResponse{File: ""}, out)
		assert.Empty(t, streamer.streamedTo)
	})

	// A deferred remote read must never fan out to the author's node — that
	// is the whole point of deferring.
	t.Run("deferred remote video does not hit the network", func(t *testing.T) {
		streamer := &mediaStreamerDouble{}
		h := StreamGetVideoHandler(streamer, newVideoRepoDouble(), remoteUsers)

		out, err := h(mustJSON(t, event.GetVideoEvent{UserId: "remote", Key: "clip", Deferred: true}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetVideoResponse{File: "", Deferred: true}, out)
		assert.Empty(t, streamer.streamedTo)
	})

	t.Run("offline peer degrades to an empty video", func(t *testing.T) {
		streamer := &mediaStreamerDouble{err: warpnet.ErrNodeIsOffline}
		h := StreamGetVideoHandler(streamer, newVideoRepoDouble(), remoteUsers)

		out, err := h(mustJSON(t, event.GetVideoEvent{UserId: "remote", Key: "clip"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetVideoResponse{File: ""}, out)
	})

	t.Run("transport failure surfaces", func(t *testing.T) {
		streamer := &mediaStreamerDouble{err: errors.New("reset")}
		h := StreamGetVideoHandler(streamer, newVideoRepoDouble(), remoteUsers)

		_, err := h(mustJSON(t, event.GetVideoEvent{UserId: "remote", Key: "clip"}), nil)
		assert.Error(t, err)
	})

	t.Run("garbage from the peer is rejected", func(t *testing.T) {
		streamer := &mediaStreamerDouble{response: []byte("not json")}
		h := StreamGetVideoHandler(streamer, newVideoRepoDouble(), remoteUsers)

		_, err := h(mustJSON(t, event.GetVideoEvent{UserId: "remote", Key: "clip"}), nil)
		assert.Error(t, err)
	})

	t.Run("fetched foreign video is cached", func(t *testing.T) {
		repo := newVideoRepoDouble()
		streamer := &mediaStreamerDouble{
			response: mustJSON(t, event.GetVideoResponse{File: "data:video/mp4;base64,REMOTE"}),
		}

		h := StreamGetVideoHandler(streamer, repo, remoteUsers)
		out, err := h(mustJSON(t, event.GetVideoEvent{UserId: "remote", Key: "clip"}), nil)
		require.NoError(t, err)

		assert.Equal(t, "data:video/mp4;base64,REMOTE", out.(event.GetVideoResponse).File)
		assert.Equal(t, database.Base64Video("data:video/mp4;base64,REMOTE"),
			repo.foreignStored["remote/clip"])
	})

	// An empty answer must not be written to the cache — otherwise a transient
	// blank permanently poisons the entry.
	t.Run("empty remote answer is not cached", func(t *testing.T) {
		repo := newVideoRepoDouble()
		streamer := &mediaStreamerDouble{response: mustJSON(t, event.GetVideoResponse{File: ""})}

		h := StreamGetVideoHandler(streamer, repo, remoteUsers)
		_, err := h(mustJSON(t, event.GetVideoEvent{UserId: "remote", Key: "clip"}), nil)
		require.NoError(t, err)
		assert.Empty(t, repo.foreignStored)
	})
}

func TestNewVideoResponse_Hostile(t *testing.T) {
	video := database.Base64Video("data:video/mp4;base64,ABC")

	full := newVideoResponse(video, false)
	assert.Equal(t, string(video), full.File)
	assert.Equal(t, int64(len(video)), full.Size)
	assert.False(t, full.Deferred)

	deferred := newVideoResponse(video, true)
	assert.Empty(t, deferred.File)
	assert.Equal(t, int64(len(video)), deferred.Size, "size must survive deferral for layout")
	assert.True(t, deferred.Deferred)

	empty := newVideoResponse("", false)
	assert.Empty(t, empty.File)
	assert.Zero(t, empty.Size)
}

func TestAmendExifMetadata_RejectsNonJPEG(t *testing.T) {
	_, err := amendExifMetadata([]byte("this is not a jpeg"), []byte("meta"))
	assert.Error(t, err, "a non-JPEG upload must not be parsed as one")

	_, err = amendExifMetadata(nil, []byte("meta"))
	assert.Error(t, err)
}

func TestJSONHelperRoundTrip(t *testing.T) {
	// Guards the helper the media tests rely on.
	bt := mustJSON(t, event.GetImageEvent{UserId: "u", Key: "k"})
	var back event.GetImageEvent
	require.NoError(t, json.Unmarshal(bt, &back))
	assert.Equal(t, domain.ID("u"), back.UserId)
	assert.Equal(t, "k", back.Key)
}

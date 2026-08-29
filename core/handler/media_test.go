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
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/Warp-net/warpnet/core/mastodon"
	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type signingInformer struct{ ownerId string }

func (s signingInformer) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{ID: testSignerID, OwnerId: s.ownerId}
}

type signingUserRepo struct{ ownerId string }

func (s signingUserRepo) Get(userId string) (domain.User, error) {
	return domain.User{Id: s.ownerId, NodeId: testSignerID.String()}, nil
}

func watermarkedImage(t *testing.T, ownerId string) (file, key string) {
	t.Helper()

	watermark, err := buildWatermark(signingInformer{ownerId}.NodeInfo(), testSignerKey, ownerOf(ownerId))
	require.NoError(t, err)

	img, err := watermarkUploadedImage(testImagePNG, watermark)
	require.NoError(t, err)

	return string(img), contentKeyOf(string(img))
}

func watermarkedVideo(t *testing.T, ownerId string) (file, key string) {
	t.Helper()

	watermark, err := buildWatermark(signingInformer{ownerId}.NodeInfo(), testSignerKey, ownerOf(ownerId))
	require.NoError(t, err)

	video, err := watermarkUploadedVideo(mp4DataURL(minimalMP4()), watermark)
	require.NoError(t, err)

	return string(video), contentKeyOf(string(video))
}

func rawOf(t *testing.T, dataURL string) []byte {
	t.Helper()

	_, encoded, ok := strings.Cut(dataURL, ",")
	require.True(t, ok)

	raw, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)

	return raw
}

func TestUploadVideo_ReplacesAnInheritedMetaBox(t *testing.T) {
	inherited, _ := watermarkedVideo(t, "alice")

	watermark, err := buildWatermark(signingInformer{"mallory"}.NodeInfo(), testSignerKey, ownerOf("mallory"))
	require.NoError(t, err)

	video, err := watermarkUploadedVideo(inherited, watermark)
	require.NoError(t, err)

	raw := rawOf(t, string(video))
	assert.NoError(t, media_meta.VerifyVideo(raw, testSignerID.String(), "mallory"),
		"the re-upload is attributed to whoever uploaded it")
	assert.ErrorIs(t, media_meta.VerifyVideo(raw, testSignerID.String(), "alice"),
		media_meta.ErrForgedMetadata)

	raw, meta, err := media_meta.SplitVideo(raw)
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.Equal(t, rawOf(t, inherited)[:len(minimalMP4())], raw)
}

func TestUploadVideo_HandlesOpenEndedTrailingBox(t *testing.T) {
	openEnded := append(minimalMP4(), []byte{
		0x00, 0x00, 0x00, 0x00, // size 0: to the end of the file
		'm', 'd', 'a', 't',
		0xAA, 0xBB, 0xCC, 0xDD,
	}...)

	watermark, err := buildWatermark(signingInformer{"alice"}.NodeInfo(), testSignerKey, ownerOf("alice"))
	require.NoError(t, err)

	video, err := watermarkUploadedVideo(mp4DataURL(openEnded), watermark)
	require.NoError(t, err)

	assert.NoError(t, media_meta.VerifyVideo(
		rawOf(t, string(video)), testSignerID.String(), "alice"))
}

func TestVerifyContentKey(t *testing.T) {
	file, key := watermarkedImage(t, "alice")

	assert.NoError(t, verifyContentKey(key, file))
	assert.ErrorIs(t, verifyContentKey(key, file+"tail"), ErrMediaKeyMismatch)

	assert.NoError(t, verifyContentKey("https://mastodon.social/avatar.png", file))
}

func TestVerifyForeignMedia(t *testing.T) {
	file, key := watermarkedImage(t, "alice")
	owner := domain.User{Id: "alice", NodeId: testSignerID.String()}

	t.Run("watermarked media of the user who serves it", func(t *testing.T) {
		assert.NoError(t, verifyForeignImage(owner, key, file))
	})

	t.Run("empty answer is nothing to check", func(t *testing.T) {
		assert.NoError(t, verifyForeignImage(owner, key, ""))
	})

	t.Run("content that is not what the key names", func(t *testing.T) {
		other, _ := watermarkedImage(t, "alice")
		assert.ErrorIs(t, verifyForeignImage(owner, key, other+"x"), ErrMediaKeyMismatch)
	})

	t.Run("media with no watermark", func(t *testing.T) {
		plain := imagePrefix + base64.StdEncoding.EncodeToString([]byte("no metadata here"))
		assert.ErrorIs(t, verifyForeignImage(owner, "avatar", plain), media_meta.ErrNoMetadata)
	})

	t.Run("media of another user on the same node", func(t *testing.T) {
		assert.ErrorIs(t,
			verifyForeignImage(domain.User{Id: "mallory", NodeId: testSignerID.String()}, "avatar", file),
			media_meta.ErrForgedMetadata)
	})

	t.Run("watermark of another node", func(t *testing.T) {
		otherNode := domain.User{Id: "alice", NodeId: remoteNodeID}
		assert.ErrorIs(t, verifyForeignImage(otherNode, key, file), media_meta.ErrForgedMetadata)
	})

	t.Run("watermark re-encoded away", func(t *testing.T) {
		stripped, err := transcodeToJPEG(rawOf(t, file))
		require.NoError(t, err)

		naked := imagePrefix + base64.StdEncoding.EncodeToString(stripped)
		assert.ErrorIs(t, verifyForeignImage(owner, "avatar", naked), media_meta.ErrNoMetadata)
	})

	t.Run("video with no watermark", func(t *testing.T) {
		plain := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(minimalMP4())
		assert.ErrorIs(t, verifyForeignVideo(
			domain.User{Id: "alice", NodeId: testSignerID.String()}, "clip", plain),
			media_meta.ErrNoMetadata)
	})

	t.Run("bridged fediverse media is out of scope", func(t *testing.T) {
		bridged := domain.User{Id: "warpnet@mastodon.social", Network: mastodon.Network}
		assert.NoError(t, verifyForeignImage(bridged, "https://mastodon.social/a.png", "data:image/png;base64,AAAA"))

		viaGateway := domain.User{Id: "someone@mastodon.social", NodeId: mastodon.GatewayNodeID()}
		assert.NoError(t, verifyForeignImage(viaGateway, "https://mastodon.social/b.png", "data:image/png;base64,AAAA"))
	})
}

func ownerOf(ownerId string) domain.User {
	return domain.User{Id: ownerId, NodeId: testSignerID.String()}
}

func contentKeyOf(file string) string {
	return hex.EncodeToString(security.ConvertToSHA256([]byte(file)))
}

func TestUpload_StoresNothingWhenTheNodeCannotWatermark(t *testing.T) {
	t.Run("image", func(t *testing.T) {
		repo := newImageRepoDouble()

		payload, err := json.Marshal(event.UploadImageEvent{Image1: testImagePNG})
		require.NoError(t, err)

		_, err = StreamUploadImageHandler(n{}, nil, repo, u{})(payload, s{})
		assert.ErrorIs(t, err, media_meta.ErrNoSigningKey)
		assert.Empty(t, repo.images)
	})

	t.Run("video", func(t *testing.T) {
		repo := newVideoRepoDouble()

		payload, err := json.Marshal(event.UploadVideoEvent{Video: mp4DataURL(minimalMP4())})
		require.NoError(t, err)

		_, err = StreamUploadVideoHandler(n{}, nil, repo, u{})(payload, s{})
		assert.ErrorIs(t, err, media_meta.ErrNoSigningKey)
		assert.Empty(t, repo.videos)
	})
}

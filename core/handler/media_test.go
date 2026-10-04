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

func imageWithMetadata(t *testing.T, ownerId string) (file, key string) {
	t.Helper()

	metadata, err := media_meta.BuildMetadata(signingInformer{ownerId}.NodeInfo(), testSignerKey, ownerOf(ownerId))
	require.NoError(t, err)

	img, err := media_meta.SignUploadedImage(testImagePNG, metadata)
	require.NoError(t, err)

	return string(img), contentKeyOf(string(img))
}

func videoWithMetadata(t *testing.T, ownerId string) (file, key string) {
	t.Helper()

	metadata, err := media_meta.BuildMetadata(signingInformer{ownerId}.NodeInfo(), testSignerKey, ownerOf(ownerId))
	require.NoError(t, err)

	video, err := signUploadedVideo(mp4DataURL(minimalMP4()), metadata)
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
	inherited, _ := videoWithMetadata(t, "alice")

	metadata, err := media_meta.BuildMetadata(signingInformer{"mallory"}.NodeInfo(), testSignerKey, ownerOf("mallory"))
	require.NoError(t, err)

	video, err := signUploadedVideo(inherited, metadata)
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

	metadata, err := media_meta.BuildMetadata(signingInformer{"alice"}.NodeInfo(), testSignerKey, ownerOf("alice"))
	require.NoError(t, err)

	video, err := signUploadedVideo(mp4DataURL(openEnded), metadata)
	require.NoError(t, err)

	assert.NoError(t, media_meta.VerifyVideo(
		rawOf(t, string(video)), testSignerID.String(), "alice"))
}

func ownerOf(ownerId string) domain.User {
	return domain.User{Id: ownerId, NodeId: testSignerID.String()}
}

func contentKeyOf(file string) string {
	return hex.EncodeToString(security.ConvertToSHA256([]byte(file)))
}

func TestUpload_StoresNothingWhenTheNodeCannotSign(t *testing.T) {
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

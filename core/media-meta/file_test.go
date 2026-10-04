// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package media_meta

import (
	"encoding/base64"
	"testing"

	"github.com/Warp-net/warpnet/core/fediverse"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func imageFile(t *testing.T, ownerId string) (file, key string) {
	t.Helper()

	meta, err := BuildMetadata(
		warpnet.NodeInfo{ID: signerID, OwnerId: ownerId},
		signerKey,
		domain.User{Id: ownerId, NodeId: signerID.String()},
	)
	require.NoError(t, err)

	img, err := SignUploadedImage(ImagePrefix+base64.StdEncoding.EncodeToString(testJPEG(t, 0x40)), meta)
	require.NoError(t, err)

	return string(img), BuildContentKey(string(img))
}

func TestVerifyContentKey(t *testing.T) {
	file, key := imageFile(t, "alice")

	assert.NoError(t, verifyContentKey(key, file))
	assert.ErrorIs(t, verifyContentKey(key, file+"tail"), ErrMediaKeyMismatch)

	assert.NoError(t, verifyContentKey("https://mastodon.social/avatar.png", file))
}

func TestVerifyForeignMedia(t *testing.T) {
	file, key := imageFile(t, "alice")
	owner := domain.User{Id: "alice", NodeId: signerID.String()}

	t.Run("signed media of the user who serves it", func(t *testing.T) {
		assert.NoError(t, VerifyForeignImage(owner, key, file))
	})

	t.Run("empty answer is nothing to check", func(t *testing.T) {
		assert.NoError(t, VerifyForeignImage(owner, key, ""))
	})

	t.Run("content that is not what the key names", func(t *testing.T) {
		other, _ := imageFile(t, "alice")
		assert.ErrorIs(t, VerifyForeignImage(owner, key, other+"x"), ErrMediaKeyMismatch)
	})

	t.Run("media with no metadata", func(t *testing.T) {
		plain := ImagePrefix + base64.StdEncoding.EncodeToString([]byte("no metadata here"))
		assert.ErrorIs(t, VerifyForeignImage(owner, "avatar", plain), ErrNoMetadata)
	})

	t.Run("media of another user on the same node", func(t *testing.T) {
		assert.ErrorIs(t,
			VerifyForeignImage(domain.User{Id: "mallory", NodeId: signerID.String()}, "avatar", file),
			ErrForgedMetadata)
	})

	t.Run("metadata of another node", func(t *testing.T) {
		_, otherID := mustSigner("another-node-seed")
		otherNode := domain.User{Id: "alice", NodeId: otherID.String()}
		assert.ErrorIs(t, VerifyForeignImage(otherNode, key, file), ErrForgedMetadata)
	})

	t.Run("metadata re-encoded away", func(t *testing.T) {
		_, raw, err := SplitDataURI(file)
		require.NoError(t, err)
		stripped, err := transcodeToJPEG(raw)
		require.NoError(t, err)

		naked := ImagePrefix + base64.StdEncoding.EncodeToString(stripped)
		assert.ErrorIs(t, VerifyForeignImage(owner, "avatar", naked), ErrNoMetadata)
	})

	t.Run("video with no metadata", func(t *testing.T) {
		plain := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(minimalMP4())
		assert.ErrorIs(t, VerifyForeignVideo(
			domain.User{Id: "alice", NodeId: signerID.String()}, "clip", plain),
			ErrNoMetadata)
	})

	t.Run("bridged fediverse media is out of scope", func(t *testing.T) {
		bridged := domain.User{Id: "warpnet@mastodon.social", Network: fediverse.MastodonNetwork}
		assert.NoError(t, VerifyForeignImage(bridged, "https://mastodon.social/a.png", "data:image/png;base64,AAAA"))

		viaGateway := domain.User{Id: "someone@mastodon.social", NodeId: fediverse.GatewayNodeID()}
		assert.NoError(t, VerifyForeignImage(viaGateway, "https://mastodon.social/b.png", "data:image/png;base64,AAAA"))
	})
}

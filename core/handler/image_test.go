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
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/security"

	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	jis "github.com/dsoprea/go-jpeg-image-structure/v2"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type (
	n struct{}
	m struct{}
	u struct{}
	s struct{}
)

func (u u) GetOtherNetworkUser(network, userId string) (user domain.User, err error) {
	return domain.User{}, err
}

func (m m) SetForeignImageWithTTL(userId, key string, img domain.Base64Image) error {
	return nil
}

func (m m) GetImage(userId, key string) (domain.Base64Image, error) {
	return "", nil
}

func (m m) SetImage(userId string, img domain.Base64Image) (key domain.ImageKey, err error) {
	return "", nil
}

var testSignerKey, testSignerID = mustTestSigner()

func mustTestSigner() (ed25519.PrivateKey, warpnet.WarpPeerID) {
	priv, err := security.GenerateKeyFromSeed([]byte("media-meta-test-seed"))
	if err != nil {
		panic(err)
	}
	id, err := warpnet.IDFromPublicKey(priv.Public().(ed25519.PublicKey))
	if err != nil {
		panic(err)
	}
	return priv, id
}

func (n n) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{ID: testSignerID, OwnerId: ownerID}
}

func (s s) Read(p []byte) (n int, err error) {
	return 0, nil
}

func (s s) Write(p []byte) (n int, err error) {
	return 0, nil
}

func (s s) Close() error {
	return nil
}

func (s s) CloseWrite() error {
	return nil
}

func (s s) CloseRead() error {
	return nil
}

func (s s) Reset() error {
	return nil
}

func (s s) ResetWithError(errCode network.StreamErrorCode) error {
	return nil
}

func (s s) SetDeadline(time time.Time) error {
	return nil
}

func (s s) SetReadDeadline(time time.Time) error {
	return nil
}

func (s s) SetWriteDeadline(time time.Time) error {
	return nil
}

func (s s) ID() string {
	return ""
}

func (s s) Protocol() protocol.ID {
	return ""
}

func (s s) SetProtocol(id protocol.ID) error {
	return nil
}

func (s s) Stat() network.Stats {
	return network.Stats{}
}

func (s s) Conn() network.Conn {
	return nil
}

func (s s) Scope() network.StreamScope {
	return nil
}

func (u u) Get(userId string) (user domain.User, err error) {
	return domain.User{Id: ownerID, NodeId: testSignerID.String()}, nil
}

type cachedMediaRepo struct {
	cached domain.Base64Image
}

func (c cachedMediaRepo) GetImage(userId, key string) (domain.Base64Image, error) {
	return c.cached, nil
}

func (c cachedMediaRepo) SetImage(userId string, img domain.Base64Image) (domain.ImageKey, error) {
	return "", nil
}

func (c cachedMediaRepo) SetForeignImageWithTTL(userId, key string, img domain.Base64Image) error {
	return nil
}

type recordingStreamer struct {
	streamed *bool
}

func (r recordingStreamer) GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
	*r.streamed = true
	return json.Marshal(event.GetImageResponse{File: "from-gateway"})
}

func (r recordingStreamer) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{OwnerId: "owner-id"}
}

type foreignUserRepo struct{}

func (foreignUserRepo) Get(userId string) (domain.User, error) {
	return domain.User{Id: userId, NodeId: "gateway-node", Network: "mastodon"}, nil
}

const testImagePNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABgAAAAYCAYAAADgdz34AAAABHNCSVQICAgIfAhkiAAAAAlwSFlzAAAApgAAAKYB3X3/OAAAABl0RVh0U29mdHdhcmUAd3d3Lmlua3NjYXBlLm9yZ5vuPBoAAANCSURBVEiJtZZPbBtFFMZ/M7ubXdtdb1xSFyeilBapySVU8h8OoFaooFSqiihIVIpQBKci6KEg9Q6H9kovIHoCIVQJJCKE1ENFjnAgcaSGC6rEnxBwA04Tx43t2FnvDAfjkNibxgHxnWb2e/u992bee7tCa00YFsffekFY+nUzFtjW0LrvjRXrCDIAaPLlW0nHL0SsZtVoaF98mLrx3pdhOqLtYPHChahZcYYO7KvPFxvRl5XPp1sN3adWiD1ZAqD6XYK1b/dvE5IWryTt2udLFedwc1+9kLp+vbbpoDh+6TklxBeAi9TL0taeWpdmZzQDry0AcO+jQ12RyohqqoYoo8RDwJrU+qXkjWtfi8Xxt58BdQuwQs9qC/afLwCw8tnQbqYAPsgxE1S6F3EAIXux2oQFKm0ihMsOF71dHYx+f3NND68ghCu1YIoePPQN1pGRABkJ6Bus96CutRZMydTl+TvuiRW1m3n0eDl0vRPcEysqdXn+jsQPsrHMquGeXEaY4Yk4wxWcY5V/9scqOMOVUFthatyTy8QyqwZ+kDURKoMWxNKr2EeqVKcTNOajqKoBgOE28U4tdQl5p5bwCw7BWquaZSzAPlwjlithJtp3pTImSqQRrb2Z8PHGigD4RZuNX6JYj6wj7O4TFLbCO/Mn/m8R+h6rYSUb3ekokRY6f/YukArN979jcW+V/S8g0eT/N3VN3kTqWbQ428m9/8k0P/1aIhF36PccEl6EhOcAUCrXKZXXWS3XKd2vc/TRBG9O5ELC17MmWubD2nKhUKZa26Ba2+D3P+4/MNCFwg59oWVeYhkzgN/JDR8deKBoD7Y+ljEjGZ0sosXVTvbc6RHirr2reNy1OXd6pJsQ+gqjk8VWFYmHrwBzW/n+uMPFiRwHB2I7ih8ciHFxIkd/3Omk5tCDV1t+2nNu5sxxpDFNx+huNhVT3/zMDz8usXC3ddaHBj1GHj/As08fwTS7Kt1HBTmyN29vdwAw+/wbwLVOJ3uAD1wi/dUH7Qei66PfyuRj4Ik9is+hglfbkbfR3cnZm7chlUWLdwmprtCohX4HUtlOcQjLYCu+fzGJH2QRKvP3UNz8bWk1qMxjGTOMThZ3kvgLI5AzFfo379UAAAAASUVORK5CYII="

func TestUploadImage_Success(t *testing.T) {
	ev := event.UploadImageEvent{
		Image1: testImagePNG,
	}
	bt, err := json.Marshal(ev)
	assert.NoError(t, err)

	_, err = StreamUploadImageHandler(n{}, testSignerKey, m{}, u{}, &stubImageHashes{})(bt, s{})
	assert.NoError(t, err)
}

func TestUploadMultipleImages_Success(t *testing.T) {
	ev := event.UploadImageEvent{
		Image1: testImagePNG,
		Image2: testImagePNG,
		Image3: testImagePNG,
		Image4: testImagePNG,
	}
	bt, err := json.Marshal(ev)
	assert.NoError(t, err)

	_, err = StreamUploadImageHandler(n{}, testSignerKey, m{}, u{}, &stubImageHashes{})(bt, s{})
	assert.NoError(t, err)
}

func TestUploadImage_StoresAPhoneShotStanding(t *testing.T) {
	stored := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for y := range 32 {
		for x := range 64 {
			c := color.RGBA{R: 0xff, A: 0xff}
			if x >= 32 {
				c = color.RGBA{B: 0xff, A: 0xff}
			}
			stored.SetRGBA(x, y, c)
		}
	}
	var plain bytes.Buffer
	require.NoError(t, jpeg.Encode(&plain, stored, &jpeg.Options{Quality: 100}))
	intfc, err := jis.NewJpegMediaParser().ParseBytes(plain.Bytes())
	require.NoError(t, err)
	segments := intfc.(*jis.SegmentList)
	ifdMapping, err := exifcommon.NewIfdMappingWithStandard()
	require.NoError(t, err)
	tagIndex := exif.NewTagIndex()
	require.NoError(t, exif.LoadStandardTags(tagIndex))
	rootIb := exif.NewIfdBuilder(ifdMapping, tagIndex, exifcommon.IfdStandardIfdIdentity, exifcommon.EncodeDefaultByteOrder)
	require.NoError(t, rootIb.SetStandardWithName("Orientation", []uint16{6}))
	require.NoError(t, segments.SetExif(rootIb))
	var photo bytes.Buffer
	require.NoError(t, segments.Write(&photo))

	repo := newImageRepoDouble()
	bt, err := json.Marshal(event.UploadImageEvent{Image1: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(photo.Bytes())})
	require.NoError(t, err)
	_, err = StreamUploadImageHandler(n{}, testSignerKey, repo, u{}, &stubImageHashes{})(bt, s{})
	require.NoError(t, err)
	require.Len(t, repo.images, 1)

	for _, file := range repo.images {
		_, raw, err := media_meta.SplitDataURI(string(file))
		require.NoError(t, err)
		upright, err := jpeg.Decode(bytes.NewReader(raw))
		require.NoError(t, err)
		assert.Equal(t, image.Rect(0, 0, 32, 64), upright.Bounds(), "a portrait shot is stored standing")
		r, _, b, _ := upright.At(16, 8).RGBA()
		assert.Greater(t, r, b, "the red left half of the sensor is on top")
		r, _, b, _ = upright.At(16, 56).RGBA()
		assert.Greater(t, b, r, "the blue right half of the sensor is at the bottom")
	}
}

func TestUploadImage_NoImages(t *testing.T) {
	ev := event.UploadImageEvent{}
	bt, err := json.Marshal(ev)
	assert.NoError(t, err)

	_, err = StreamUploadImageHandler(n{}, testSignerKey, m{}, u{}, &stubImageHashes{})(bt, s{})
	assert.ErrorIs(t, err, ErrNoImagesProvided)
}

// Layout produced by security.EncryptAES and embedded verbatim into media
// files: salt || nonce || ciphertext || tag. Pinned here so that a change to
// the sealing format cannot land without the media handlers noticing — the
// security package tests alone would not catch it.
const (
	metaSaltSize  = 16
	metaNonceSize = 12
	metaTagSize   = 16
)

func assertSealedMediaMeta(t *testing.T, sealed []byte) {
	t.Helper()

	assert.Greater(t, len(sealed), metaSaltSize+metaNonceSize+metaTagSize,
		"sealed meta must carry salt, nonce, ciphertext and tag")

	salt := sealed[:metaSaltSize]
	nonce := sealed[metaSaltSize : metaSaltSize+metaNonceSize]

	// Regression: the key used to come from a shuffled clock reading with a
	// fixed all-zero nonce, which made the metadata brute-forceable in ms.
	assert.NotEqual(t, make([]byte, metaSaltSize), salt, "salt must be random, not all-zero")
	assert.NotEqual(t, make([]byte, metaNonceSize), nonce, "nonce must be random, not all-zero")

	for _, marker := range []string{"node", "user", "MAC"} {
		assert.False(t, bytes.Contains(sealed, []byte(marker)),
			"plaintext marker %q must not survive sealing", marker)
	}
}

func readExifMeta(t *testing.T, data []byte) []byte {
	t.Helper()

	parser := jis.NewJpegMediaParser()

	intfc, err := parser.ParseBytes(data)
	assert.NoError(t, err)

	sl, ok := intfc.(*jis.SegmentList)
	assert.True(t, ok, "read exif meta: not a segment list")

	_, _, exifTags, err := sl.DumpExif()
	assert.NoError(t, err)

	for _, et := range exifTags {
		if et.TagName != media_meta.ImageDescriptionTag {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(et.FormattedFirst)
		assert.NoError(t, err)
		return decoded
	}

	t.Fatal("read exif meta: image description tag not found")
	return nil
}

func TestMediaMeta_EmbeddedInExifStaysSealed(t *testing.T) {
	metadata, err := media_meta.BuildMetadata(n{}.NodeInfo(), testSignerKey, ownerOf(ownerID))
	assert.NoError(t, err)
	meta := metadata.EncryptedMeta

	assertSealedMediaMeta(t, meta)

	parts := strings.SplitN(testImagePNG, ",", 2)

	imgBytes, err := base64.StdEncoding.DecodeString(parts[1])
	assert.NoError(t, err)

	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	assert.NoError(t, err)

	var imageBuf bytes.Buffer
	err = jpeg.Encode(&imageBuf, img, &jpeg.Options{Quality: 100})
	assert.NoError(t, err)

	amended, err := media_meta.EmbedInJPEG(imageBuf.Bytes(), meta)
	assert.NoError(t, err)

	// The blob must survive the EXIF round trip byte for byte.
	assert.Equal(t, meta, readExifMeta(t, amended))
}

func TestMediaMeta_EachUploadSealsAfresh(t *testing.T) {
	firstMetadata, err := media_meta.BuildMetadata(n{}.NodeInfo(), testSignerKey, ownerOf(ownerID))
	assert.NoError(t, err)
	first := firstMetadata.EncryptedMeta

	secondMetadata, err := media_meta.BuildMetadata(n{}.NodeInfo(), testSignerKey, ownerOf(ownerID))
	assert.NoError(t, err)
	second := secondMetadata.EncryptedMeta

	assert.NotEqual(t, first, second, "identical metadata must not seal identically")
	assert.NotEqual(t, first[:metaSaltSize], second[:metaSaltSize], "salt must be per-upload")
}

// A warm cache for a known foreign (e.g. Mastodon) user must be served from
// disk without a gateway round-trip, so avatars survive node restarts.
func TestGetImage_ServesForeignCacheWithoutGateway(t *testing.T) {
	var streamed bool
	h := StreamGetImageHandler(
		recordingStreamer{streamed: &streamed},
		cachedMediaRepo{cached: domain.Base64Image("data:image/png;base64,CACHED")},
		foreignUserRepo{}, &stubImageHashes{},
	)

	input, err := json.Marshal(event.GetImageEvent{UserId: "warpnet@mastodon.social", Key: "https://mastodon.social/avatar.png"})
	assert.NoError(t, err)

	out, err := h(input, s{})
	assert.NoError(t, err)

	resp, ok := out.(event.GetImageResponse)
	assert.True(t, ok)
	assert.Equal(t, "data:image/png;base64,CACHED", resp.File)
	assert.False(t, streamed, "warm foreign cache must not hit the gateway")
}

const (
	ownerID      = "owner-id"
	selfNodeID   = "12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"
	remoteNodeID = "12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU"
)

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
		id = selfNodeID
	}
	pid := warpnet.FromStringToPeerID(id)
	owner := m.ownerId
	if owner == "" {
		owner = ownerID
	}
	return warpnet.NodeInfo{OwnerId: owner, ID: pid}
}

func (m *mediaStreamerDouble) GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
	m.streamedTo = append(m.streamedTo, nodeId)
	return m.response, m.err
}

type imageRepoDouble struct {
	images     map[string]domain.Base64Image
	getErr     error
	getPartial domain.Base64Image

	foreignStored map[string]domain.Base64Image
	foreignErr    error
}

func newImageRepoDouble() *imageRepoDouble {
	return &imageRepoDouble{
		images:        map[string]domain.Base64Image{},
		foreignStored: map[string]domain.Base64Image{},
	}
}

func (r *imageRepoDouble) GetImage(userId, key string) (domain.Base64Image, error) {
	if r.getErr != nil {
		return r.getPartial, r.getErr
	}
	img, ok := r.images[userId+"/"+key]
	if !ok {
		return "", database.ErrMediaNotFound
	}
	return img, nil
}

func (r *imageRepoDouble) SetImage(userId string, img domain.Base64Image) (domain.ImageKey, error) {
	r.images[userId+"/key"] = img
	return "key", nil
}

func (r *imageRepoDouble) SetForeignImageWithTTL(userId, key string, img domain.Base64Image) error {
	if r.foreignErr != nil {
		return r.foreignErr
	}
	r.foreignStored[userId+"/"+key] = img
	return nil
}

type videoRepoDouble struct {
	videos     map[string]domain.Base64Video
	getErr     error
	getPartial domain.Base64Video

	foreignStored map[string]domain.Base64Video
}

func newVideoRepoDouble() *videoRepoDouble {
	return &videoRepoDouble{
		videos:        map[string]domain.Base64Video{},
		foreignStored: map[string]domain.Base64Video{},
	}
}

func (r *videoRepoDouble) GetVideo(userId, key string) (domain.Base64Video, error) {
	if r.getErr != nil {
		return r.getPartial, r.getErr
	}
	v, ok := r.videos[userId+"/"+key]
	if !ok {
		return "", database.ErrMediaNotFound
	}
	return v, nil
}

func (r *videoRepoDouble) SetVideo(userId string, video domain.Base64Video) (domain.VideoKey, error) {
	r.videos[userId+"/key"] = video
	return "key", nil
}

func (r *videoRepoDouble) SetForeignVideoWithTTL(userId, key string, video domain.Base64Video) error {
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

func TestStreamGetImageHandler(t *testing.T) {
	t.Run("malformed payload", func(t *testing.T) {
		h := StreamGetImageHandler(&mediaStreamerDouble{}, newImageRepoDouble(), mediaUserDouble{}, &stubImageHashes{})
		_, err := h([]byte("{"), nil)
		assert.Error(t, err)
	})

	t.Run("empty key is rejected", func(t *testing.T) {
		h := StreamGetImageHandler(&mediaStreamerDouble{}, newImageRepoDouble(), mediaUserDouble{}, &stubImageHashes{})
		_, err := h(mustJSON(t, event.GetImageEvent{UserId: ownerID}), nil)
		assert.ErrorIs(t, err, ErrEmptyImageKey)
	})

	t.Run("own missing image answers empty, not an error", func(t *testing.T) {
		h := StreamGetImageHandler(&mediaStreamerDouble{}, newImageRepoDouble(), mediaUserDouble{}, &stubImageHashes{})
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: ownerID, Key: "gone"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: ""}, out)
	})

	t.Run("own image is served from local storage", func(t *testing.T) {
		repo := newImageRepoDouble()
		repo.images[ownerID+"/avatar"] = "data:image/jpeg;base64,MINE"

		h := StreamGetImageHandler(&mediaStreamerDouble{}, repo, mediaUserDouble{}, &stubImageHashes{})
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: ownerID, Key: "avatar"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: "data:image/jpeg;base64,MINE"}, out)
	})

	t.Run("missing user id defaults to this node's owner", func(t *testing.T) {
		repo := newImageRepoDouble()
		repo.images[ownerID+"/avatar"] = "data:image/jpeg;base64,MINE"

		streamer := &mediaStreamerDouble{}
		h := StreamGetImageHandler(streamer, repo, mediaUserDouble{}, &stubImageHashes{})
		out, err := h(mustJSON(t, event.GetImageEvent{Key: "avatar"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: "data:image/jpeg;base64,MINE"}, out)
		assert.Empty(t, streamer.streamedTo, "an own-image read must not hit the network")
	})

	t.Run("empty payload with a storage error degrades to a placeholder", func(t *testing.T) {
		repo := newImageRepoDouble()
		repo.getErr = errors.New("disk on fire")

		h := StreamGetImageHandler(&mediaStreamerDouble{}, repo, mediaUserDouble{}, &stubImageHashes{})
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: ownerID, Key: "avatar"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: ""}, out)
	})

	t.Run("storage error with partial data surfaces", func(t *testing.T) {
		repo := newImageRepoDouble()
		repo.getErr = errors.New("disk on fire")
		repo.getPartial = "data:image/jpeg;base64,TRUNCATED"

		h := StreamGetImageHandler(&mediaStreamerDouble{}, repo, mediaUserDouble{}, &stubImageHashes{})
		_, err := h(mustJSON(t, event.GetImageEvent{UserId: ownerID, Key: "avatar"}), nil)
		assert.Error(t, err)
	})

	t.Run("unknown remote user falls back to whatever is cached", func(t *testing.T) {
		repo := newImageRepoDouble()
		repo.images["stranger/avatar"] = "data:image/jpeg;base64,CACHED"

		streamer := &mediaStreamerDouble{}
		h := StreamGetImageHandler(streamer, repo, mediaUserDouble{}, &stubImageHashes{})
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: "stranger", Key: "avatar"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: "data:image/jpeg;base64,CACHED"}, out)
		assert.Empty(t, streamer.streamedTo, "an unknown user has no node to ask")
	})

	t.Run("user lookup failure surfaces", func(t *testing.T) {
		h := StreamGetImageHandler(&mediaStreamerDouble{}, newImageRepoDouble(),
			mediaUserDouble{err: errors.New("db down")}, &stubImageHashes{})
		_, err := h(mustJSON(t, event.GetImageEvent{UserId: "someone", Key: "avatar"}), nil)
		assert.Error(t, err)
	})

	t.Run("own alias is not streamed to", func(t *testing.T) {
		streamer := &mediaStreamerDouble{}
		users := mediaUserDouble{users: map[string]domain.User{
			"alias": {Id: "alias", NodeId: selfNodeID},
		}}

		h := StreamGetImageHandler(streamer, newImageRepoDouble(), users, &stubImageHashes{})
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: "alias", Key: "avatar"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: ""}, out)
		assert.Empty(t, streamer.streamedTo)
	})

	t.Run("cached foreign image short-circuits the network", func(t *testing.T) {
		repo := newImageRepoDouble()
		repo.images["remote/avatar"] = "data:image/jpeg;base64,CACHED"

		streamer := &mediaStreamerDouble{}
		users := mediaUserDouble{users: map[string]domain.User{
			"remote": {Id: "remote", NodeId: remoteNodeID},
		}}

		h := StreamGetImageHandler(streamer, repo, users, &stubImageHashes{})
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: "remote", Key: "avatar"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: "data:image/jpeg;base64,CACHED"}, out)
		assert.Empty(t, streamer.streamedTo)
	})

	t.Run("offline peer degrades to an empty image", func(t *testing.T) {
		streamer := &mediaStreamerDouble{err: warpnet.ErrNodeIsOffline}
		users := mediaUserDouble{users: map[string]domain.User{
			"remote": {Id: "remote", NodeId: remoteNodeID},
		}}

		h := StreamGetImageHandler(streamer, newImageRepoDouble(), users, &stubImageHashes{})
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: "remote", Key: "avatar"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: ""}, out)
	})

	t.Run("transport failure surfaces", func(t *testing.T) {
		streamer := &mediaStreamerDouble{err: errors.New("connection reset")}
		users := mediaUserDouble{users: map[string]domain.User{
			"remote": {Id: "remote", NodeId: remoteNodeID},
		}}

		h := StreamGetImageHandler(streamer, newImageRepoDouble(), users, &stubImageHashes{})
		_, err := h(mustJSON(t, event.GetImageEvent{UserId: "remote", Key: "avatar"}), nil)
		assert.Error(t, err)
	})

	t.Run("garbage from the peer is rejected", func(t *testing.T) {
		streamer := &mediaStreamerDouble{response: []byte("<html>nope</html>")}
		users := mediaUserDouble{users: map[string]domain.User{
			"remote": {Id: "remote", NodeId: remoteNodeID},
		}}

		h := StreamGetImageHandler(streamer, newImageRepoDouble(), users, &stubImageHashes{})
		_, err := h(mustJSON(t, event.GetImageEvent{UserId: "remote", Key: "avatar"}), nil)
		assert.Error(t, err)
	})

	t.Run("fetched foreign image is cached for next time", func(t *testing.T) {
		file, key := imageWithMetadata(t, "remote")
		repo := newImageRepoDouble()
		streamer := &mediaStreamerDouble{
			response: mustJSON(t, event.GetImageResponse{File: file}),
		}
		users := mediaUserDouble{users: map[string]domain.User{
			"remote": {Id: "remote", NodeId: testSignerID.String()},
		}}

		h := StreamGetImageHandler(streamer, repo, users, &stubImageHashes{})
		_, err := h(mustJSON(t, event.GetImageEvent{UserId: "remote", Key: key}), nil)
		require.NoError(t, err)

		assert.Equal(t, []string{testSignerID.String()}, streamer.streamedTo)
		assert.Equal(t, domain.Base64Image(file), repo.foreignStored["remote/"+key])
	})

	t.Run("unattributable foreign image is refused", func(t *testing.T) {
		repo := newImageRepoDouble()
		streamer := &mediaStreamerDouble{
			response: mustJSON(t, event.GetImageResponse{File: "data:image/jpeg;base64,REMOTE"}),
		}
		users := mediaUserDouble{users: map[string]domain.User{
			"remote": {Id: "remote", NodeId: remoteNodeID},
		}}

		h := StreamGetImageHandler(streamer, repo, users, &stubImageHashes{})
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: "remote", Key: "avatar"}), nil)
		require.NoError(t, err)

		assert.Equal(t, event.GetImageResponse{File: ""}, out)
		assert.Empty(t, repo.foreignStored)
	})

	t.Run("cache write failure still returns the image", func(t *testing.T) {
		file, key := imageWithMetadata(t, "remote")
		repo := newImageRepoDouble()
		repo.foreignErr = errors.New("disk full")
		streamer := &mediaStreamerDouble{
			response: mustJSON(t, event.GetImageResponse{File: file}),
		}
		users := mediaUserDouble{users: map[string]domain.User{
			"remote": {Id: "remote", NodeId: testSignerID.String()},
		}}

		h := StreamGetImageHandler(streamer, repo, users, &stubImageHashes{})
		out, err := h(mustJSON(t, event.GetImageEvent{UserId: "remote", Key: key}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.GetImageResponse{File: file}, out)
	})
}

func TestJSONHelperRoundTrip(t *testing.T) {
	bt := mustJSON(t, event.GetImageEvent{UserId: "u", Key: "k"})
	var back event.GetImageEvent
	require.NoError(t, json.Unmarshal(bt, &back))
	assert.Equal(t, domain.ID("u"), back.UserId)
	assert.Equal(t, "k", back.Key)
}

type stubImageHashes struct {
	known []domain.TweetImageHashes
	saved []domain.TweetImageHashes
}

func (s *stubImageHashes) SetImageHashes(h domain.TweetImageHashes) error {
	s.saved = append(s.saved, h)
	return nil
}

func (s *stubImageHashes) ListImageHashes() ([]domain.TweetImageHashes, error) {
	return append(s.known, s.saved...), nil
}

type stubSponsoredTweetImages map[string]domain.Base64Image

func (s stubSponsoredTweetImages) GetImage(_, key string) (domain.Base64Image, error) {
	img, ok := s[key]
	if !ok {
		return "", database.ErrMediaNotFound
	}
	return img, nil
}

func smoothPhoto(t *testing.T, mirrored bool) string {
	t.Helper()
	const width, height = 240, 160
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			fx, fy := float64(x)/width, float64(y)/height
			if mirrored {
				fx = 1 - fx
			}
			v := 128 + 60*math.Sin(7*fx+1) + 45*math.Cos(5*fy+2) + 20*math.Sin(9*(fx+fy)+3)
			img.Set(x, y, color.RGBA{R: uint8(v), G: uint8(v * 0.8), B: uint8(255 - v), A: 0xff}) //nolint:gosec // v stays within 0..255
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}))
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func signedPhoto(t *testing.T, ownerId, file string) (signed, key string) {
	t.Helper()
	metadata, err := media_meta.BuildMetadata(signingInformer{ownerId}.NodeInfo(), testSignerKey, ownerOf(ownerId))
	require.NoError(t, err)
	img, err := media_meta.SignUploadedImage(file, metadata)
	require.NoError(t, err)
	return string(img), contentKeyOf(string(img))
}

func sponsoredHashOf(t *testing.T, authorId, file string) *stubImageHashes {
	t.Helper()
	hash, _, err := media_meta.HashImage(file)
	require.NoError(t, err)
	return &stubImageHashes{known: []domain.TweetImageHashes{{UserId: authorId, TweetId: "sponsored-1", Hashes: []string{hash}}}}
}

func TestStreamGetImageHandler_RefusesAnotherAuthorsSponsoredPhoto(t *testing.T) {
	users := mediaUserDouble{users: map[string]domain.User{
		"remote": {Id: "remote", NodeId: testSignerID.String()},
	}}
	fetch := func(t *testing.T, file, key string, hashes *stubImageHashes) (event.GetImageResponse, *imageRepoDouble) {
		t.Helper()
		repo := newImageRepoDouble()
		streamer := &mediaStreamerDouble{response: mustJSON(t, event.GetImageResponse{File: file})}
		out, err := StreamGetImageHandler(streamer, repo, users, hashes)(mustJSON(t, event.GetImageEvent{UserId: "remote", Key: key}), nil)
		require.NoError(t, err)
		return out.(event.GetImageResponse), repo
	}

	photo := smoothPhoto(t, false)
	repost, key := signedPhoto(t, "remote", photo)

	out, repo := fetch(t, repost, key, sponsoredHashOf(t, "author-1", photo))
	assert.Empty(t, out.File, "a node signing another author's sponsored photo as its own is not shown")
	assert.Empty(t, repo.foreignStored, "nor kept to be served onwards")

	mirroredRepost, mirroredKey := signedPhoto(t, "remote", smoothPhoto(t, true))
	out, _ = fetch(t, mirroredRepost, mirroredKey, sponsoredHashOf(t, "author-1", photo))
	assert.Empty(t, out.File, "mirroring the photo does not hide it")

	out, repo = fetch(t, repost, key, sponsoredHashOf(t, "remote", photo))
	assert.Equal(t, repost, out.File, "the author's own photo is served")
	assert.NotEmpty(t, repo.foreignStored)
}

func TestUploadImage_RefusesAnotherAuthorsSponsoredPhoto(t *testing.T) {
	photo := smoothPhoto(t, false)
	upload := mustJSON(t, event.UploadImageEvent{Image1: photo})

	_, err := StreamUploadImageHandler(n{}, testSignerKey, m{}, u{}, sponsoredHashOf(t, "author-1", photo))(upload, s{})
	assert.ErrorIs(t, err, ErrSponsoredImage)

	_, err = StreamUploadImageHandler(n{}, testSignerKey, m{}, u{}, sponsoredHashOf(t, ownerID, photo))(upload, s{})
	assert.NoError(t, err, "an author may post their own sponsored photo again")
}

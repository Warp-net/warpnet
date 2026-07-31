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
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	jis "github.com/dsoprea/go-jpeg-image-structure/v2"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/assert"
)

const testImagePNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABgAAAAYCAYAAADgdz34AAAABHNCSVQICAgIfAhkiAAAAAlwSFlzAAAApgAAAKYB3X3/OAAAABl0RVh0U29mdHdhcmUAd3d3Lmlua3NjYXBlLm9yZ5vuPBoAAANCSURBVEiJtZZPbBtFFMZ/M7ubXdtdb1xSFyeilBapySVU8h8OoFaooFSqiihIVIpQBKci6KEg9Q6H9kovIHoCIVQJJCKE1ENFjnAgcaSGC6rEnxBwA04Tx43t2FnvDAfjkNibxgHxnWb2e/u992bee7tCa00YFsffekFY+nUzFtjW0LrvjRXrCDIAaPLlW0nHL0SsZtVoaF98mLrx3pdhOqLtYPHChahZcYYO7KvPFxvRl5XPp1sN3adWiD1ZAqD6XYK1b/dvE5IWryTt2udLFedwc1+9kLp+vbbpoDh+6TklxBeAi9TL0taeWpdmZzQDry0AcO+jQ12RyohqqoYoo8RDwJrU+qXkjWtfi8Xxt58BdQuwQs9qC/afLwCw8tnQbqYAPsgxE1S6F3EAIXux2oQFKm0ihMsOF71dHYx+f3NND68ghCu1YIoePPQN1pGRABkJ6Bus96CutRZMydTl+TvuiRW1m3n0eDl0vRPcEysqdXn+jsQPsrHMquGeXEaY4Yk4wxWcY5V/9scqOMOVUFthatyTy8QyqwZ+kDURKoMWxNKr2EeqVKcTNOajqKoBgOE28U4tdQl5p5bwCw7BWquaZSzAPlwjlithJtp3pTImSqQRrb2Z8PHGigD4RZuNX6JYj6wj7O4TFLbCO/Mn/m8R+h6rYSUb3ekokRY6f/YukArN979jcW+V/S8g0eT/N3VN3kTqWbQ428m9/8k0P/1aIhF36PccEl6EhOcAUCrXKZXXWS3XKd2vc/TRBG9O5ELC17MmWubD2nKhUKZa26Ba2+D3P+4/MNCFwg59oWVeYhkzgN/JDR8deKBoD7Y+ljEjGZ0sosXVTvbc6RHirr2reNy1OXd6pJsQ+gqjk8VWFYmHrwBzW/n+uMPFiRwHB2I7ih8ciHFxIkd/3Omk5tCDV1t+2nNu5sxxpDFNx+huNhVT3/zMDz8usXC3ddaHBj1GHj/As08fwTS7Kt1HBTmyN29vdwAw+/wbwLVOJ3uAD1wi/dUH7Qei66PfyuRj4Ik9is+hglfbkbfR3cnZm7chlUWLdwmprtCohX4HUtlOcQjLYCu+fzGJH2QRKvP3UNz8bWk1qMxjGTOMThZ3kvgLI5AzFfo379UAAAAASUVORK5CYII="

func TestUploadImage_Success(t *testing.T) {
	ev := event.UploadImageEvent{
		Image1: testImagePNG,
	}
	bt, err := json.Marshal(ev)
	assert.NoError(t, err)

	_, err = StreamUploadImageHandler(n{}, m{}, u{})(bt, s{})
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

	_, err = StreamUploadImageHandler(n{}, m{}, u{})(bt, s{})
	assert.NoError(t, err)
}

func TestUploadImage_NoImages(t *testing.T) {
	ev := event.UploadImageEvent{}
	bt, err := json.Marshal(ev)
	assert.NoError(t, err)

	_, err = StreamUploadImageHandler(n{}, m{}, u{})(bt, s{})
	assert.ErrorIs(t, err, ErrNoImagesProvided)
}

const (
	testMetaTag   = imageDescriptionTag
	testMetaValue = "test meta value"
)

func TestAmendExif_Success(t *testing.T) {
	parts := strings.SplitN(testImagePNG, ",", 2)

	imgBytes, err := base64.StdEncoding.DecodeString(parts[1])
	assert.NoError(t, err)

	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	assert.NoError(t, err)

	var imageBuf bytes.Buffer
	err = jpeg.Encode(&imageBuf, img, &jpeg.Options{Quality: 100})
	assert.NoError(t, err)

	metaBytes := []byte(testMetaValue)

	result, err := amendExifMetadata(imageBuf.Bytes(), metaBytes)
	assert.NoError(t, err)

	validateExif(t, result)
	assert.NoError(t, err)
}

func validateExif(t *testing.T, data []byte) {
	t.Helper()

	parser := jis.NewJpegMediaParser()

	intfc, err := parser.ParseBytes(data)
	assert.NoError(t, err)

	sl, ok := intfc.(*jis.SegmentList)
	assert.True(t, ok, "validate: invalid exif type: not a segment list")

	_, _, exifTags, err := sl.DumpExif()
	assert.NoError(t, err)

	var isFound bool
	for _, et := range exifTags {
		decoded, err := base64.StdEncoding.DecodeString(et.FormattedFirst)
		assert.NoError(t, err)

		if et.TagName == testMetaTag {
			assert.Equal(t, testMetaValue, string(decoded))
			isFound = true
			break
		}
	}

	assert.True(t, isFound, "validate: meta data not found")
}

type (
	n struct{}
	m struct{}
	u struct{}
	s struct{}
)

func (u u) GetOtherNetworkUser(network, userId string) (user domain.User, err error) {
	return domain.User{}, err
}

func (m m) SetForeignImageWithTTL(userId, key string, img database.Base64Image) error {
	return nil
}

func (m m) GetImage(userId, key string) (database.Base64Image, error) {
	return "", nil
}

func (m m) SetImage(userId string, img database.Base64Image) (key database.ImageKey, err error) {
	return "", nil
}

func (n n) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{}
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
	return domain.User{}, nil
}

type cachedMediaRepo struct {
	cached database.Base64Image
}

func (c cachedMediaRepo) GetImage(userId, key string) (database.Base64Image, error) {
	return c.cached, nil
}

func (c cachedMediaRepo) SetImage(userId string, img database.Base64Image) (database.ImageKey, error) {
	return "", nil
}

func (c cachedMediaRepo) SetForeignImageWithTTL(userId, key string, img database.Base64Image) error {
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

// A warm cache for a known foreign (e.g. Mastodon) user must be served from
// disk without a gateway round-trip, so avatars survive node restarts.
func TestGetImage_ServesForeignCacheWithoutGateway(t *testing.T) {
	var streamed bool
	h := StreamGetImageHandler(
		recordingStreamer{streamed: &streamed},
		cachedMediaRepo{cached: database.Base64Image("data:image/png;base64,CACHED")},
		foreignUserRepo{},
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

// --- video ---------------------------------------------------------------

// minimalMP4 is a 24-byte ISO base media file: one `ftyp` box with major
// brand "isom". Enough for the container check, which is all the node does —
// decoding is left to the client's system codecs.
func minimalMP4() []byte {
	return []byte{
		0x00, 0x00, 0x00, 0x18, // box size = 24
		'f', 't', 'y', 'p',
		'i', 's', 'o', 'm', // major brand
		0x00, 0x00, 0x02, 0x00, // minor version
		'i', 's', 'o', 'm',
		'm', 'p', '4', '1',
	}
}

func mp4DataURL(raw []byte) string {
	return "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(raw)
}

type videoRepoStub struct {
	stored database.Base64Video
	getErr error
}

func (v *videoRepoStub) GetVideo(userId, key string) (database.Base64Video, error) {
	return v.stored, v.getErr
}

func (v *videoRepoStub) SetVideo(userId string, video database.Base64Video) (database.VideoKey, error) {
	v.stored = video
	return "video-key", nil
}

func (v *videoRepoStub) SetForeignVideoWithTTL(userId, key string, video database.Base64Video) error {
	v.stored = video
	return nil
}

func TestUploadVideo_Success(t *testing.T) {
	repo := &videoRepoStub{}
	h := StreamUploadVideoHandler(n{}, repo, u{})

	bt, err := json.Marshal(event.UploadVideoEvent{Video: mp4DataURL(minimalMP4())})
	assert.NoError(t, err)

	out, err := h(bt, s{})
	assert.NoError(t, err)

	resp, ok := out.(event.UploadVideoResponse)
	assert.True(t, ok)
	assert.Equal(t, "video-key", resp.Key)
	assert.True(t, strings.HasPrefix(string(repo.stored), "data:video/mp4;base64,"))
}

func TestUploadVideo_NoVideo(t *testing.T) {
	h := StreamUploadVideoHandler(n{}, &videoRepoStub{}, u{})

	bt, err := json.Marshal(event.UploadVideoEvent{Video: ""})
	assert.NoError(t, err)

	_, err = h(bt, s{})
	assert.ErrorIs(t, err, ErrNoVideoProvided)
}

func TestUploadVideo_InvalidPayload(t *testing.T) {
	h := StreamUploadVideoHandler(n{}, &videoRepoStub{}, u{})

	_, err := h([]byte("not json"), s{})
	assert.Error(t, err)
}

func TestUploadVideo_MissingBase64Signature(t *testing.T) {
	h := StreamUploadVideoHandler(n{}, &videoRepoStub{}, u{})

	bt, err := json.Marshal(event.UploadVideoEvent{Video: "no-comma-here"})
	assert.NoError(t, err)

	_, err = h(bt, s{})
	assert.ErrorIs(t, err, ErrInvalidBase64Signature)
}

// Unsupported containers must be rejected with a message that names what is
// actually accepted, rather than failing somewhere deep in storage.
func TestUploadVideo_UnsupportedFormatRejected(t *testing.T) {
	cases := map[string][]byte{
		"webm/matroska": {0x1A, 0x45, 0xDF, 0xA3, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		"png":           {0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D},
		"avi/riff":      {'R', 'I', 'F', 'F', 0x00, 0x00, 0x00, 0x00, 'A', 'V', 'I', ' '},
		"empty":         {},
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			h := StreamUploadVideoHandler(n{}, &videoRepoStub{}, u{})

			bt, err := json.Marshal(event.UploadVideoEvent{Video: mp4DataURL(raw)})
			assert.NoError(t, err)

			_, err = h(bt, s{})
			assert.ErrorIs(t, err, ErrUnsupportedVideo)
			assert.Contains(t, err.Error(), "MP4")
		})
	}
}

func TestUploadVideo_TooLarge(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates >50 MiB")
	}
	oversized := make([]byte, maxVideoSize+1)
	copy(oversized, minimalMP4())

	h := StreamUploadVideoHandler(n{}, &videoRepoStub{}, u{})

	bt, err := json.Marshal(event.UploadVideoEvent{Video: mp4DataURL(oversized)})
	assert.NoError(t, err)

	_, err = h(bt, s{})
	assert.ErrorIs(t, err, ErrTooLargeVideo)
}

func TestIsISOBaseMediaFile(t *testing.T) {
	leadingFree := append([]byte{
		0x00, 0x00, 0x00, 0x08, 'f', 'r', 'e', 'e',
	}, minimalMP4()...)

	assert.True(t, isISOBaseMediaFile(minimalMP4()))
	assert.True(t, isISOBaseMediaFile(leadingFree))
	assert.False(t, isISOBaseMediaFile([]byte{0x1A, 0x45, 0xDF, 0xA3, 0, 0, 0, 0}))
	assert.False(t, isISOBaseMediaFile([]byte("short")))
	assert.False(t, isISOBaseMediaFile(nil))
	// A zero-sized leading box must not spin the scan forever.
	assert.False(t, isISOBaseMediaFile([]byte{0x00, 0x00, 0x00, 0x00, 'f', 'r', 'e', 'e'}))
}

// The ownership blob must ride inside the file, mirroring the EXIF stamp on
// images, and must not disturb the original stream.
func TestAmendVideoMetadata_AppendsUUIDBox(t *testing.T) {
	original := minimalMP4()
	meta := []byte("encrypted-owner-blob")

	out, err := amendVideoMetadata(original, meta)
	assert.NoError(t, err)

	assert.True(t, bytes.HasPrefix(out, original), "original stream must be preserved verbatim")
	assert.True(t, isISOBaseMediaFile(out), "stamped file must still be a valid container")

	box := out[len(original):]
	size := binary.BigEndian.Uint32(box[0:4])
	assert.Equal(t, len(box), int(size), "box size field must match actual box length")
	assert.Equal(t, "uuid", string(box[4:8]))
	assert.Equal(t, warpnetMetaUUID[:], box[8:24])

	decoded, err := base64.StdEncoding.DecodeString(string(box[24:]))
	assert.NoError(t, err)
	assert.Equal(t, meta, decoded)
}

func TestGetVideo_EmptyKey(t *testing.T) {
	h := StreamGetVideoHandler(videoStreamerStub{}, &videoRepoStub{}, u{})

	bt, err := json.Marshal(event.GetVideoEvent{UserId: "owner-id", Key: ""})
	assert.NoError(t, err)

	_, err = h(bt, s{})
	assert.ErrorIs(t, err, ErrEmptyVideoKey)
}

type videoStreamerStub struct {
	streamed *bool
}

func (v videoStreamerStub) GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
	if v.streamed != nil {
		*v.streamed = true
	}
	return json.Marshal(event.GetVideoResponse{File: "from-peer"})
}

func (v videoStreamerStub) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{OwnerId: "owner-id"}
}

func TestGetVideo_ServesOwnVideo(t *testing.T) {
	stored := database.Base64Video("data:video/mp4;base64,STORED")
	h := StreamGetVideoHandler(videoStreamerStub{}, &videoRepoStub{stored: stored}, u{})

	bt, err := json.Marshal(event.GetVideoEvent{UserId: "owner-id", Key: "abc"})
	assert.NoError(t, err)

	out, err := h(bt, s{})
	assert.NoError(t, err)

	resp, ok := out.(event.GetVideoResponse)
	assert.True(t, ok)
	assert.Equal(t, string(stored), resp.File)
	assert.Equal(t, int64(len(stored)), resp.Size)
	assert.False(t, resp.Deferred)
}

// A deferred request is how a thin client or a metered connection avoids
// paying for a video nobody has asked to play yet: it learns the size but
// receives no bytes.
func TestGetVideo_DeferredWithholdsBytes(t *testing.T) {
	stored := database.Base64Video("data:video/mp4;base64,STORED")
	h := StreamGetVideoHandler(videoStreamerStub{}, &videoRepoStub{stored: stored}, u{})

	bt, err := json.Marshal(event.GetVideoEvent{UserId: "owner-id", Key: "abc", Deferred: true})
	assert.NoError(t, err)

	out, err := h(bt, s{})
	assert.NoError(t, err)

	resp, ok := out.(event.GetVideoResponse)
	assert.True(t, ok)
	assert.Empty(t, resp.File, "deferred response must not carry the payload")
	assert.True(t, resp.Deferred)
	assert.Equal(t, int64(len(stored)), resp.Size, "size must still be reported")
}

// A deferred request for a video this node does not hold must not trigger a
// remote fetch — that would pull megabytes only to discard them.
func TestGetVideo_DeferredSkipsRemoteFetch(t *testing.T) {
	var streamed bool
	h := StreamGetVideoHandler(
		videoStreamerStub{streamed: &streamed},
		&videoRepoStub{},
		foreignUserRepo{},
	)

	bt, err := json.Marshal(event.GetVideoEvent{UserId: "someone-else", Key: "abc", Deferred: true})
	assert.NoError(t, err)

	out, err := h(bt, s{})
	assert.NoError(t, err)

	resp, ok := out.(event.GetVideoResponse)
	assert.True(t, ok)
	assert.True(t, resp.Deferred)
	assert.False(t, streamed, "deferred request must not hit the peer")
}

// The stored prefix is echoed into every viewer's <video src>, so it must be
// rebuilt from the allow-list rather than trusted from the uploader.
func TestVideoDataPrefix(t *testing.T) {
	mp4, ok := videoDataPrefix("data:video/mp4;base64")
	assert.True(t, ok)
	assert.Equal(t, "data:video/mp4;base64,", mp4)

	mov, ok := videoDataPrefix("data:video/quicktime;base64")
	assert.True(t, ok)
	assert.Equal(t, "data:video/quicktime;base64,", mov)

	_, ok = videoDataPrefix("data:text/html;base64")
	assert.False(t, ok, "a non-video MIME must never round-trip into a player")

	_, ok = videoDataPrefix("data:video/webm;base64")
	assert.False(t, ok)

	_, ok = videoDataPrefix("data:video/mp4")
	assert.False(t, ok, "non-base64 data URLs are not accepted")
}

// A QuickTime upload must stay labelled QuickTime: nothing is transcoded, so
// normalising every video to video/mp4 would misdescribe the stored bytes.
func TestUploadVideo_PreservesDeclaredContainer(t *testing.T) {
	repo := &videoRepoStub{}
	h := StreamUploadVideoHandler(n{}, repo, u{})

	payload := "data:video/quicktime;base64," + base64.StdEncoding.EncodeToString(minimalMP4())
	bt, err := json.Marshal(event.UploadVideoEvent{Video: payload})
	assert.NoError(t, err)

	_, err = h(bt, s{})
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(repo.stored), "data:video/quicktime;base64,"))
}

func TestUploadVideo_RejectsNonVideoDataURL(t *testing.T) {
	h := StreamUploadVideoHandler(n{}, &videoRepoStub{}, u{})

	payload := "data:text/html;base64," + base64.StdEncoding.EncodeToString(minimalMP4())
	bt, err := json.Marshal(event.UploadVideoEvent{Video: payload})
	assert.NoError(t, err)

	_, err = h(bt, s{})
	assert.ErrorIs(t, err, ErrUnsupportedVideo)
}

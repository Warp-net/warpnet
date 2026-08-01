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
	"strings"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
)

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

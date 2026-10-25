// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package media_meta

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jpegWithOrientation(t *testing.T, img image.Image, orientation int) []byte {
	t.Helper()

	segments, err := parseJPEGSegments(encodeJPEG(t, img, 100))
	require.NoError(t, err)

	ifdMapping, err := exifcommon.NewIfdMappingWithStandard()
	require.NoError(t, err)
	tagIndex := exif.NewTagIndex()
	require.NoError(t, exif.LoadStandardTags(tagIndex))

	rootIb := exif.NewIfdBuilder(ifdMapping, tagIndex, exifcommon.IfdStandardIfdIdentity, exifcommon.EncodeDefaultByteOrder)
	require.NoError(t, rootIb.SetStandardWithName(orientationTag, []uint16{uint16(orientation)}))
	require.NoError(t, segments.SetExif(rootIb))

	var buf bytes.Buffer
	require.NoError(t, segments.Write(&buf))
	return buf.Bytes()
}

func letterRows(img image.Image) []string {
	bounds := img.Bounds()
	rows := make([]string, 0, bounds.Dy())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		row := make([]byte, 0, bounds.Dx())
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			row = append(row, byte(r>>8))
		}
		rows = append(rows, string(row))
	}
	return rows
}

func TestApplyOrientation_TurnsThePictureUpright(t *testing.T) {
	stored := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i, letter := range "abcdef" {
		stored.SetRGBA(i%3, i/3, color.RGBA{R: uint8(letter), A: 0xff})
	}

	cases := map[int][]string{
		orientationFlipX:          {"cba", "fed"},
		orientationTurn180:        {"fed", "cba"},
		orientationFlipY:          {"def", "abc"},
		orientationFlipXTurnLeft:  {"ad", "be", "cf"},
		orientationTurnRight:      {"da", "eb", "fc"},
		orientationFlipXTurnRight: {"fc", "eb", "da"},
		orientationTurnLeft:       {"cf", "be", "ad"},
	}
	for orientation, want := range cases {
		assert.Equal(t, want, letterRows(applyOrientation(stored, orientation)), "orientation %d", orientation)
	}

	for _, orientation := range []int{0, 1, 9} {
		assert.Same(t, stored, applyOrientation(stored, orientation))
	}
}

func TestSignUploadedImage_TurnsAPhoneShotUpright(t *testing.T) {
	stored := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for y := range 32 {
		for x := range 64 {
			stored.SetRGBA(x, y, color.RGBA{A: 0xff})
			if x >= 32 {
				stored.SetRGBA(x, y, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
			}
		}
	}
	photo := jpegWithOrientation(t, stored, orientationTurnRight)
	require.Equal(t, orientationTurnRight, extractOrientation(photo))

	meta, err := BuildMetadata(
		warpnet.NodeInfo{ID: signerID, OwnerId: "alice"},
		signerKey,
		domain.User{Id: "alice", NodeId: signerID.String()},
	)
	require.NoError(t, err)

	signed, err := SignUploadedImage(ImagePrefix+base64.StdEncoding.EncodeToString(photo), meta)
	require.NoError(t, err)
	_, raw, err := SplitDataURI(string(signed))
	require.NoError(t, err)

	upright := decodeJPEG(t, raw)
	require.Equal(t, image.Pt(32, 64), upright.Bounds().Size())
	top, _, _, _ := upright.At(16, 8).RGBA()
	bottom, _, _, _ := upright.At(16, 56).RGBA()
	assert.Less(t, top>>8, uint32(0x10))
	assert.Greater(t, bottom>>8, uint32(0xf0))
	assert.Zero(t, extractOrientation(raw))
}

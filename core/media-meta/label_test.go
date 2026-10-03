// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package media_meta

import (
	"bytes"
	"encoding/hex"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/Warp-net/warpnet/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func uniformJPEG(t *testing.T, size image.Point, gray uint8) []byte {
	t.Helper()
	img := image.NewGray(image.Rectangle{Max: size})
	for i := range img.Pix {
		img.Pix[i] = gray
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}))
	return buf.Bytes()
}

func markedShare(img image.Image, r image.Rectangle, gray uint8) float64 {
	var marked int
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if diff := int(color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y) - int(gray); diff > 16 || diff < -16 {
				marked++
			}
		}
	}
	return float64(marked) / float64(r.Dx()*r.Dy())
}

func TestDrawLabels_CoversEveryQuarter(t *testing.T) {
	size := image.Pt(640, 480)
	plain := uniformJPEG(t, size, 0x80)
	label, err := LabelPNG(plain, "@leaker · 01M3ZEY4M40NH3H9JP8E2DVCZH")
	require.NoError(t, err)

	drawn, err := DrawLabels(plain, label)
	require.NoError(t, err)
	img, err := jpeg.Decode(bytes.NewReader(drawn))
	require.NoError(t, err)

	half := size.Div(2)
	for _, quarter := range []image.Rectangle{
		image.Rect(0, 0, half.X, half.Y), image.Rect(half.X, 0, size.X, half.Y),
		image.Rect(0, half.Y, half.X, size.Y), image.Rect(half.X, half.Y, size.X, size.Y),
	} {
		assert.Greater(t, markedShare(img, quarter, 0x80), 0.02, "no crop leaves %v clean", quarter)
	}
}

func TestDrawLabels_IsTheSameOnEveryMachine(t *testing.T) {
	stripes := image.NewGray(image.Rect(0, 0, 120, 16))
	for i := range stripes.Pix {
		stripes.Pix[i] = uint8(i * 37)
	}
	var mask bytes.Buffer
	require.NoError(t, png.Encode(&mask, stripes))

	drawn, err := DrawLabels(uniformJPEG(t, image.Pt(320, 200), 0x30), mask.Bytes())
	require.NoError(t, err)
	assert.Equal(t, "cdb205bda3e1f47e0aaf28795210512368c8044569dd1aa837a1c50f7f7a41a3", hex.EncodeToString(security.ConvertToSHA256(drawn)), "a copy is rebuilt on request, so its bytes and key must never drift")
}

func TestDrawLabels_RefusesAColourLabel(t *testing.T) {
	var colour bytes.Buffer
	require.NoError(t, png.Encode(&colour, image.NewRGBA(image.Rect(0, 0, 4, 4))))

	_, err := DrawLabels(uniformJPEG(t, image.Pt(32, 32), 0x30), colour.Bytes())
	assert.ErrorIs(t, err, ErrBadLabel)
}

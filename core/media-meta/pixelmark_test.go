// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package media_meta

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"strconv"
	"testing"

	"github.com/Warp-net/warpnet/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	xdraw "golang.org/x/image/draw"
)

func testPixelMarkSeed(n int) []byte {
	seed := sha256.Sum256([]byte("seed " + strconv.Itoa(n)))
	return seed[:]
}

func photoJPEG(t *testing.T, size image.Point) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rectangle{Max: size})
	for y := range size.Y {
		for x := range size.X {
			fx, fy := float64(x), float64(y)
			v := 110 + 60*math.Sin(fx/97) + 35*math.Cos(fy/53+fx/211)
			if dx, dy := fx-float64(size.X)/3, fy-float64(size.Y)/2; dx*dx+dy*dy < 90*90 {
				v = 210 - 0.2*fy
			}
			if x > size.X*2/3 && y%40 < 18 {
				v -= 70
			}
			v += float64((x*7919+y*104729)%11) - 5
			c := uint8(min(max(v, 0), 255))
			img.Set(x, y, color.RGBA{R: c, G: uint8(min(max(v*0.8+30, 0), 255)), B: uint8(min(max(255-v*0.6, 0), 255)), A: 0xff})
		}
	}
	return encodeJPEG(t, img, 95)
}

func encodeJPEG(t *testing.T, img image.Image, quality int) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}))
	return buf.Bytes()
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func decodeJPEG(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(b))
	require.NoError(t, err)
	return img
}

func scaleImage(img image.Image, size image.Point) *image.RGBA {
	dst := image.NewRGBA(image.Rectangle{Max: size})
	xdraw.BiLinear.Scale(dst, dst.Rect, img, img.Bounds(), xdraw.Src, nil)
	return dst
}

func cropImage(img image.Image, r image.Rectangle) *image.RGBA {
	dst := image.NewRGBA(image.Rectangle{Max: r.Size()})
	xdraw.Copy(dst, image.Point{}, img, r, xdraw.Src, nil)
	return dst
}

func TestDrawWatermark_PixelMarkIsTheSameOnEveryMachine(t *testing.T) {
	stripes := image.NewGray(image.Rect(0, 0, 120, 16))
	for i := range stripes.Pix {
		stripes.Pix[i] = uint8(i * 37)
	}
	var mask bytes.Buffer
	require.NoError(t, png.Encode(&mask, stripes))

	drawn, err := DrawWatermark(uniformJPEG(t, image.Pt(320, 200), 0x30), mask.Bytes(), testPixelMarkSeed(1))
	require.NoError(t, err)
	assert.Equal(t, "19f8d4f14234ea665f87d40c1df8c00c5a1540565a0e449e6c6319e41e999ae5", hex.EncodeToString(security.ConvertToSHA256(drawn)), "a copy is rebuilt on request, so its bytes and key must never drift")
}

func TestDrawWatermark_PixelMarkIsFaint(t *testing.T) {
	plain := uniformJPEG(t, image.Pt(64, 48), 0x80)
	stripes := image.NewGray(image.Rect(0, 0, 1, 1))
	var mask bytes.Buffer
	require.NoError(t, png.Encode(&mask, stripes))

	drawn, err := DrawWatermark(plain, mask.Bytes(), testPixelMarkSeed(1))
	require.NoError(t, err)
	img := decodeJPEG(t, drawn)
	var changed int
	for y := range 48 {
		for x := range 64 {
			diff := int(color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y) - 0x80
			assert.LessOrEqual(t, diff*diff, 4, "the pixel mark is invisible")
			if diff != 0 {
				changed++
			}
		}
	}
	assert.Greater(t, changed, 64*48/4, "the pixel mark covers the image")
}

func TestScorePixelMark_NamesTheBuyer(t *testing.T) {
	size := image.Pt(512, 342)
	original := photoJPEG(t, size)
	watermark, err := WatermarkPNG(original, "@leaker · 01M3ZEY4M40NH3H9JP8E2DVCZH")
	require.NoError(t, err)
	other, err := WatermarkPNG(original, "@other · 01M3ZEY4M40NH3H9JP8E2DVCZK")
	require.NoError(t, err)
	copied, err := DrawWatermark(original, watermark, testPixelMarkSeed(1))
	require.NoError(t, err)
	otherCopy, err := DrawWatermark(original, other, testPixelMarkSeed(2))
	require.NoError(t, err)
	img, otherImg := decodeJPEG(t, copied), decodeJPEG(t, otherCopy)

	averaged := image.NewRGBA(img.Bounds())
	for y := range size.Y {
		for x := range size.X {
			a, b := img.At(x, y).(color.YCbCr), otherImg.At(x, y).(color.YCbCr)
			ar, ag, ab := color.YCbCrToRGB(a.Y, a.Cb, a.Cr)
			br, bg, bb := color.YCbCrToRGB(b.Y, b.Cb, b.Cr)
			averaged.Set(x, y, color.RGBA{R: uint8((int(ar) + int(br)) / 2), G: uint8((int(ag) + int(bg)) / 2), B: uint8((int(ab) + int(bb)) / 2), A: 0xff})
		}
	}
	screen := image.NewRGBA(image.Rect(0, 0, 900, 700))
	xdraw.Draw(screen, screen.Rect, image.NewUniform(color.RGBA{R: 31, G: 26, B: 23, A: 0xff}), image.Point{}, xdraw.Src)
	for i := range 12 {
		bar := image.Rect(20+i*61%700, 10+i*53%650, 120+i*61%700, 18+i*53%650)
		xdraw.Draw(screen, bar, image.NewUniform(color.RGBA{R: 190, G: 170, B: 140, A: 0xff}), image.Point{}, xdraw.Src)
	}
	shown := scaleImage(img, image.Pt(410, 274))
	xdraw.Draw(screen, shown.Rect.Add(image.Pt(250, 120)), shown, image.Point{}, xdraw.Src)

	seeds := make([][]byte, 52)
	for i := range seeds {
		seeds[i] = testPixelMarkSeed(i + 1)
	}
	for name, suspect := range map[string][]byte{
		"as served":            copied,
		"jpeg q75":             encodeJPEG(t, img, 75),
		"half size png":        encodePNG(t, scaleImage(img, size.Div(2))),
		"centre crop":          encodePNG(t, cropImage(img, image.Rect(51, 34, 461, 308))),
		"ui screenshot jpeg80": encodeJPEG(t, screen, 80),
	} {
		scores, err := ScorePixelMark(original, suspect, seeds)
		require.NoError(t, err, name)
		assert.GreaterOrEqual(t, scores[0], float64(MinPixelMarkScore), "%s names the buyer", name)
		for i, score := range scores[1:] {
			assert.Less(t, math.Abs(score), 5.0, "%s names nobody else (seed %d)", name, i+1)
		}
	}

	scores, err := ScorePixelMark(original, encodeJPEG(t, averaged, 90), seeds[:2])
	require.NoError(t, err)
	assert.GreaterOrEqual(t, scores[0], float64(MinPixelMarkScore), "two copies averaged still name the first buyer")
	assert.GreaterOrEqual(t, scores[1], float64(MinPixelMarkScore), "two copies averaged still name the second buyer")

	scores, err = ScorePixelMark(original, original, seeds)
	require.NoError(t, err)
	for _, score := range scores {
		assert.Less(t, math.Abs(score), 5.0, "the original names no buyer")
	}
}

var quarterTurnOrientations = []int{1, orientationTurnRight, orientationTurn180, orientationTurnLeft}

func pixelMarkFixture(t *testing.T, size image.Point) ([]byte, image.Image, [][]byte) {
	t.Helper()
	original := photoJPEG(t, size)
	watermark, err := WatermarkPNG(original, "01M3ZEY4M40NH3H9JP8E2DVCZH")
	require.NoError(t, err)
	copied, err := DrawWatermark(original, watermark, testPixelMarkSeed(1))
	require.NoError(t, err)
	seeds := make([][]byte, 30)
	for i := range seeds {
		seeds[i] = testPixelMarkSeed(i + 1)
	}
	return original, decodeJPEG(t, copied), seeds
}

func uiScreenshot(img image.Image, shown image.Point) *image.RGBA {
	screen := image.NewRGBA(image.Rectangle{Max: shown.Add(image.Pt(160, 120))})
	xdraw.Draw(screen, screen.Rect, image.NewUniform(color.RGBA{R: 31, G: 26, B: 23, A: 0xff}), image.Point{}, xdraw.Src)
	for i := range 12 {
		at := image.Pt(i*61%screen.Rect.Dx(), i*53%screen.Rect.Dy())
		xdraw.Draw(screen, image.Rectangle{Min: at, Max: at.Add(image.Pt(100, 8))}, image.NewUniform(color.RGBA{R: 190, G: 170, B: 140, A: 0xff}), image.Point{}, xdraw.Src)
	}
	scaled := scaleImage(img, shown)
	xdraw.Draw(screen, scaled.Rect.Add(image.Pt(80, 60)), scaled, image.Point{}, xdraw.Src)
	return screen
}

func assertNamesOnlyTheBuyer(t *testing.T, scores []float64, name string) {
	t.Helper()
	assert.GreaterOrEqual(t, scores[0], float64(MinPixelMarkScore), "%s names the buyer", name)
	for i, score := range scores[1:] {
		assert.Less(t, math.Abs(score), 5.0, "%s names nobody else (seed %d)", name, i+2)
	}
}

func TestScorePixelMark_NamesTheBuyerInEveryTurn(t *testing.T) {
	t.Parallel()
	original, img, seeds := pixelMarkFixture(t, image.Pt(512, 342))
	leaks := map[string]func(image.Image) []byte{
		"the whole copy": func(turned image.Image) []byte { return encodePNG(t, turned) },
		"ui screenshot jpeg80": func(turned image.Image) []byte {
			return encodeJPEG(t, uiScreenshot(turned, turned.Bounds().Size().Mul(4).Div(5)), 80)
		},
	}
	for name, leak := range leaks {
		for _, orientation := range quarterTurnOrientations {
			t.Run(fmt.Sprintf("%s turned by orientation %d", name, orientation), func(t *testing.T) {
				t.Parallel()
				scores, err := ScorePixelMark(original, leak(applyOrientation(img, orientation)), seeds)
				require.NoError(t, err)
				assertNamesOnlyTheBuyer(t, scores, name)
			})
		}
	}
}

func TestScorePixelMark_NamesTheBuyerOfASquareCopyInEveryTurn(t *testing.T) {
	original, img, seeds := pixelMarkFixture(t, image.Pt(320, 320))
	for _, orientation := range quarterTurnOrientations {
		scores, err := ScorePixelMark(original, encodePNG(t, applyOrientation(img, orientation)), seeds)
		require.NoError(t, err)
		assertNamesOnlyTheBuyer(t, scores, fmt.Sprintf("a square copy turned by orientation %d", orientation))
	}
}

func TestScorePixelMark_TheOriginalNamesNobodyWhenTurned(t *testing.T) {
	t.Parallel()
	original, _, seeds := pixelMarkFixture(t, image.Pt(512, 342))
	plain := decodeJPEG(t, original)
	for _, orientation := range []int{orientationTurnRight, orientationTurn180} {
		t.Run(fmt.Sprintf("orientation %d", orientation), func(t *testing.T) {
			t.Parallel()
			scores, err := ScorePixelMark(original, encodePNG(t, applyOrientation(plain, orientation)), seeds)
			require.NoError(t, err)
			for i, score := range scores {
				assert.Less(t, math.Abs(score), 5.0, "the original turned by orientation %d names nobody (seed %d)", orientation, i+1)
			}
		})
	}
}

func TestScorePixelMark_AnUnrelatedPictureNamesNobody(t *testing.T) {
	t.Parallel()
	original, _, seeds := pixelMarkFixture(t, image.Pt(512, 342))
	for name, suspect := range map[string][]byte{
		"a flat picture": uniformJPEG(t, image.Pt(200, 150), 0x60),
		"another photo":  encodePNG(t, decodeJPEG(t, photoJPEG(t, image.Pt(240, 400)))),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			scores, err := ScorePixelMark(original, suspect, seeds)
			require.NoError(t, err)
			for i, score := range scores {
				assert.Less(t, math.Abs(score), 5.0, "%s names nobody (seed %d)", name, i+1)
			}
		})
	}
}

func TestScorePixelMark_DoesNothingWithoutSales(t *testing.T) {
	scores, err := ScorePixelMark([]byte("not an image"), []byte("not an image"), nil)
	assert.NoError(t, err)
	assert.Nil(t, scores)
}

func TestScorePixelMark_RefusesFilesItCannotRead(t *testing.T) {
	original, img, seeds := pixelMarkFixture(t, image.Pt(64, 48))
	_, err := ScorePixelMark(original, []byte("not an image"), seeds)
	assert.Error(t, err)
	_, err = ScorePixelMark([]byte("not an image"), encodePNG(t, img), seeds)
	assert.Error(t, err)
}

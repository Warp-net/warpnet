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

package media_meta

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"

	"github.com/Warp-net/warpnet/core/warpnet"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	watermarkFontDivisor = 24
	watermarkMinFontSize = 12
	watermarkMaxFontSize = 320
	watermarkDPI         = 72

	rotateCos = 58618
	rotateSin = 29309

	ErrBadWatermark warpnet.WarpError = "copy watermark is not a grayscale mask"
)

var (
	watermarkColor  = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x55}
	watermarkShadow = color.NRGBA{A: 0x44}
)

func WatermarkPNG(jpegBytes []byte, text string) ([]byte, error) {
	config, err := jpeg.DecodeConfig(bytes.NewReader(jpegBytes))
	if err != nil {
		return nil, err
	}
	mask, err := textMask(text, watermarkFontSize(image.Pt(config.Width, config.Height)))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, &image.Gray{Pix: mask.Pix, Stride: mask.Stride, Rect: mask.Rect}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func DrawWatermark(jpegBytes, watermark []byte) ([]byte, error) {
	src, err := jpeg.Decode(bytes.NewReader(jpegBytes))
	if err != nil {
		return nil, err
	}
	decoded, err := png.Decode(bytes.NewReader(watermark))
	if err != nil {
		return nil, err
	}
	gray, ok := decoded.(*image.Gray)
	if !ok {
		return nil, ErrBadWatermark
	}
	mask := rotate(&image.Alpha{Pix: gray.Pix, Stride: gray.Stride, Rect: gray.Rect})
	bounds := src.Bounds()
	fontSize := watermarkFontSize(bounds.Size())

	canvas := image.NewRGBA(bounds)
	draw.Draw(canvas, bounds, src, bounds.Min, draw.Src)

	size := mask.Rect.Size()
	shadow := image.Pt(1, 1).Mul(max(fontSize/16, 1))
	along := image.Pt(2, -1).Mul(size.X/2 + size.Y/4)
	across := image.Pt(1, 2).Mul(max(2*size.Y-size.X, 1))
	reach := (bounds.Dx()+bounds.Dy())/min(-along.Y, across.X) + 2
	for i := -reach; i <= reach; i++ {
		for j := -reach; j <= reach; j++ {
			at := bounds.Min.Add(along.Mul(i)).Add(across.Mul(j))
			place := image.Rectangle{Min: at, Max: at.Add(size)}
			draw.DrawMask(canvas, place.Add(shadow), image.NewUniform(watermarkShadow), image.Point{}, mask, image.Point{}, draw.Over)
			draw.DrawMask(canvas, place, image.NewUniform(watermarkColor), image.Point{}, mask, image.Point{}, draw.Over)
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: 100}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func watermarkFontSize(size image.Point) int {
	return min(max(min(size.X, size.Y)/watermarkFontDivisor, watermarkMinFontSize), watermarkMaxFontSize)
}

func textMask(text string, fontSize int) (*image.Alpha, error) {
	ttf, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(ttf, &opentype.FaceOptions{Size: float64(fontSize), DPI: watermarkDPI})
	if err != nil {
		return nil, err
	}
	defer func() { _ = face.Close() }()

	metrics := face.Metrics()
	mask := image.NewAlpha(image.Rect(0, 0, font.MeasureString(face, text).Ceil(), metrics.Height.Ceil()))
	drawer := font.Drawer{Dst: mask, Src: image.Opaque, Face: face, Dot: fixed.P(0, metrics.Ascent.Ceil())}
	drawer.DrawString(text)
	return mask, nil
}

func rotate(src *image.Alpha) *image.Alpha {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	lift := (w*rotateSin + 0xffff) >> 16
	dst := image.NewAlpha(image.Rect(0, 0, (w*rotateCos+h*rotateSin+0xffff)>>16, lift+(h*rotateCos+0xffff)>>16))
	for y := range dst.Rect.Dy() {
		for x := range dst.Rect.Dx() {
			sx := x*rotateCos - (y-lift)*rotateSin
			sy := x*rotateSin + (y-lift)*rotateCos
			dst.Pix[y*dst.Stride+x] = blendPixel(src, sx>>8, sy>>8)
		}
	}
	return dst
}

func blendPixel(src *image.Alpha, x, y int) uint8 {
	x0, y0, fx, fy := x>>8, y>>8, x&0xff, y&0xff
	top := alphaAt(src, x0, y0)*(0x100-fx) + alphaAt(src, x0+1, y0)*fx
	bottom := alphaAt(src, x0, y0+1)*(0x100-fx) + alphaAt(src, x0+1, y0+1)*fx
	return uint8((top*(0x100-fy) + bottom*fy) >> 16) //nolint:gosec // at most 0xff
}

func alphaAt(src *image.Alpha, x, y int) int {
	if !image.Pt(x, y).In(src.Rect) {
		return 0
	}
	return int(src.Pix[y*src.Stride+x])
}

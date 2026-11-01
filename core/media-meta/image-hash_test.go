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
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"testing"

	xdraw "golang.org/x/image/draw"
)

func patternPhoto(t *testing.T, seed int, width, height int) image.Image {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	phase := float64(seed)
	for y := range height {
		for x := range width {
			fx, fy := float64(x)/float64(width), float64(y)/float64(height)
			v := 128 + 60*math.Sin(7*fx+phase) + 45*math.Cos(5*fy+2*phase) + 20*math.Sin(9*(fx+fy)+3*phase)
			img.Set(x, y, color.RGBA{R: uint8(v), G: uint8(v * 0.8), B: uint8(255 - v), A: 0xff}) //nolint:gosec // v stays within 0..255
		}
	}
	return img
}

func toJPEGURI(t *testing.T, img image.Image, quality int) string {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatal(err)
	}
	return ImagePrefix + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestHashImageSurvivesReEncodingAndScaling(t *testing.T) {
	photo := patternPhoto(t, 1, 480, 320)
	hash, _, err := HashImage(toJPEGURI(t, photo, 100))
	if err != nil {
		t.Fatal(err)
	}

	small := image.NewRGBA(image.Rect(0, 0, 160, 107))
	xdraw.ApproxBiLinear.Scale(small, small.Rect, photo, photo.Bounds(), xdraw.Src, nil)
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, photo); err != nil {
		t.Fatal(err)
	}
	for name, file := range map[string]string{
		"jpeg q50": toJPEGURI(t, photo, 50),
		"a third":  toJPEGURI(t, small, 80),
		"png":      "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBuf.Bytes()),
	} {
		got, _, err := HashImage(file)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !IsSameImage(hash, got) {
			t.Fatalf("%s: %s is not taken for %s", name, got, hash)
		}
	}

	other, _, err := HashImage(toJPEGURI(t, patternPhoto(t, 9, 480, 320), 100))
	if err != nil {
		t.Fatal(err)
	}
	if IsSameImage(hash, other) {
		t.Fatalf("another photo %s passes for %s", other, hash)
	}
}

func TestHashImageCatchesAMirroredCopy(t *testing.T) {
	photo := patternPhoto(t, 2, 300, 200)
	hash, _, err := HashImage(toJPEGURI(t, photo, 100))
	if err != nil {
		t.Fatal(err)
	}
	mirror := image.NewRGBA(photo.Bounds())
	for y := range 200 {
		for x := range 300 {
			mirror.Set(299-x, y, photo.At(x, y))
		}
	}
	straight, mirrored, err := HashImage(toJPEGURI(t, mirror, 100))
	if err != nil {
		t.Fatal(err)
	}
	if IsSameImage(hash, straight) {
		t.Fatal("a plain hash should not see through a mirror, the mirrored one is for that")
	}
	if !IsSameImage(hash, mirrored) {
		t.Fatalf("the mirrored hash %s of the mirrored copy is not taken for %s", mirrored, hash)
	}
}

func TestIsSameImageIgnoresFlatAndBrokenHashes(t *testing.T) {
	flat := image.NewUniform(color.RGBA{R: 40, G: 90, B: 160, A: 0xff})
	plain := image.NewRGBA(image.Rect(0, 0, 90, 80))
	xdraw.Draw(plain, plain.Rect, flat, image.Point{}, xdraw.Src)
	hash, _, err := HashImage(toJPEGURI(t, plain, 100))
	if err != nil {
		t.Fatal(err)
	}
	if IsSameImage(hash, hash) {
		t.Fatalf("a flat picture's hash %s says nothing and must not match even itself", hash)
	}
	for _, broken := range []string{"", "zz", "123456789012345678"} {
		if IsSameImage(broken, broken) {
			t.Fatalf("%q matched", broken)
		}
	}
	if _, _, err := HashImage("data:image/jpeg;base64,AAAA"); err == nil {
		t.Fatal("bytes that are no picture hash to nothing")
	}
}

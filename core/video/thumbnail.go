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

package video

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
)

const thumbnailWidth = 160

type thumbnails struct {
	width, height int
	frames        [][]byte
}

func thumbnailSize(display image.Point) image.Point {
	if display.X <= 0 || display.Y <= 0 {
		return image.Pt(thumbnailWidth, thumbnailWidth)
	}
	height := max(2*((thumbnailWidth*display.Y+display.X)/(2*display.X)), 2) //nolint:mnd
	return image.Pt(thumbnailWidth, height)
}

func (f *FFmpeg) decodeThumbnails(ctx context.Context, input string, t track, size image.Point, threads int) (thumbnails, error) {
	coded := turnedRect(size.X, size.Y, t.rotation).Size()
	output := input + ".gray"
	err := f.run(ctx,
		"-noautorotate", "-threads", strconv.Itoa(threads), "-i", input,
		"-map", "0:v:0", "-an", "-sn", "-dn",
		"-vf", "scale="+strconv.Itoa(coded.X)+":"+strconv.Itoa(coded.Y)+":flags=fast_bilinear,format=gray",
		"-fps_mode", "passthrough", "-f", "rawvideo", "-y", output,
	)
	if err != nil {
		return thumbnails{}, err
	}
	defer func() { _ = os.Remove(output) }()
	raw, err := os.ReadFile(filepath.Clean(output))
	if err != nil {
		return thumbnails{}, err
	}
	frameSize := coded.X * coded.Y
	thumbs := thumbnails{width: size.X, height: size.Y, frames: make([][]byte, 0, len(raw)/frameSize)}
	for off := 0; off+frameSize <= len(raw); off += frameSize {
		thumbs.frames = append(thumbs.frames, turnGray(raw[off:off+frameSize], coded.X, coded.Y, t.rotation))
	}
	return thumbs, nil
}

func turnGray(pix []byte, w, h, degrees int) []byte {
	if degrees == 0 {
		return pix
	}
	out := make([]byte, len(pix))
	turned := turnedRect(w, h, degrees)
	for y := range h {
		for x := range w {
			tx, ty := turnPoint(x, y, w, h, degrees)
			out[ty*turned.Dx()+tx] = pix[y*w+x]
		}
	}
	return out
}

func (t thumbnails) encodePNG(i int) ([]byte, error) {
	img := &image.Gray{Pix: t.frames[i], Stride: t.width, Rect: image.Rect(0, 0, t.width, t.height)}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeThumbnail(b []byte) (image.Point, []byte, error) {
	decoded, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return image.Point{}, nil, err
	}
	gray, ok := decoded.(*image.Gray)
	if !ok || gray.Stride != gray.Rect.Dx() {
		return image.Point{}, nil, ErrUnsupportedVideo
	}
	return gray.Rect.Size(), gray.Pix, nil
}

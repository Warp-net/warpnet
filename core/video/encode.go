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
	"crypto/sha256"
	"encoding/binary"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
)

const (
	frameQuality  = "23"
	encodeTimeout = 3 * time.Minute

	ErrNoFramePlaces warpnet.WarpError = "video: no keyframe can take a frame"
)

func PickFrames(b []byte, seed []byte, count int) ([]int, error) {
	m, err := parseMovie(b)
	if err != nil {
		return nil, err
	}
	places := m.video.findFramePlaces()
	if len(places) == 0 {
		return nil, ErrNoFramePlaces
	}
	picked := make([]int, 0, count)
	for i := 0; len(places) > 0 && len(picked) < count; i++ {
		digest := sha256.Sum256(binary.BigEndian.AppendUint64(slices.Clone(seed), uint64(i))) //nolint:gosec // a counter
		at := int(binary.BigEndian.Uint64(digest[:8]) % uint64(len(places)))                  //nolint:gosec // bounded by the slice length
		picked = append(picked, places[at])
		places = slices.Delete(places, at, at+1)
	}
	slices.Sort(picked)
	return picked, nil
}

func (t *track) findFramePlaces() []int {
	if t.hasEmptyEdit {
		return nil
	}
	places := make([]int, 0, 32) //nolint:mnd
	isFirstKeyframe := true
	latestAt := -1
	for i, s := range t.samples {
		if s.isSync && !isFirstKeyframe && latestAt >= 0 {
			if _, err := t.placeFrame(t.samples[latestAt], s); err == nil {
				places = append(places, i)
			}
		}
		if s.isSync {
			isFirstKeyframe = false
		}
		if latestAt < 0 || s.pts() >= t.samples[latestAt].pts() {
			latestAt = i
		}
	}
	return places
}

func (t *track) findSeekTime(index int) string {
	latestPTS := int64(math.MinInt64)
	for _, s := range t.samples[:index] {
		latestPTS = max(latestPTS, s.pts())
	}
	pts := t.samples[index].pts()
	at := float64(pts-t.mediaTime-(pts-latestPTS)/2) / float64(t.timescale) //nolint:mnd
	return strconv.FormatFloat(max(at, 0), 'f', 6, 64)                      //nolint:mnd
}

func (f *FFmpeg) EncodeFrames(b []byte, samples []int, watermark []byte) ([]domain.VideoFrame, error) {
	m, err := parseMovie(b)
	if err != nil {
		return nil, err
	}
	t := m.video
	if t.hasEmptyEdit {
		return nil, ErrUnsupportedVideo
	}
	config, err := parseAVCConfig(t.avcC.head)
	if err != nil {
		return nil, err
	}
	profile, err := findX264Profile(config.profile)
	if err != nil {
		return nil, err
	}
	ids, err := config.findFreeIds(len(samples))
	if err != nil {
		return nil, err
	}
	overlay, err := turnPNG(watermark, (360-t.rotation)%360) //nolint:mnd
	if err != nil {
		return nil, err
	}

	dir, err := makeWorkDir()
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	original, overlayPath := filepath.Join(dir, "original.mp4"), filepath.Join(dir, "watermark.png")
	if err := os.WriteFile(original, b, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(overlayPath, overlay, 0o600); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), encodeTimeout)
	defer cancel()
	frames := make([]domain.VideoFrame, 0, len(samples))
	for i, index := range samples {
		if index <= 0 || index >= len(t.samples) || !t.samples[index].isSync {
			return nil, ErrBadFramePlace
		}
		frame, err := f.encodeFrame(ctx, dir, t, config, index, ids[i], profile)
		if err != nil {
			return nil, err
		}
		frames = append(frames, frame)
	}
	return frames, nil
}

func (f *FFmpeg) encodeFrame(
	ctx context.Context, dir string, t track, config avcConfig, index, id int, profile string,
) (domain.VideoFrame, error) {
	output := filepath.Join(dir, "frame-"+strconv.Itoa(index)+".mp4")
	err := f.run(ctx,
		"-threads", "1", "-noautorotate", "-ss", t.findSeekTime(index),
		"-i", filepath.Join(dir, "original.mp4"), "-i", filepath.Join(dir, "watermark.png"),
		"-filter_complex", "[0:v][1:v]overlay=0:0[v]", "-filter_threads", "1",
		"-map", "[v]", "-frames:v", "1", "-an", "-sn", "-dn",
		"-c:v", "libx264", "-profile:v", profile, "-crf", frameQuality,
		"-x264-params", "sps-id="+strconv.Itoa(id), "-threads", "1",
		"-f", "mp4", "-y", output,
	)
	if err != nil {
		return domain.VideoFrame{}, err
	}
	encoded, err := os.ReadFile(output) //nolint:gosec // our own work directory
	if err != nil {
		return domain.VideoFrame{}, err
	}
	frame, err := extractFrame(encoded, config.lengthSize, id)
	if err != nil {
		return domain.VideoFrame{}, err
	}
	frame.SampleIndex = index

	thumbnails, err := f.decodeThumbnails(ctx, output, t, thumbnailSize(t.displaySize()), 1)
	if err != nil {
		return domain.VideoFrame{}, err
	}
	if len(thumbnails.frames) == 0 {
		return domain.VideoFrame{}, ErrUnsupportedVideo
	}
	if frame.Thumbnail, err = thumbnails.encodePNG(0); err != nil {
		return domain.VideoFrame{}, err
	}
	return frame, nil
}

func extractFrame(b []byte, lengthSize, id int) (domain.VideoFrame, error) {
	m, err := parseMovie(b)
	if err != nil {
		return domain.VideoFrame{}, err
	}
	config, err := parseAVCConfig(m.video.avcC.head)
	if err != nil {
		return domain.VideoFrame{}, err
	}
	if len(config.sps) != 1 || len(config.pps) != 1 || len(m.video.samples) == 0 || !m.video.samples[0].isSync {
		return domain.VideoFrame{}, ErrUnsupportedVideo
	}
	if spsId, err := readSPSId(config.sps[0]); err != nil || spsId != id {
		return domain.VideoFrame{}, ErrUnsupportedVideo
	}
	s := m.video.samples[0]
	data, err := convertNALs(b[s.offset:s.offset+s.size], config.lengthSize, lengthSize)
	if err != nil {
		return domain.VideoFrame{}, err
	}
	return domain.VideoFrame{SPS: config.sps[0], PPS: config.pps[0], Data: data}, nil
}

func findX264Profile(profile byte) (string, error) {
	switch profile {
	case 66: //nolint:mnd
		return "baseline", nil
	case 77: //nolint:mnd
		return "main", nil
	case 100: //nolint:mnd
		return "high", nil
	}
	return "", ErrUnsupportedVideo
}

func DisplaySize(b []byte) (image.Point, error) {
	m, err := parseMovie(b)
	if err != nil {
		return image.Point{}, err
	}
	return m.video.displaySize(), nil
}

func turnPNG(b []byte, degrees int) ([]byte, error) {
	if degrees == 0 {
		return b, nil
	}
	decoded, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	src := image.NewNRGBA(decoded.Bounds())
	draw.Draw(src, src.Rect, decoded, decoded.Bounds().Min, draw.Src)
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewNRGBA(turnedRect(w, h, degrees))
	for y := range h {
		for x := range w {
			tx, ty := turnPoint(x, y, w, h, degrees)
			copy(dst.Pix[dst.PixOffset(tx, ty):dst.PixOffset(tx, ty)+4], src.Pix[src.PixOffset(x, y):src.PixOffset(x, y)+4])
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func turnedRect(w, h, degrees int) image.Rectangle {
	if degrees == 90 || degrees == 270 {
		return image.Rect(0, 0, h, w)
	}
	return image.Rect(0, 0, w, h)
}

func turnPoint(x, y, w, h, degrees int) (int, int) {
	switch degrees {
	case 90: //nolint:mnd
		return h - 1 - y, x
	case 180: //nolint:mnd
		return w - 1 - x, h - 1 - y
	case 270: //nolint:mnd
		return y, w - 1 - x
	}
	return x, y
}

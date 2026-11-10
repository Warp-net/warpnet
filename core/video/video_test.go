//nolint:all
package video

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Warp-net/warpnet/core/ratelimit"
	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestFFmpeg(t *testing.T) *FFmpeg {
	t.Helper()
	f := NewFFmpeg(ratelimit.NewSemaphore(1))
	if _, err := f.unpack(); errors.Is(err, ErrUnsupportedPlatform) {
		t.Skip("no ffmpeg for this platform")
	} else {
		require.NoError(t, err)
	}
	return f
}

func buildClip(t *testing.T, f *FFmpeg, size image.Point, frames, gop, bframes int) []byte {
	t.Helper()
	dir := t.TempDir()
	for i := range frames {
		img := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
		for y := range size.Y {
			for x := range size.X {
				v := uint8((x*7 + y*13 + i*5) % 256)
				if (x/16+y/16+i/4)%2 == 0 {
					v = 255 - v
				}
				img.Set(x, y, color.RGBA{v, uint8((x + i*3) % 256), uint8((y * 2) % 256), 255})
			}
		}
		var buf bytes.Buffer
		require.NoError(t, png.Encode(&buf, img))
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("frame%03d.png", i)), buf.Bytes(), 0o600))
	}
	out := filepath.Join(dir, "clip.mp4")
	require.NoError(t, f.run(context.Background(),
		"-framerate", "30", "-i", filepath.Join(dir, "frame%03d.png"),
		"-vf", "format=yuv420p", "-c:v", "libx264", "-profile:v", "high", "-crf", "20",
		"-x264-params", fmt.Sprintf("keyint=%d:min-keyint=%d:scenecut=0:bframes=%d", gop, gop, bframes),
		"-f", "mp4", "-y", out,
	))
	b, err := os.ReadFile(out)
	require.NoError(t, err)
	return b
}

func buildOverlay(t *testing.T, size image.Point, shift int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, size.X, size.Y))
	for y := range size.Y {
		for x := range size.X {
			if ((x+shift)/6+y/6)%3 == 0 {
				img.Set(x, y, color.NRGBA{255, 255, 255, 120})
			}
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func countFrames(t *testing.T, f *FFmpeg, b []byte) int {
	t.Helper()
	m, err := parseMovie(b)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "count.mp4")
	require.NoError(t, os.WriteFile(path, b, 0o600))
	thumbs, err := f.decodeThumbnails(context.Background(), path, m.video, image.Pt(32, 18), 1)
	require.NoError(t, err)
	return len(thumbs.frames)
}

func TestInsertFrames_TheCopyDecodesWithTheExtraFrames(t *testing.T) {
	f := newTestFFmpeg(t)
	for _, bframes := range []int{0, 2} {
		t.Run(fmt.Sprintf("bframes=%d", bframes), func(t *testing.T) {
			size := image.Pt(320, 180)
			original := buildClip(t, f, size, 90, 15, bframes)

			samples, err := PickFrames(original, []byte("seed"), 3)
			require.NoError(t, err)
			require.Len(t, samples, 3)

			frames, err := f.EncodeFrames(original, samples, buildOverlay(t, size, 0))
			require.NoError(t, err)
			copied, err := InsertFrames(original, frames)
			require.NoError(t, err)

			originalLength, err := Duration(original)
			require.NoError(t, err)
			copyLength, err := Duration(copied)
			require.NoError(t, err)
			assert.Equal(t, originalLength, copyLength)
			assert.Equal(t, countFrames(t, f, original)+3, countFrames(t, f, copied))

			again, err := InsertFrames(original, frames)
			require.NoError(t, err)
			assert.Equal(t, copied, again)
		})
	}
}

func TestScoreFrames_FindsTheBuyer(t *testing.T) {
	f := newTestFFmpeg(t)
	size := image.Pt(320, 180)
	original := buildClip(t, f, size, 120, 15, 0)

	sales := make([][]domain.VideoFrame, 0, 3)
	var sold []byte
	for i, seed := range []string{"alice", "bob", "carol"} {
		samples, err := PickFrames(original, []byte(seed), 3)
		require.NoError(t, err)
		frames, err := f.EncodeFrames(original, samples, buildOverlay(t, size, i*2))
		require.NoError(t, err)
		sales = append(sales, frames)
		if seed == "bob" {
			sold, err = InsertFrames(original, frames)
			require.NoError(t, err)
		}
	}

	scores, err := f.ScoreFrames(original, sold, sales)
	require.NoError(t, err)
	t.Logf("scores %v", scores)
	assert.Greater(t, scores[1].Score, MinFrameScore)
	assert.Equal(t, 3, scores[1].FramesCount)
	assert.Greater(t, scores[1].Score, scores[0].Score)
	assert.Greater(t, scores[1].Score, scores[2].Score)

	clean, err := f.ScoreFrames(original, original, sales)
	require.NoError(t, err)
	t.Logf("clean %v", clean)
	for _, score := range clean {
		assert.Less(t, score.Score, MinFrameScore)
	}
}

func TestScoreFrames_TellsBuyersApartAtTheSamePlaces(t *testing.T) {
	f := newTestFFmpeg(t)
	size := image.Pt(320, 180)
	original := buildClip(t, f, size, 120, 15, 0)

	samples, err := PickFrames(original, []byte("shared"), 3)
	require.NoError(t, err)
	sales := make([][]domain.VideoFrame, 0, 2)
	copies := make([][]byte, 0, 2)
	for _, shift := range []int{0, 3} {
		frames, err := f.EncodeFrames(original, samples, buildOverlay(t, size, shift))
		require.NoError(t, err)
		sold, err := InsertFrames(original, frames)
		require.NoError(t, err)
		sales = append(sales, frames)
		copies = append(copies, sold)
	}

	for buyer, sold := range copies {
		scores, err := f.ScoreFrames(original, sold, sales)
		require.NoError(t, err)
		t.Logf("buyer %d scores %v", buyer, scores)
		other := 1 - buyer
		assert.Equal(t, 3, scores[buyer].FramesCount)
		assert.Zero(t, scores[other].FramesCount)
		assert.GreaterOrEqual(t, scores[buyer].Score-scores[other].Score, MinFrameScore)
	}
}

func TestDisplaySize_RefusesAnOversizedTrackHeader(t *testing.T) {
	f := newTestFFmpeg(t)
	clip := buildClip(t, f, image.Pt(64, 36), 10, 5, 0)
	at := bytes.Index(clip, []byte("tkhd")) - 4
	require.GreaterOrEqual(t, at, 0)
	end := at + int(binary.BigEndian.Uint32(clip[at:]))

	size, err := DisplaySize(clip)
	require.NoError(t, err)
	assert.Equal(t, image.Pt(64, 36), size)

	for _, wh := range [][2]uint32{{65535, 65535}, {8193, 16}, {8192, 8192}, {0, 36}} {
		bad := slices.Clone(clip)
		binary.BigEndian.PutUint32(bad[end-8:], wh[0]<<16)
		binary.BigEndian.PutUint32(bad[end-4:], wh[1]<<16)
		_, err := DisplaySize(bad)
		assert.ErrorIs(t, err, ErrUnsupportedVideo, "%dx%d", wh[0], wh[1])
		_, err = PickFrames(bad, []byte("seed"), 3)
		assert.ErrorIs(t, err, ErrUnsupportedVideo, "%dx%d", wh[0], wh[1])
	}
}

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
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"image"
	"math"
	"slices"
	"sort"

	_ "golang.org/x/image/webp"
)

const (
	MinPixelMarkScore = 8

	pixelMarkBlockSize       = 8
	pixelMarkSignsInHash     = 8 * sha256.Size
	pixelMarkLevelStep       = 4
	pixelMarkMinLevelSamples = 50
	pixelMarkClipSigmas      = 2.5
	pixelMarkMADScale        = 1.4826
	pixelMarkSampleStep      = 3
	sameAspectDelta          = 0.01
)

var pixelMarkMask = [pixelMarkBlockSize]uint8{0x00, 0x18, 0x3c, 0x7e, 0x7e, 0x3c, 0x18, 0x00}

func BuildPixelMarkSeed(key ed25519.PrivateKey, originalKey, buyerId string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(originalKey))
	mac.Write([]byte{0})
	mac.Write([]byte(buyerId))
	return mac.Sum(nil)
}

func ScorePixelMark(original, suspect []byte, seeds [][]byte) ([]float64, error) {
	if len(seeds) == 0 {
		return nil, nil
	}
	o, err := decodeLuma(original)
	if err != nil {
		return nil, err
	}
	s, err := decodeLuma(suspect)
	if err != nil {
		return nil, err
	}

	var scores []float64
	turned := s
	for turn := range quarterTurns {
		if turn > 0 {
			turned = turnLuma(turned)
		}
		if math.Abs(turned.aspect()-o.aspect()) < sameAspectDelta {
			resized := scoreSeeds(measureResidual(o, resizeLuma(turned, o.width, o.height), image.Point{}), seeds)
			if scores == nil || slices.Max(resized) > slices.Max(scores) {
				scores = resized
			}
			if slices.Max(scores) >= MinPixelMarkScore {
				return scores, nil
			}
		}
	}
	aligned, offset := alignSuspect(o, s)
	registered := scoreSeeds(measureResidual(o, aligned, offset), seeds)
	if scores == nil || slices.Max(registered) > slices.Max(scores) {
		scores = registered
	}
	return scores, nil
}

func drawPixelMark(canvas *image.RGBA, seed []byte) {
	size := canvas.Rect.Size()
	cols := (size.X + pixelMarkBlockSize - 1) / pixelMarkBlockSize
	rows := (size.Y + pixelMarkBlockSize - 1) / pixelMarkBlockSize
	signs := generatePixelMarkSigns(seed, cols*rows)
	for y := range size.Y {
		mask := pixelMarkMask[y%pixelMarkBlockSize]
		for x := range size.X {
			if mask>>(x%pixelMarkBlockSize)&1 == 0 {
				continue
			}
			isPositive := signs[(y/pixelMarkBlockSize)*cols+x/pixelMarkBlockSize]
			i := canvas.PixOffset(canvas.Rect.Min.X+x, canvas.Rect.Min.Y+y)
			for c := i; c < i+3; c++ {
				switch {
				case isPositive && canvas.Pix[c] < 0xff:
					canvas.Pix[c]++
				case !isPositive && canvas.Pix[c] > 0:
					canvas.Pix[c]--
				}
			}
		}
	}
}

func generatePixelMarkSigns(seed []byte, count int) []bool {
	signs := make([]bool, count)
	input := make([]byte, len(seed)+8)
	copy(input, seed)
	var (
		hash    [sha256.Size]byte
		counter uint64
	)
	for i := range signs {
		bit := i % pixelMarkSignsInHash
		if bit == 0 {
			binary.BigEndian.PutUint64(input[len(seed):], counter)
			hash = sha256.Sum256(input)
			counter++
		}
		signs[i] = hash[bit/8]>>(7-bit%8)&1 == 1
	}
	return signs
}

type pixelMarkResidual struct {
	blocks []float64
	norm   float64
}

func scoreSeeds(r pixelMarkResidual, seeds [][]byte) []float64 {
	scores := make([]float64, len(seeds))
	if r.norm == 0 {
		return scores
	}
	for i, seed := range seeds {
		var sum float64
		for b, isPositive := range generatePixelMarkSigns(seed, len(r.blocks)) {
			if isPositive {
				sum += r.blocks[b]
			} else {
				sum -= r.blocks[b]
			}
		}
		scores[i] = sum / r.norm
	}
	return scores
}

func measureResidual(original, aligned lumaPlane, offset image.Point) pixelMarkResidual {
	cols := (original.width + pixelMarkBlockSize - 1) / pixelMarkBlockSize
	rows := (original.height + pixelMarkBlockSize - 1) / pixelMarkBlockSize
	r := pixelMarkResidual{blocks: make([]float64, cols*rows)}
	overlap := image.Rect(0, 0, original.width, original.height).
		Intersect(image.Rect(0, 0, aligned.width, aligned.height).Add(offset))
	if overlap.Empty() {
		return r
	}
	tone := buildToneCurve(original, aligned, offset, overlap)
	if len(tone.levels) == 0 {
		return r
	}

	var window [pixelMarkBlockSize][pixelMarkBlockSize]float64
	for i := range pixelMarkBlockSize {
		for j := range pixelMarkBlockSize {
			window[i][j] = math.Sin(math.Pi*(float64(i)+0.5)/pixelMarkBlockSize) * math.Sin(math.Pi*(float64(j)+0.5)/pixelMarkBlockSize)
		}
	}
	weights := make([]float64, len(r.blocks))
	counts := make([]int, len(r.blocks))
	for y := overlap.Min.Y; y < overlap.Max.Y; y++ {
		for x := overlap.Min.X; x < overlap.Max.X; x++ {
			b := (y/pixelMarkBlockSize)*cols + x/pixelMarkBlockSize
			w := window[y%pixelMarkBlockSize][x%pixelMarkBlockSize]
			diff := float64(aligned.at(x-offset.X, y-offset.Y)) - tone.apply(float64(original.at(x, y)))
			r.blocks[b] += diff * w
			weights[b] += w
			counts[b]++
		}
	}

	used := make([]float64, 0, len(r.blocks))
	for b := range r.blocks {
		if 2*counts[b] <= pixelMarkBlockSize*pixelMarkBlockSize {
			r.blocks[b] = 0
			counts[b] = 0
			continue
		}
		r.blocks[b] /= max(weights[b], 1e-6)
		used = append(used, r.blocks[b])
	}
	if len(used) == 0 {
		return r
	}
	centre := findMedian(used)
	for i, v := range used {
		used[i] = math.Abs(v - centre)
	}
	limit := pixelMarkClipSigmas * (pixelMarkMADScale*findMedian(used) + 1e-6)
	var sum float64
	for b, v := range r.blocks {
		if counts[b] == 0 {
			continue
		}
		r.blocks[b] = min(max(v, -limit), limit)
		sum += r.blocks[b] * r.blocks[b]
	}
	r.norm = math.Sqrt(sum) + 1e-9
	return r
}

type toneCurve struct {
	levels, values []float64
}

func buildToneCurve(original, aligned lumaPlane, offset image.Point, overlap image.Rectangle) toneCurve {
	bins := make([][2][]float64, 256/pixelMarkLevelStep)
	var n int
	for y := overlap.Min.Y; y < overlap.Max.Y; y++ {
		for x := overlap.Min.X; x < overlap.Max.X; x++ {
			n++
			if (n-1)%pixelMarkSampleStep != 0 {
				continue
			}
			level := float64(original.at(x, y))
			bin := min(max(int(level)/pixelMarkLevelStep, 0), len(bins)-1)
			bins[bin][0] = append(bins[bin][0], level)
			bins[bin][1] = append(bins[bin][1], float64(aligned.at(x-offset.X, y-offset.Y)))
		}
	}
	var tone toneCurve
	for _, bin := range bins {
		if len(bin[0]) < pixelMarkMinLevelSamples {
			continue
		}
		tone.levels = append(tone.levels, findMedian(bin[0]))
		tone.values = append(tone.values, findMedian(bin[1]))
	}
	return tone
}

func (t toneCurve) apply(level float64) float64 {
	i := sort.SearchFloat64s(t.levels, level)
	switch {
	case i == 0:
		return t.values[0]
	case i == len(t.levels):
		return t.values[len(t.values)-1]
	}
	lo, hi := t.levels[i-1], t.levels[i]
	if hi == lo {
		return t.values[i]
	}
	return t.values[i-1] + (t.values[i]-t.values[i-1])*(level-lo)/(hi-lo)
}

func findMedian(values []float64) float64 {
	slices.Sort(values)
	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}

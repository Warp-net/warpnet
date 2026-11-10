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
	"cmp"
	"context"
	"image"
	"math"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Warp-net/warpnet/domain"
)

const (
	MinFrameScore = 0.3

	minFrameCorr = 0.25

	scoreTimeout     = 5 * time.Minute
	decodeThreads    = 4
	signatureBlock   = 8
	minSignatureCorr = 0.8
	candidateCount   = 64
	neighbourReach   = 2
	placeSearch      = 3
	maxAspectChange  = 0.03
	blurRadius       = 2
)

type FrameMatch struct {
	Score       float64
	FramesCount int
}

func (f *FFmpeg) ScoreFrames(original, suspect []byte, sales [][]domain.VideoFrame) ([]FrameMatch, error) {
	matches := make([]FrameMatch, len(sales))
	om, err := parseMovie(original)
	if err != nil {
		return nil, err
	}
	sm, err := parseMovie(suspect)
	if err != nil {
		return matches, nil //nolint:nilerr // a suspect we cannot read holds no frames we can find
	}
	size := thumbnailSize(om.video.displaySize())
	if !isSameAspect(om.video.displaySize(), sm.video.displaySize()) {
		return matches, nil
	}

	dir, err := f.makeWorkDir()
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	ctx, cancel := context.WithTimeout(context.Background(), scoreTimeout)
	defer cancel()

	originals, err := f.decodeFile(ctx, filepath.Join(dir, "original.mp4"), original, om.video, size)
	if err != nil {
		return nil, err
	}
	suspects, err := f.decodeFile(ctx, filepath.Join(dir, "suspect.mp4"), suspect, sm.video, size)
	if err != nil {
		return matches, nil //nolint:nilerr // a suspect ffmpeg cannot decode holds no frames we can find
	}

	places := findSoldPlaces(originals, om.video.rankByPresentation(), sales, size)
	nearest := findNearestOriginals(originals, suspects, size)
	presence := make(map[int]float64, len(places))
	for at, marks := range places {
		key := originals.frames[at]
		best := 0.0
		for _, j := range findCandidates(nearest, at) {
			residual := buildResidual(suspects.frames[j], key, size)
			for _, mark := range marks {
				best = max(best, correlate(residual, mark.residual))
			}
		}
		if best >= minFrameCorr {
			presence[at] = best
		}
	}

	for at, corr := range presence {
		owners := make(map[int]bool, len(places[at]))
		for _, mark := range places[at] {
			owners[mark.sale] = true
		}
		for sale := range matches {
			if owners[sale] {
				matches[sale].Score += corr
				matches[sale].FramesCount++
				continue
			}
			matches[sale].Score -= corr
		}
	}
	return matches, nil
}

type soldMark struct {
	sale     int
	residual []float64
}

func findSoldPlaces(originals thumbnails, ranks []int, sales [][]domain.VideoFrame, size image.Point) map[int][]soldMark {
	places := map[int][]soldMark{}
	for sale, frames := range sales {
		for _, frame := range frames {
			thumbSize, thumb, err := decodeThumbnail(frame.Thumbnail)
			if err != nil || thumbSize != size || frame.SampleIndex < 0 || frame.SampleIndex >= len(ranks) {
				continue
			}
			at := findPlace(originals.frames, ranks[frame.SampleIndex], thumb, size)
			if at < 0 {
				continue
			}
			places[at] = append(places[at], soldMark{sale: sale, residual: buildResidual(thumb, originals.frames[at], size)})
		}
	}
	return places
}

func findNearestOriginals(originals, suspects thumbnails, size image.Point) []int {
	signatures := make([][]float64, len(originals.frames))
	for i, frame := range originals.frames {
		signatures[i] = buildSignature(frame, size)
	}
	nearest := make([]int, len(suspects.frames))
	for j, frame := range suspects.frames {
		signature := buildSignature(frame, size)
		best, bestCorr := -1, minSignatureCorr
		for i, other := range signatures {
			if corr := dot(signature, other); corr > bestCorr {
				best, bestCorr = i, corr
			}
		}
		nearest[j] = best
	}
	return nearest
}

func findCandidates(nearest []int, at int) []int {
	candidates := make([]int, 0, candidateCount)
	seen := make(map[int]bool, candidateCount)
	for j, i := range nearest {
		if i < 0 || i < at-neighbourReach || i > at+neighbourReach {
			continue
		}
		for k := max(j-neighbourReach, 0); k <= min(j+neighbourReach, len(nearest)-1); k++ {
			if !seen[k] && len(candidates) < candidateCount {
				seen[k] = true
				candidates = append(candidates, k)
			}
		}
	}
	return candidates
}

func (f *FFmpeg) decodeFile(ctx context.Context, path string, b []byte, t track, size image.Point) (thumbnails, error) {
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return thumbnails{}, err
	}
	return f.decodeThumbnails(ctx, path, t, size, decodeThreads)
}

func isSameAspect(a, b image.Point) bool {
	if a.X <= 0 || a.Y <= 0 || b.X <= 0 || b.Y <= 0 {
		return false
	}
	ratio := float64(a.X*b.Y) / float64(a.Y*b.X)
	return math.Abs(ratio-1) <= maxAspectChange
}

func (t *track) rankByPresentation() []int {
	order := make([]int, len(t.samples))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Compare(t.samples[a].pts(), t.samples[b].pts())
	})
	skipped := 0
	for _, i := range order {
		if t.samples[i].pts() < t.mediaTime {
			skipped++
		}
	}
	ranks := make([]int, len(t.samples))
	for rank, i := range order {
		ranks[i] = rank - skipped
	}
	return ranks
}

func findPlace(frames [][]byte, rank int, thumb []byte, size image.Point) int {
	best, bestEnergy := -1, math.Inf(1)
	for at := rank - placeSearch; at <= rank+placeSearch; at++ {
		if at < 0 || at >= len(frames) {
			continue
		}
		energy := measureEnergy(buildResidual(thumb, frames[at], size))
		if energy < bestEnergy {
			best, bestEnergy = at, energy
		}
	}
	return best
}

func buildSignature(frame []byte, size image.Point) []float64 {
	cols, rows := max(size.X/signatureBlock, 1), max(size.Y/signatureBlock, 1)
	signature := make([]float64, cols*rows)
	for y := range rows * signatureBlock {
		for x := range cols * signatureBlock {
			if x < size.X && y < size.Y {
				signature[(y/signatureBlock)*cols+x/signatureBlock] += float64(frame[y*size.X+x])
			}
		}
	}
	return normalize(signature)
}

func buildResidual(frame, key []byte, size image.Point) []float64 {
	n := size.X * size.Y
	var sumKey, sumFrame, sumKeyKey, sumKeyFrame float64
	for i := range n {
		k, v := float64(key[i]), float64(frame[i])
		sumKey += k
		sumFrame += v
		sumKeyKey += k * k
		sumKeyFrame += k * v
	}
	count := float64(n)
	variance := sumKeyKey - sumKey*sumKey/count
	gain := 1.0
	if variance > 0 {
		gain = (sumKeyFrame - sumKey*sumFrame/count) / variance
	}
	offset := (sumFrame - gain*sumKey) / count
	residual := make([]float64, n)
	for i := range n {
		residual[i] = float64(frame[i]) - gain*float64(key[i]) - offset
	}
	return subtractBlur(residual, size)
}

func subtractBlur(values []float64, size image.Point) []float64 {
	w, h := size.X, size.Y
	integral := make([]float64, (w+1)*(h+1))
	for y := range h {
		row := 0.0
		for x := range w {
			row += values[y*w+x]
			integral[(y+1)*(w+1)+x+1] = integral[y*(w+1)+x+1] + row
		}
	}
	out := make([]float64, len(values))
	for y := range h {
		y0, y1 := max(y-blurRadius, 0), min(y+blurRadius+1, h)
		for x := range w {
			x0, x1 := max(x-blurRadius, 0), min(x+blurRadius+1, w)
			sum := integral[y1*(w+1)+x1] - integral[y0*(w+1)+x1] - integral[y1*(w+1)+x0] + integral[y0*(w+1)+x0]
			out[y*w+x] = values[y*w+x] - sum/float64((x1-x0)*(y1-y0))
		}
	}
	return out
}

func measureEnergy(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v * v
	}
	return sum
}

func correlate(a, b []float64) float64 {
	return dot(normalize(slices.Clone(a)), normalize(slices.Clone(b)))
}

func normalize(values []float64) []float64 {
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(max(len(values), 1))
	norm := 0.0
	for i := range values {
		values[i] -= mean
		norm += values[i] * values[i]
	}
	if norm == 0 {
		return values
	}
	norm = math.Sqrt(norm)
	for i := range values {
		values[i] /= norm
	}
	return values
}

func dot(a, b []float64) float64 {
	sum := 0.0
	for i := range min(len(a), len(b)) {
		sum += a[i] * b[i]
	}
	return sum
}

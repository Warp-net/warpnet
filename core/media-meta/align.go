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
	"image/draw"
	_ "image/gif"
	"math"
	"math/cmplx"
	"runtime"
	"sync"

	"github.com/Warp-net/warpnet/core/warpnet"
	"gonum.org/v1/gonum/dsp/fourier"
)

const (
	coarseAlignSide  = 400
	fineAlignSide    = 800
	alignWindowSide  = 1024
	minAlignSide     = 24
	maxAlignGrowth   = 4
	maxAlignWorkers  = 4
	coarseScaleCount = 180
	fineScaleCount   = 61
	minAlignScale    = 0.2
	maxAlignScale    = 3.0
	fineScaleSpread  = 0.03
	coarseCutoff     = 0.12
	fineCutoff       = 0.06
	quarterTurns     = 4

	footprintMarginDivisor = 8

	ErrEmptyImage warpnet.WarpError = "copy check: the image is empty"
)

type lumaPlane struct {
	width, height int
	pix           []float32
}

func (p lumaPlane) at(x, y int) float32 {
	return p.pix[y*p.width+x]
}

func (p lumaPlane) aspect() float64 {
	return float64(p.width) / float64(p.height)
}

func decodeLuma(b []byte) (lumaPlane, error) {
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return lumaPlane{}, err
	}
	bounds := img.Bounds()
	if bounds.Empty() {
		return lumaPlane{}, ErrEmptyImage
	}
	canvas := image.NewRGBA(image.Rectangle{Max: bounds.Size()})
	draw.Draw(canvas, canvas.Rect, img, bounds.Min, draw.Src)
	p := lumaPlane{width: bounds.Dx(), height: bounds.Dy(), pix: make([]float32, bounds.Dx()*bounds.Dy())}
	for i := range p.pix {
		rgb := canvas.Pix[4*i : 4*i+3]
		p.pix[i] = float32(0.299*float64(rgb[0]) + 0.587*float64(rgb[1]) + 0.114*float64(rgb[2]))
	}
	return p, nil
}

func turnLuma(p lumaPlane) lumaPlane {
	out := lumaPlane{width: p.height, height: p.width, pix: make([]float32, len(p.pix))}
	for y := range p.height {
		for x := range p.width {
			out.pix[x*out.width+p.height-1-y] = p.pix[y*p.width+x]
		}
	}
	return out
}

func cropLuma(p lumaPlane, r image.Rectangle) lumaPlane {
	out := lumaPlane{width: r.Dx(), height: r.Dy(), pix: make([]float32, r.Dx()*r.Dy())}
	for y := range out.height {
		copy(out.pix[y*out.width:(y+1)*out.width], p.pix[(r.Min.Y+y)*p.width+r.Min.X:])
	}
	return out
}

type filterTap struct {
	start   int
	weights []float32
}

func buildFilterTaps(from, to int) []filterTap {
	scale := float64(from) / float64(to)
	support := max(scale, 1)
	taps := make([]filterTap, to)
	for i := range taps {
		centre := (float64(i) + 0.5) * scale
		lo := max(int(centre-support+0.5), 0)
		hi := min(int(centre+support+0.5), from)
		weights := make([]float32, 0, max(hi-lo, 1))
		var sum float64
		for j := lo; j < hi; j++ {
			w := max(1-math.Abs((float64(j)+0.5-centre)/support), 0)
			weights = append(weights, float32(w))
			sum += w
		}
		if sum == 0 {
			lo, weights, sum = min(max(int(centre), 0), from-1), []float32{1}, 1
		}
		for k := range weights {
			weights[k] /= float32(sum)
		}
		taps[i] = filterTap{start: lo, weights: weights}
	}
	return taps
}

func resizeLuma(p lumaPlane, width, height int) lumaPlane {
	if width == p.width && height == p.height {
		return p
	}
	across := buildFilterTaps(p.width, width)
	down := buildFilterTaps(p.height, height)
	mid := make([]float32, width*p.height)
	for y := range p.height {
		row := p.pix[y*p.width : (y+1)*p.width]
		for x, tap := range across {
			var v float32
			for k, w := range tap.weights {
				v += w * row[tap.start+k]
			}
			mid[y*width+x] = v
		}
	}
	out := lumaPlane{width: width, height: height, pix: make([]float32, width*height)}
	for y, tap := range down {
		dst := out.pix[y*width : (y+1)*width]
		for k, w := range tap.weights {
			src := mid[(tap.start+k)*width : (tap.start+k+1)*width]
			for x := range dst {
				dst[x] += w * src[x]
			}
		}
	}
	return out
}

func alignSuspect(original, suspect lumaPlane) (lumaPlane, image.Point) {
	scale, suspect := searchCoarseTurn(original, suspect)
	scale, shift := searchFineScale(original, suspect, scale)
	aligned := resizeLuma(suspect, scaleSide(suspect.width, 1/scale), scaleSide(suspect.height, 1/scale))
	return aligned, correctShift(original, aligned, shift)
}

func searchCoarseTurn(original, suspect lumaPlane) (float64, lumaPlane) {
	bestScale, best, bestPeak := searchCoarseScale(original, suspect)
	for range quarterTurns - 1 {
		suspect = turnLuma(suspect)
		scale, footprint, peak := searchCoarseScale(original, suspect)
		if peak > bestPeak {
			bestScale, best, bestPeak = scale, footprint, peak
		}
	}
	return bestScale, best
}

func searchCoarseScale(original, suspect lumaPlane) (float64, lumaPlane, float64) {
	factor := min(0.25, coarseAlignSide/float64(max(original.width, original.height)))
	scales := make([]float64, coarseScaleCount)
	for i := range scales {
		scales[i] = minAlignScale * math.Pow(maxAlignScale/minAlignScale, float64(i)/(coarseScaleCount-1))
	}
	i, shift, peak := searchScale(original, suspect, factor, scales, coarseCutoff)
	if i < 0 {
		return 1, suspect, peak
	}
	return scales[i], cropFootprint(original, suspect, scales[i], scalePoint(shift, -scales[i]/factor)), peak
}

func searchFineScale(original, suspect lumaPlane, scale float64) (float64, image.Point) {
	factor := min(0.5, fineAlignSide/float64(max(original.width, original.height)))
	scales := make([]float64, fineScaleCount)
	for i := range scales {
		scales[i] = scale * (1 - fineScaleSpread + 2*fineScaleSpread*float64(i)/(fineScaleCount-1))
	}
	i, shift, _ := searchScale(original, suspect, factor, scales, fineCutoff)
	if i < 0 {
		return scale, image.Point{}
	}
	return scales[i], scalePoint(shift, 1/factor)
}

func correctShift(original, aligned lumaPlane, shift image.Point) image.Point {
	overlap := image.Rect(0, 0, original.width, original.height).
		Intersect(image.Rect(0, 0, aligned.width, aligned.height).Add(shift))
	if overlap.Dx() < minAlignSide || overlap.Dy() < minAlignSide {
		return shift
	}
	size := image.Pt(min(overlap.Dx(), alignWindowSide), min(overlap.Dy(), alignWindowSide))
	from := overlap.Min.Add(overlap.Size().Sub(size).Div(2))
	window := image.Rectangle{Min: from, Max: from.Add(size)}
	_, correction := correlatePhase(newSpectrumCache(cropLuma(original, window)), cropLuma(aligned, window.Sub(shift)), fineCutoff)
	return shift.Add(correction)
}

func cropFootprint(original, suspect lumaPlane, scale float64, from image.Point) lumaPlane {
	size := image.Pt(scaleSide(original.width, scale), scaleSide(original.height, scale))
	footprint := image.Rectangle{Min: from, Max: from.Add(size)}
	part := footprint.Inset(-max(size.X, size.Y) / footprintMarginDivisor).Intersect(image.Rect(0, 0, suspect.width, suspect.height))
	if part.Dx() < minAlignSide || part.Dy() < minAlignSide {
		return suspect
	}
	return cropLuma(suspect, part)
}

func searchScale(original, suspect lumaPlane, factor float64, scales []float64, cutoff float64) (int, image.Point, float64) {
	o := resizeLuma(original, max(int(float64(original.width)*factor), 1), max(int(float64(original.height)*factor), 1))
	spectra := newSpectrumCache(o)
	peaks := make([]float64, len(scales))
	shifts := make([]image.Point, len(scales))
	indexes := make(chan int, len(scales))
	for i := range scales {
		indexes <- i
	}
	close(indexes)

	var wg sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), maxAlignWorkers) {
		wg.Go(func() {
			for i := range indexes {
				peaks[i] = -1
				width := scaleSide(suspect.width, factor/scales[i])
				height := scaleSide(suspect.height, factor/scales[i])
				if width < minAlignSide || height < minAlignSide || width > maxAlignGrowth*o.width || height > maxAlignGrowth*o.height {
					continue
				}
				peaks[i], shifts[i] = correlatePhase(spectra, resizeLuma(suspect, width, height), cutoff)
			}
		})
	}
	wg.Wait()

	best := -1
	for i, peak := range peaks {
		if peak > -1 && (best < 0 || peak > peaks[best]) {
			best = i
		}
	}
	if best < 0 {
		return best, image.Point{}, -1
	}
	return best, shifts[best], peaks[best]
}

func scaleSide(side int, factor float64) int {
	return max(int(math.Round(float64(side)*factor)), 1)
}

func scalePoint(p image.Point, factor float64) image.Point {
	return image.Pt(int(math.Round(float64(p.X)*factor)), int(math.Round(float64(p.Y)*factor)))
}

type spectrumCache struct {
	plane   lumaPlane
	mu      sync.Mutex
	spectra map[image.Point]*cachedSpectrum
	used    []image.Point
}

type cachedSpectrum struct {
	once     sync.Once
	spectrum []complex128
}

func newSpectrumCache(p lumaPlane) *spectrumCache {
	return &spectrumCache{plane: p, spectra: make(map[image.Point]*cachedSpectrum)}
}

func (c *spectrumCache) transform(height, width int) []complex128 {
	size := image.Pt(width, height)
	c.mu.Lock()
	entry, ok := c.spectra[size]
	if !ok {
		entry = &cachedSpectrum{}
		c.spectra[size] = entry
		c.used = append(c.used, size)
		if len(c.used) > maxAlignWorkers+1 {
			delete(c.spectra, c.used[0])
			c.used = c.used[1:]
		}
	}
	c.mu.Unlock()
	entry.once.Do(func() { entry.spectrum = transformLuma(c.plane, height, width) })
	return entry.spectrum
}

func correlatePhase(a *spectrumCache, b lumaPlane, cutoff float64) (float64, image.Point) {
	height := findFFTSize(max(a.plane.height, b.height))
	width := findFFTSize(max(a.plane.width, b.width))
	cols := width/2 + 1
	fa := a.transform(height, width)
	fb := transformLuma(b, height, width)
	for y := range height {
		fy := float64(y) / float64(height)
		if y > height/2 {
			fy -= 1
		}
		for x := range cols {
			fx := float64(x) / float64(width)
			v := fa[y*cols+x] * cmplx.Conj(fb[y*cols+x])
			gain := math.Exp(-(fy*fy+fx*fx)/(2*cutoff*cutoff)) / (cmplx.Abs(v) + 1e-9)
			fb[y*cols+x] = v * complex(gain, 0)
		}
	}
	c := transformBack(fb, height, width)

	best := 0
	for i := range c {
		if c[i] > c[best] {
			best = i
		}
	}
	dy, dx := best/width, best%width
	if dy > height/2 {
		dy -= height
	}
	if dx > width/2 {
		dx -= width
	}
	return c[best] / float64(height*width), image.Pt(dx, dy)
}

func transformLuma(p lumaPlane, height, width int) []complex128 {
	var mean float64
	for _, v := range p.pix {
		mean += float64(v)
	}
	mean /= float64(len(p.pix))

	cols := width/2 + 1
	spectrum := make([]complex128, height*cols)
	rows := fourier.NewFFT(width)
	seq := make([]float64, width)
	for y := range p.height {
		for x := range p.width {
			seq[x] = float64(p.pix[y*p.width+x]) - mean
		}
		rows.Coefficients(spectrum[y*cols:(y+1)*cols], seq)
	}
	transformColumns(spectrum, cols, fourier.NewCmplxFFT(height).Coefficients)
	return spectrum
}

func transformBack(spectrum []complex128, height, width int) []float64 {
	cols := width/2 + 1
	transformColumns(spectrum, cols, fourier.NewCmplxFFT(height).Sequence)
	rows := fourier.NewFFT(width)
	out := make([]float64, height*width)
	for y := range height {
		rows.Sequence(out[y*width:(y+1)*width], spectrum[y*cols:(y+1)*cols])
	}
	return out
}

func transformColumns(spectrum []complex128, cols int, transform func(dst, seq []complex128) []complex128) {
	column := make([]complex128, len(spectrum)/cols)
	for x := range cols {
		for y := range column {
			column[y] = spectrum[y*cols+x]
		}
		transform(column, column)
		for y, v := range column {
			spectrum[y*cols+x] = v
		}
	}
}

func findFFTSize(n int) int {
	for size := max(n, 1); ; size++ {
		rest := size
		for _, p := range []int{2, 3, 5} {
			for rest%p == 0 {
				rest /= p
			}
		}
		if rest == 1 {
			return size
		}
	}
}

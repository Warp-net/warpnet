// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package media_meta

import (
	"fmt"
	"image"
	"math"
	"math/cmplx"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func texturedLuma(width, height int) lumaPlane {
	p := lumaPlane{width: width, height: height, pix: make([]float32, width*height)}
	for y := range height {
		for x := range width {
			fx, fy := float64(x), float64(y)
			v := 110 + 60*math.Sin(fx/17) + 35*math.Cos(fy/11+fx/29)
			if dx, dy := fx-float64(width)/3, fy-float64(height)/2; dx*dx+dy*dy < float64(height*height)/16 {
				v = 210 - 0.2*fy
			}
			if x > width*2/3 && y%12 < 5 {
				v -= 70
			}
			if (x/9+y/7)%5 == 0 {
				v += 40
			}
			v += float64((x*7919+y*104729)%23) - 11
			p.pix[y*width+x] = float32(v)
		}
	}
	return p
}

func turnLumaTimes(p lumaPlane, turns int) lumaPlane {
	for range turns {
		p = turnLuma(p)
	}
	return p
}

func correlatePhaseWithoutCache(a, b lumaPlane, cutoff float64) (float64, image.Point) {
	height := findFFTSize(max(a.height, b.height))
	width := findFFTSize(max(a.width, b.width))
	cols := width/2 + 1
	fa := transformLuma(a, height, width)
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
			fa[y*cols+x] = v * complex(gain, 0)
		}
	}
	c := transformBack(fa, height, width)
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

func TestTurnLuma_TurnsAQuarterRight(t *testing.T) {
	p := lumaPlane{width: 3, height: 2, pix: []float32{1, 2, 3, 4, 5, 6}}
	assert.Equal(t, lumaPlane{width: 2, height: 3, pix: []float32{4, 1, 5, 2, 6, 3}}, turnLuma(p))
	assert.Equal(t, p, lumaPlane{width: 3, height: 2, pix: []float32{1, 2, 3, 4, 5, 6}}, "the input is left as it was")
}

func TestTurnLuma_ComesBackAfterFourTurns(t *testing.T) {
	for _, size := range []image.Point{{1, 1}, {1, 5}, {5, 1}, {4, 3}, {7, 7}, {33, 20}} {
		p := texturedLuma(size.X, size.Y)
		for turns := range 4 {
			turned := turnLumaTimes(p, turns)
			if turns%2 == 1 {
				assert.Equal(t, image.Pt(size.Y, size.X), image.Pt(turned.width, turned.height))
			} else {
				assert.Equal(t, size, image.Pt(turned.width, turned.height))
			}
		}
		assert.Equal(t, p, turnLumaTimes(p, 4), "size %v", size)
	}
}

func TestTurnLuma_TurnsLikeTheUpload(t *testing.T) {
	img := patternImage(image.Pt(13, 8))
	luma, err := decodeLuma(encodePNG(t, img))
	require.NoError(t, err)
	turned, err := decodeLuma(encodePNG(t, applyOrientation(img, orientationTurnRight)))
	require.NoError(t, err)
	assert.Equal(t, turned, turnLuma(luma))
}

func TestSpectrumCache_GivesTheDirectTransform(t *testing.T) {
	p := texturedLuma(40, 30)
	cache := newSpectrumCache(p)
	for _, size := range []image.Point{{40, 30}, {45, 32}, {64, 48}, {81, 75}} {
		assert.Equal(t, transformLuma(p, size.Y, size.X), cache.transform(size.Y, size.X), "size %v", size)
	}
}

func TestSpectrumCache_TransformsASizeOnce(t *testing.T) {
	cache := newSpectrumCache(texturedLuma(40, 30))
	first := cache.transform(32, 40)
	assert.Same(t, &first[0], &cache.transform(32, 40)[0], "the same size comes from the cache")
	assert.NotSame(t, &first[0], &cache.transform(32, 45)[0], "another size is its own transform")
}

func TestSpectrumCache_KeepsOnlyTheRecentSizes(t *testing.T) {
	p := texturedLuma(40, 30)
	cache := newSpectrumCache(p)
	first := cache.transform(30, 40)
	for i := 1; i <= maxAlignWorkers+2; i++ {
		cache.transform(30, 40+i)
	}
	assert.Len(t, cache.spectra, maxAlignWorkers+1)
	assert.Len(t, cache.used, maxAlignWorkers+1)
	assert.NotContains(t, cache.spectra, image.Pt(40, 30), "the oldest size goes first")
	assert.Contains(t, cache.spectra, image.Pt(40+maxAlignWorkers+2, 30), "the newest size stays")

	again := cache.transform(30, 40)
	assert.Equal(t, first, again, "a size dropped from the cache comes back the same")
	assert.NotSame(t, &first[0], &again[0])
}

func TestSpectrumCache_SharesOneTransformAcrossWorkers(t *testing.T) {
	cache := newSpectrumCache(texturedLuma(64, 48))
	spectra := make([][]complex128, 32)
	var wg sync.WaitGroup
	for i := range spectra {
		wg.Go(func() { spectra[i] = cache.transform(48, 64) })
	}
	wg.Wait()
	for _, spectrum := range spectra[1:] {
		assert.Same(t, &spectra[0][0], &spectrum[0])
	}
}

func TestCorrelatePhase_FindsWhereTheSuspectSits(t *testing.T) {
	original := texturedLuma(96, 64)
	for _, at := range []image.Point{{0, 0}, {13, 7}, {40, 25}, {1, 30}} {
		suspect := cropLuma(original, image.Rectangle{Min: at, Max: at.Add(image.Pt(48, 32))})
		_, shift := correlatePhase(newSpectrumCache(original), suspect, coarseCutoff)
		assert.Equal(t, at, shift, "a crop taken at %v", at)
	}

	frame := lumaPlane{width: 140, height: 100, pix: make([]float32, 140*100)}
	for y := range original.height {
		copy(frame.pix[(y+20)*frame.width+30:], original.pix[y*original.width:(y+1)*original.width])
	}
	_, shift := correlatePhase(newSpectrumCache(original), frame, coarseCutoff)
	assert.Equal(t, image.Pt(-30, -20), shift, "the original inside a larger screenshot")
}

func TestCorrelatePhase_CorrectsASmallMisplacement(t *testing.T) {
	original := texturedLuma(256, 192)
	for _, at := range []image.Point{{0, 0}, {3, 2}, {7, 0}, {0, 5}} {
		window := cropLuma(original, image.Rectangle{Min: at, Max: at.Add(image.Pt(240, 180))})
		_, shift := correlatePhase(newSpectrumCache(original), window, fineCutoff)
		assert.Equal(t, at, shift, "a window misplaced by %v", at)
	}
}

func TestCorrelatePhase_MatchesTheUncachedCorrelation(t *testing.T) {
	original := texturedLuma(120, 90)
	cache := newSpectrumCache(original)
	suspects := []lumaPlane{
		original,
		cropLuma(original, image.Rect(10, 5, 90, 70)),
		resizeLuma(original, 60, 45),
		resizeLuma(original, 300, 225),
		turnLuma(original),
		turnLumaTimes(cropLuma(original, image.Rect(20, 10, 110, 80)), 2),
		texturedLuma(200, 40),
	}
	for round := range 2 {
		for i, suspect := range suspects {
			wantPeak, wantShift := correlatePhaseWithoutCache(original, suspect, coarseCutoff)
			peak, shift := correlatePhase(cache, suspect, coarseCutoff)
			assert.Equal(t, wantPeak, peak, "round %d suspect %d", round, i)
			assert.Equal(t, wantShift, shift, "round %d suspect %d", round, i)
		}
	}
	for size, entry := range cache.spectra {
		assert.Equal(t, transformLuma(original, size.Y, size.X), entry.spectrum, "a correlation must not write into the cached spectrum of %v", size)
	}
}

func TestSearchScale_ReportsNoPeakWhenNoScaleFits(t *testing.T) {
	original := texturedLuma(400, 300)
	tiny := texturedLuma(10, 10)
	i, shift, peak := searchScale(original, tiny, 0.25, []float64{0.5, 1, 2}, coarseCutoff)
	assert.Equal(t, -1, i)
	assert.Equal(t, image.Point{}, shift)
	assert.Equal(t, -1.0, peak)

	match := searchCoarseTurns(original, tiny)[0]
	assert.Equal(t, 1.0, match.scale)
	assert.Equal(t, -1.0, match.peak)
	assert.Equal(t, tiny, match.footprint, "with nothing to match the suspect stays as it came")
}

func TestSearchCoarseTurns_RanksEveryTurnByItsPeak(t *testing.T) {
	original := texturedLuma(320, 240)
	matches := searchCoarseTurns(original, turnLuma(resizeLuma(original, 192, 144)))
	require.Len(t, matches, 4)
	for i := 1; i < len(matches); i++ {
		assert.GreaterOrEqual(t, matches[i-1].peak, matches[i].peak, "the best peak comes first")
	}
	var landscape, portrait int
	for _, match := range matches {
		if match.footprint.width > match.footprint.height {
			landscape++
		} else {
			portrait++
		}
	}
	assert.Equal(t, 2, landscape, "two turns stand the suspect like the original")
	assert.Equal(t, 2, portrait, "two turns lay it on its side")
}

func TestSearchCoarseTurns_PutsTheTurnBackFirst(t *testing.T) {
	original := texturedLuma(320, 240)
	for turns := range 4 {
		t.Run(fmt.Sprintf("%d quarter turns", turns), func(t *testing.T) {
			t.Parallel()
			match := searchCoarseTurns(original, turnLumaTimes(resizeLuma(original, 192, 144), turns))[0]
			assert.InDelta(t, 0.6, match.scale, 0.03)
			assert.Greater(t, match.footprint.width, match.footprint.height, "the suspect comes back to the original's shape")
		})
	}
}

func TestAlignSuspect_PlacesATurnedCrop(t *testing.T) {
	original := texturedLuma(320, 240)
	crop := image.Rect(48, 32, 240, 176)
	shown := cropLuma(original, crop)
	shown = resizeLuma(shown, scaleSide(shown.width, 0.7), scaleSide(shown.height, 0.7))
	for turns := range 4 {
		t.Run(fmt.Sprintf("%d quarter turns", turns), func(t *testing.T) {
			t.Parallel()
			turned := turnLumaTimes(shown, turns)
			aligned, offset := alignSuspect(original, searchCoarseTurns(original, turned)[0])
			assert.InDelta(t, crop.Dx(), aligned.width, 4)
			assert.InDelta(t, crop.Dy(), aligned.height, 4)
			assert.InDelta(t, crop.Min.X, offset.X, 3)
			assert.InDelta(t, crop.Min.Y, offset.Y, 3)
		})
	}
}

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
	"fmt"
	"image"
	"image/color"
	"math/bits"
	"strconv"
)

const (
	imageHashColumns     = 9
	imageHashRows        = 8
	maxImageHashDistance = 10
	minImageHashBits     = 8
)

func HashImage(file string) (hash, mirrored string, err error) {
	_, raw, err := SplitDataURI(file)
	if err != nil {
		return "", "", err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", "", err
	}
	cells := measureCells(img)
	var straight, flipped uint64
	for y := range imageHashRows {
		for x := range imageHashColumns - 1 {
			straight <<= 1
			if cells[y][x] < cells[y][x+1] {
				straight |= 1
			}
			flipped <<= 1
			if cells[y][imageHashColumns-1-x] < cells[y][imageHashColumns-2-x] {
				flipped |= 1
			}
		}
	}
	return fmt.Sprintf("%016x", straight), fmt.Sprintf("%016x", flipped), nil
}

func IsSameImage(hash, other string) bool {
	a, err := strconv.ParseUint(hash, 16, 64)
	if err != nil {
		return false
	}
	b, err := strconv.ParseUint(other, 16, 64)
	if err != nil || !isInformative(a) || !isInformative(b) {
		return false
	}
	return bits.OnesCount64(a^b) <= maxImageHashDistance
}

func isInformative(hash uint64) bool {
	set := bits.OnesCount64(hash)
	return set >= minImageHashBits && set <= 64-minImageHashBits
}

func measureCells(img image.Image) [imageHashRows][imageHashColumns]float64 {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	var sums [imageHashRows][imageHashColumns]float64
	var counts [imageHashRows][imageHashColumns]int
	ycbcr, isYCbCr := img.(*image.YCbCr)
	for y := range height {
		row := y * imageHashRows / height
		for x := range width {
			column := x * imageHashColumns / width
			var luma uint8
			if isYCbCr {
				luma = ycbcr.Y[ycbcr.YOffset(bounds.Min.X+x, bounds.Min.Y+y)]
			} else {
				luma = color.GrayModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.Gray).Y
			}
			sums[row][column] += float64(luma)
			counts[row][column]++
		}
	}
	for y := range imageHashRows {
		for x := range imageHashColumns {
			if counts[y][x] > 0 {
				sums[y][x] /= float64(counts[y][x])
			}
		}
	}
	return sums
}

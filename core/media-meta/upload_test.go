// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package media_meta

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	tiffShort       = 3
	tiffLong        = 4
	tiffImageWidth  = 0x0100
	tiffOrientation = 0x0112
)

var allOrientations = []int{1, 2, 3, 4, 5, 6, 7, 8}

type tiffEntry struct {
	tag, kind uint16
	count     uint32
	value     uint32
}

func buildTIFF(order binary.AppendByteOrder, ifds ...[]tiffEntry) []byte {
	b := []byte("II\x2a\x00")
	if order == binary.BigEndian {
		b = []byte("MM\x00\x2a")
	}
	b = order.AppendUint32(b, 8)
	for i, entries := range ifds {
		b = order.AppendUint16(b, uint16(len(entries)))
		for _, e := range entries {
			b = order.AppendUint16(b, e.tag)
			b = order.AppendUint16(b, e.kind)
			b = order.AppendUint32(b, e.count)
			if e.kind == tiffShort {
				b = order.AppendUint16(b, uint16(e.value))
				b = order.AppendUint16(b, uint16(e.value))
			} else {
				b = order.AppendUint32(b, e.value)
			}
		}
		next := uint32(0)
		if i < len(ifds)-1 {
			next = uint32(len(b) + 4)
		}
		b = order.AppendUint32(b, next)
	}
	return b
}

func orientationTIFF(order binary.AppendByteOrder, orientation int) []byte {
	return buildTIFF(order, []tiffEntry{{tag: tiffOrientation, kind: tiffShort, count: 1, value: uint32(orientation)}})
}

func jpegSegment(marker byte, payload []byte) []byte {
	size := len(payload) + 2
	return append([]byte{0xff, marker, byte(size >> 8), byte(size)}, payload...)
}

func exifSegment(tiff []byte) []byte {
	return jpegSegment(0xe1, append([]byte("Exif\x00\x00"), tiff...))
}

func insertSegments(jpegBytes []byte, segments ...[]byte) []byte {
	out := append([]byte{}, jpegBytes[:2]...)
	for _, segment := range segments {
		out = append(out, segment...)
	}
	return append(out, jpegBytes[2:]...)
}

func jpegWithOrientation(t *testing.T, jpegBytes []byte, orientation int) []byte {
	t.Helper()

	segments, err := parseJPEGSegments(jpegBytes)
	require.NoError(t, err)

	ifdMapping, err := exifcommon.NewIfdMappingWithStandard()
	require.NoError(t, err)
	tagIndex := exif.NewTagIndex()
	require.NoError(t, exif.LoadStandardTags(tagIndex))

	rootIb := exif.NewIfdBuilder(ifdMapping, tagIndex, exifcommon.IfdStandardIfdIdentity, exifcommon.EncodeDefaultByteOrder)
	require.NoError(t, rootIb.SetStandardWithName(orientationTag, []uint16{uint16(orientation)}))
	require.NoError(t, segments.SetExif(rootIb))

	var buf bytes.Buffer
	require.NoError(t, segments.Write(&buf))
	return buf.Bytes()
}

func uploadImage(t *testing.T, file []byte, mime string) image.Image {
	t.Helper()

	meta, err := BuildMetadata(
		warpnet.NodeInfo{ID: signerID, OwnerId: "alice"},
		signerKey,
		domain.User{Id: "alice", NodeId: signerID.String()},
	)
	require.NoError(t, err)

	signed, err := SignUploadedImage("data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(file), meta)
	require.NoError(t, err)
	_, raw, err := SplitDataURI(string(signed))
	require.NoError(t, err)
	require.NoError(t, VerifyImage(raw, signerID.String(), "alice"), "the stored file is signed")
	assert.Zero(t, extractOrientation(raw), "the stored file carries no orientation, so nothing turns it again")
	return decodeJPEG(t, raw)
}

func uprightPoint(orientation int, p, size image.Point) image.Point {
	w, h := size.X, size.Y
	switch orientation {
	case 2:
		return image.Pt(w-1-p.X, p.Y)
	case 3:
		return image.Pt(w-1-p.X, h-1-p.Y)
	case 4:
		return image.Pt(p.X, h-1-p.Y)
	case 5:
		return image.Pt(p.Y, p.X)
	case 6:
		return image.Pt(h-1-p.Y, p.X)
	case 7:
		return image.Pt(h-1-p.Y, w-1-p.X)
	case 8:
		return image.Pt(p.Y, w-1-p.X)
	}
	return p
}

func uprightSize(orientation int, size image.Point) image.Point {
	if orientation >= 5 && orientation <= 8 {
		return image.Pt(size.Y, size.X)
	}
	return size
}

func patternImage(size image.Point) *image.RGBA {
	img := image.NewRGBA(image.Rectangle{Max: size})
	for y := range size.Y {
		for x := range size.X {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 37), G: uint8(y * 53), B: uint8(x*y + 11), A: 0xff})
		}
	}
	return img
}

func rgbaAt(img image.Image, p image.Point) color.RGBA {
	return color.RGBAModel.Convert(img.At(p.X, p.Y)).(color.RGBA)
}

func assertUpright(t *testing.T, stored, upright image.Image, orientation int, tolerance int) {
	t.Helper()

	bounds := stored.Bounds()
	require.Equal(t, image.Rectangle{Max: uprightSize(orientation, bounds.Size())}, upright.Bounds(), "orientation %d", orientation)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			want := rgbaAt(stored, image.Pt(x, y))
			got := rgbaAt(upright, uprightPoint(orientation, image.Pt(x, y).Sub(bounds.Min), bounds.Size()))
			for i, pair := range [][2]uint8{{want.R, got.R}, {want.G, got.G}, {want.B, got.B}, {want.A, got.A}} {
				if diff := int(pair[0]) - int(pair[1]); diff > tolerance || diff < -tolerance {
					t.Fatalf("orientation %d: stored pixel (%d,%d) channel %d is %d, upright shows %d", orientation, x, y, i, pair[0], pair[1])
				}
			}
		}
	}
}

func quadrantImage(size image.Point) *image.RGBA {
	img := image.NewRGBA(image.Rectangle{Max: size})
	colours := [2][2]color.RGBA{
		{{R: 0xff, A: 0xff}, {G: 0xff, A: 0xff}},
		{{B: 0xff, A: 0xff}, {R: 0xff, G: 0xff, B: 0xff, A: 0xff}},
	}
	for y := range size.Y {
		for x := range size.X {
			img.SetRGBA(x, y, colours[min(2*y/size.Y, 1)][min(2*x/size.X, 1)])
		}
	}
	return img
}

func TestApplyOrientation_TurnsThePictureUpright(t *testing.T) {
	stored := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i, letter := range "abcdef" {
		stored.SetRGBA(i%3, i/3, color.RGBA{R: uint8(letter), A: 0xff})
	}
	letterRows := func(img image.Image) []string {
		bounds := img.Bounds()
		rows := make([]string, 0, bounds.Dy())
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			row := make([]byte, 0, bounds.Dx())
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				row = append(row, rgbaAt(img, image.Pt(x, y)).R)
			}
			rows = append(rows, string(row))
		}
		return rows
	}

	cases := map[int][]string{
		orientationFlipX:          {"cba", "fed"},
		orientationTurn180:        {"fed", "cba"},
		orientationFlipY:          {"def", "abc"},
		orientationFlipXTurnLeft:  {"ad", "be", "cf"},
		orientationTurnRight:      {"da", "eb", "fc"},
		orientationFlipXTurnRight: {"fc", "eb", "da"},
		orientationTurnLeft:       {"cf", "be", "ad"},
	}
	for orientation, want := range cases {
		assert.Equal(t, want, letterRows(applyOrientation(stored, orientation)), "orientation %d", orientation)
	}
}

func TestApplyOrientation_LeavesUprightAndUnknownValuesAlone(t *testing.T) {
	stored := patternImage(image.Pt(5, 3))
	for _, orientation := range []int{-1, 0, 1, 9, 255, 1 << 16} {
		assert.Same(t, stored, applyOrientation(stored, orientation), "orientation %d", orientation)
	}
}

func TestApplyOrientation_MovesEveryPixel(t *testing.T) {
	for _, size := range []image.Point{{1, 1}, {1, 7}, {7, 1}, {2, 2}, {5, 3}, {4, 9}, {16, 16}} {
		stored := patternImage(size)
		for _, orientation := range allOrientations {
			assertUpright(t, stored, applyOrientation(stored, orientation), orientation, 0)
		}
	}
}

func TestApplyOrientation_UndoesItselfInPairs(t *testing.T) {
	stored := patternImage(image.Pt(7, 4))
	pairs := [][2]int{
		{orientationFlipX, orientationFlipX},
		{orientationTurn180, orientationTurn180},
		{orientationFlipY, orientationFlipY},
		{orientationFlipXTurnLeft, orientationFlipXTurnLeft},
		{orientationFlipXTurnRight, orientationFlipXTurnRight},
		{orientationTurnRight, orientationTurnLeft},
		{orientationTurnLeft, orientationTurnRight},
	}
	for _, pair := range pairs {
		back := applyOrientation(applyOrientation(stored, pair[0]), pair[1])
		assertUpright(t, stored, back, 1, 0)
	}

	twice := applyOrientation(applyOrientation(stored, orientationTurnRight), orientationTurnRight)
	assert.Equal(t, applyOrientation(stored, orientationTurn180), twice, "two right turns are one half turn")
}

func TestApplyOrientation_ReadsEveryDecodedKind(t *testing.T) {
	base := patternImage(image.Pt(6, 4))
	paletted := image.NewPaletted(base.Rect, color.Palette{color.Black, color.White, color.RGBA{R: 0xff, A: 0xff}, color.RGBA{B: 0xff, A: 0xff}})
	for i := range paletted.Pix {
		paletted.Pix[i] = uint8(i % 4)
	}
	kinds := map[string]draw.Image{
		"rgba":     image.NewRGBA(base.Rect),
		"nrgba":    image.NewNRGBA(base.Rect),
		"gray":     image.NewGray(base.Rect),
		"gray16":   image.NewGray16(base.Rect),
		"cmyk":     image.NewCMYK(base.Rect),
		"rgba64":   image.NewRGBA64(base.Rect),
		"paletted": paletted,
	}
	for name, kind := range kinds {
		if name != "paletted" {
			draw.Draw(kind, kind.Bounds(), base, image.Point{}, draw.Src)
		}
		for _, orientation := range allOrientations {
			t.Run(fmt.Sprintf("%s/%d", name, orientation), func(t *testing.T) {
				assertUpright(t, kind, applyOrientation(kind, orientation), orientation, 0)
			})
		}
	}

	for _, ratio := range []image.YCbCrSubsampleRatio{image.YCbCrSubsampleRatio444, image.YCbCrSubsampleRatio422, image.YCbCrSubsampleRatio420, image.YCbCrSubsampleRatio440} {
		ycbcr := image.NewYCbCr(base.Rect, ratio)
		for y := range base.Rect.Dy() {
			for x := range base.Rect.Dx() {
				c := base.RGBAAt(x, y)
				yy, cb, cr := color.RGBToYCbCr(c.R, c.G, c.B)
				ycbcr.Y[ycbcr.YOffset(x, y)] = yy
				ycbcr.Cb[ycbcr.COffset(x, y)] = cb
				ycbcr.Cr[ycbcr.COffset(x, y)] = cr
			}
		}
		for _, orientation := range allOrientations {
			t.Run(fmt.Sprintf("ycbcr %v/%d", ratio, orientation), func(t *testing.T) {
				assertUpright(t, ycbcr, applyOrientation(ycbcr, orientation), orientation, 1)
			})
		}
	}
}

func TestApplyOrientation_TurnsASubImageByItsOwnBounds(t *testing.T) {
	sub := patternImage(image.Pt(9, 7)).SubImage(image.Rect(2, 1, 7, 4))
	for _, orientation := range allOrientations[1:] {
		assertUpright(t, sub, applyOrientation(sub, orientation), orientation, 0)
	}
}

func TestExtractOrientation_ReadsEveryValueInBothByteOrders(t *testing.T) {
	plain := encodeJPEG(t, patternImage(image.Pt(8, 8)), 90)
	for _, order := range []binary.AppendByteOrder{binary.LittleEndian, binary.BigEndian} {
		for _, orientation := range allOrientations {
			file := insertSegments(plain, exifSegment(orientationTIFF(order, orientation)))
			assert.Equal(t, orientation, extractOrientation(file), "%v orientation %d", order, orientation)
		}
	}
}

func TestExtractOrientation_ReadsWhatALibraryWrites(t *testing.T) {
	for _, orientation := range allOrientations {
		assert.Equal(t, orientation, extractOrientation(jpegWithOrientation(t, encodeJPEG(t, patternImage(image.Pt(8, 8)), 90), orientation)))
	}
}

func TestExtractOrientation_FindsTheExifSegmentAmongOthers(t *testing.T) {
	plain := encodeJPEG(t, patternImage(image.Pt(8, 8)), 90)
	jfif := jpegSegment(0xe0, []byte("JFIF\x00\x01\x02\x00\x00\x01\x00\x01\x00\x00"))
	xmp := jpegSegment(0xe1, []byte("http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta tiff:Orientation=\"3\"/>"))
	icc := jpegSegment(0xe2, bytes.Repeat([]byte{0x42}, 60000))
	exifSix := exifSegment(orientationTIFF(binary.BigEndian, orientationTurnRight))

	assert.Equal(t, orientationTurnRight, extractOrientation(insertSegments(plain, jfif, xmp, icc, exifSix)))
	assert.Equal(t, orientationTurnRight, extractOrientation(insertSegments(plain, exifSix, exifSegment(orientationTIFF(binary.BigEndian, orientationTurnLeft)))), "the first EXIF segment counts")
	assert.Zero(t, extractOrientation(insertSegments(plain, jfif, xmp)), "an XMP orientation is not EXIF")
}

func TestExtractOrientation_ThePicturesTagWinsOverTheThumbnails(t *testing.T) {
	plain := encodeJPEG(t, patternImage(image.Pt(8, 8)), 90)
	tiff := buildTIFF(binary.LittleEndian,
		[]tiffEntry{{tag: tiffOrientation, kind: tiffShort, count: 1, value: orientationTurnRight}},
		[]tiffEntry{{tag: tiffOrientation, kind: tiffShort, count: 1, value: 1}},
	)
	assert.Equal(t, orientationTurnRight, extractOrientation(insertSegments(plain, exifSegment(tiff))))
}

func TestExtractOrientation_FindsNothingWhereThereIsNone(t *testing.T) {
	plain := encodeJPEG(t, patternImage(image.Pt(8, 8)), 90)
	var pngFile, gifFile bytes.Buffer
	require.NoError(t, png.Encode(&pngFile, patternImage(image.Pt(8, 8))))
	require.NoError(t, gif.Encode(&gifFile, patternImage(image.Pt(8, 8)), nil))
	scan := append([]byte{0xff, 0xd8}, jpegSegment(0xda, []byte{0x01, 0x01, 0x00, 0x00, 0x3f, 0x00})...)
	scan = append(scan, 0x12, 0x34, 0x56)
	scan = append(scan, exifSegment(orientationTIFF(binary.LittleEndian, orientationTurnRight))...)

	cases := map[string][]byte{
		"nothing":                    nil,
		"one byte":                   {0xff},
		"a bare start marker":        {0xff, 0xd8},
		"a jpeg without exif":        plain,
		"an uploaded warpnet file":   jpegWithMetadata(t, "alice", 0x40),
		"a png":                      pngFile.Bytes(),
		"a gif":                      gifFile.Bytes(),
		"exif after the scan starts": scan,
		"exif with no orientation": insertSegments(plain, exifSegment(buildTIFF(binary.LittleEndian,
			[]tiffEntry{{tag: tiffImageWidth, kind: tiffShort, count: 1, value: 8}}))),
		"orientation stored as a long": insertSegments(plain, exifSegment(buildTIFF(binary.LittleEndian,
			[]tiffEntry{{tag: tiffOrientation, kind: tiffLong, count: 1, value: orientationTurnRight}}))),
		"orientation with two values": insertSegments(plain, exifSegment(buildTIFF(binary.LittleEndian,
			[]tiffEntry{{tag: tiffOrientation, kind: tiffShort, count: 2, value: orientationTurnRight}}))),
		"a segment shorter than its length": {0xff, 0xd8, 0xff, 0xe1, 0xff, 0xff, 'E', 'x', 'i', 'f'},
		"a segment length below two":        {0xff, 0xd8, 0xff, 0xe1, 0x00, 0x01, 0xff, 0xd9},
		"the end marker first":              append([]byte{0xff, 0xd8, 0xff, 0xd9, 0x00, 0x00}, exifSegment(orientationTIFF(binary.LittleEndian, orientationTurnRight))...),
		"an unknown byte order":             insertSegments(plain, exifSegment(append([]byte("XX\x2a\x00"), orientationTIFF(binary.LittleEndian, orientationTurnRight)[4:]...))),
		"an ifd past the end":               insertSegments(plain, exifSegment([]byte("II\x2a\x00\xff\xff\x00\x00"))),
		"an exif header with no tiff":       insertSegments(plain, jpegSegment(0xe1, []byte("Exif\x00\x00"))),
	}
	for name, file := range cases {
		assert.Zero(t, extractOrientation(file), name)
	}
}

func TestFindJPEGExif_ReturnsTheTIFFBlock(t *testing.T) {
	tiff := orientationTIFF(binary.BigEndian, orientationTurnLeft)
	file := insertSegments(encodeJPEG(t, patternImage(image.Pt(8, 8)), 90), jpegSegment(0xe0, []byte("JFIF\x00")), exifSegment(tiff))
	assert.Equal(t, tiff, findJPEGExif(file))
}

func TestSignUploadedImage_TurnsAPhoneShotUpright(t *testing.T) {
	for _, size := range []image.Point{{64, 32}, {49, 31}} {
		stored := quadrantImage(size)
		for _, orientation := range allOrientations {
			upright := uploadImage(t, jpegWithOrientation(t, encodeJPEG(t, stored, 100), orientation), "image/jpeg")
			require.Equal(t, image.Rectangle{Max: uprightSize(orientation, size)}, upright.Bounds(), "orientation %d", orientation)
			for _, centre := range []image.Point{{size.X / 4, size.Y / 4}, {3 * size.X / 4, size.Y / 4}, {size.X / 4, 3 * size.Y / 4}, {3 * size.X / 4, 3 * size.Y / 4}} {
				want := stored.RGBAAt(centre.X, centre.Y)
				got := rgbaAt(upright, uprightPoint(orientation, centre, size))
				for _, pair := range [][2]uint8{{want.R, got.R}, {want.G, got.G}, {want.B, got.B}} {
					assert.InDelta(t, pair[0], pair[1], 24, "orientation %d, the quadrant at %v", orientation, centre)
				}
			}
		}
	}
}

func TestSignUploadedImage_LeavesFormatsWithoutExifAlone(t *testing.T) {
	stored := quadrantImage(image.Pt(48, 32))
	var pngFile, gifFile bytes.Buffer
	require.NoError(t, png.Encode(&pngFile, stored))
	require.NoError(t, gif.Encode(&gifFile, stored, nil))

	for mime, file := range map[string][]byte{"image/png": pngFile.Bytes(), "image/gif": gifFile.Bytes()} {
		upright := uploadImage(t, file, mime)
		assert.Equal(t, stored.Rect, upright.Bounds(), mime)
		assert.InDelta(t, 0xff, rgbaAt(upright, image.Pt(12, 8)).R, 24, "%s keeps red at the top left", mime)
		assert.InDelta(t, 0xff, rgbaAt(upright, image.Pt(36, 8)).G, 24, "%s keeps green at the top right", mime)
	}
}

func TestTranscodeToJPEG_IgnoresAnUprightTag(t *testing.T) {
	plain := encodeJPEG(t, patternImage(image.Pt(40, 24)), 90)
	want, err := transcodeToJPEG(plain)
	require.NoError(t, err)

	for _, file := range [][]byte{
		insertSegments(plain, exifSegment(orientationTIFF(binary.LittleEndian, 1))),
		insertSegments(plain, exifSegment(buildTIFF(binary.LittleEndian, []tiffEntry{{tag: tiffImageWidth, kind: tiffShort, count: 1, value: 40}}))),
		jpegWithOrientation(t, plain, 1),
	} {
		got, err := transcodeToJPEG(file)
		require.NoError(t, err)
		assert.Equal(t, want, got, "an upright or untagged photo is stored exactly as before")
	}
}

func TestTranscodeToJPEG_TurnsOnlyByTheTag(t *testing.T) {
	stored := quadrantImage(image.Pt(48, 32))
	plain := encodeJPEG(t, stored, 100)
	for _, orientation := range allOrientations {
		out, err := transcodeToJPEG(insertSegments(plain, exifSegment(orientationTIFF(binary.BigEndian, orientation))))
		require.NoError(t, err)
		upright := decodeJPEG(t, out)
		require.Equal(t, image.Rectangle{Max: uprightSize(orientation, stored.Rect.Size())}, upright.Bounds())
		want := stored.RGBAAt(12, 8)
		got := rgbaAt(upright, uprightPoint(orientation, image.Pt(12, 8), stored.Rect.Size()))
		assert.InDelta(t, want.R, got.R, 24, "orientation %d moves the red quadrant", orientation)
		assert.InDelta(t, want.G, got.G, 24, "orientation %d moves the red quadrant", orientation)
	}
}

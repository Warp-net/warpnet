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
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/security"
	"github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	jis "github.com/dsoprea/go-jpeg-image-structure/v2"
)

const ImageDescriptionTag = "ImageDescription"

const (
	ErrInvalidEXIF   warpnet.WarpError = "invalid exif type: not a segment list"
	ErrMalformedJPEG warpnet.WarpError = "malformed JPEG file"
)

const (
	jpegMarker  = 0xFF
	jpegSOI     = 0xD8
	jpegAPP1    = 0xE1
	jpegSOS     = 0xDA
	jpegEOI     = 0xD9
	jpegTEM     = 0x01
	jpegRST0    = 0xD0
	jpegRST7    = 0xD7
	jpegHeadLen = 4 // 0xFF, marker, two length bytes
)

var exifSegmentHeader = []byte("Exif\x00\x00")

func EmbedInJPEG(imageBytes, watermarkBytes []byte) ([]byte, error) {
	segments, err := parseJPEGSegments(imageBytes)
	if err != nil {
		return nil, err
	}

	rootIb, err := newDescriptionIfd(watermarkBytes)
	if err != nil {
		return nil, err
	}

	if err := segments.SetExif(rootIb); err != nil {
		return nil, fmt.Errorf("amend EXIF: set: %w", err)
	}

	buf := new(bytes.Buffer)
	if err := segments.Write(buf); err != nil {
		return nil, fmt.Errorf("amend EXIF: write bytes: %w", err)
	}
	return buf.Bytes(), nil
}

func VerifyImage(jpegBytes []byte, nodeId, ownerId string) error {
	watermarkBytes, err := extractFromJPEG(jpegBytes)
	if err != nil {
		return err
	}
	raw, err := stripJPEGExif(jpegBytes)
	if err != nil {
		return err
	}
	return verify(watermarkBytes, security.ConvertToSHA256(raw), nodeId, ownerId)
}

func extractFromJPEG(b []byte) ([]byte, error) {
	rawExif, err := exif.SearchAndExtractExif(b)
	if err != nil {
		return nil, ErrNoMetadata
	}
	tags, _, err := exif.GetFlatExifData(rawExif, nil)
	if err != nil {
		return nil, ErrNoMetadata
	}

	encoded, ok := imageDescription(tags)
	if !ok {
		return nil, ErrNoMetadata
	}

	watermarkBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, ErrNoMetadata
	}
	return watermarkBytes, nil
}

func stripJPEGExif(b []byte) ([]byte, error) {
	if len(b) < 2 || b[0] != jpegMarker || b[1] != jpegSOI {
		return nil, ErrMalformedJPEG
	}

	out := bytes.NewBuffer(make([]byte, 0, len(b)))
	out.Write(b[:2])

	offset := 2
	for offset+jpegHeadLen <= len(b) {
		if b[offset] != jpegMarker {
			break
		}
		marker := b[offset+1]
		if marker == jpegSOS || marker == jpegEOI {
			break // entropy-coded data follows, no segments left to skip
		}
		if marker == jpegTEM || (marker >= jpegRST0 && marker <= jpegRST7) {
			out.Write(b[offset : offset+2])
			offset += 2
			continue
		}

		size := int(binary.BigEndian.Uint16(b[offset+2 : offset+jpegHeadLen]))
		if size < 2 || offset+2+size > len(b) {
			return nil, ErrMalformedJPEG
		}
		segment := b[offset : offset+2+size]
		if !isExifSegment(marker, segment) {
			out.Write(segment)
		}
		offset += 2 + size
	}

	out.Write(b[offset:])
	return out.Bytes(), nil
}

func isExifSegment(marker byte, segment []byte) bool {
	return marker == jpegAPP1 &&
		len(segment) >= jpegHeadLen+len(exifSegmentHeader) &&
		bytes.Equal(segment[jpegHeadLen:jpegHeadLen+len(exifSegmentHeader)], exifSegmentHeader)
}

func parseJPEGSegments(imageBytes []byte) (*jis.SegmentList, error) {
	intfc, err := jis.NewJpegMediaParser().ParseBytes(imageBytes)
	if err != nil {
		return nil, fmt.Errorf("amend EXIF: parse bytes: %w", err)
	}

	segments, ok := intfc.(*jis.SegmentList)
	if !ok {
		return nil, fmt.Errorf("amend EXIF: %w", ErrInvalidEXIF)
	}
	return segments, nil
}

func newDescriptionIfd(watermarkBytes []byte) (*exif.IfdBuilder, error) {
	ifdMapping, err := exifcommon.NewIfdMappingWithStandard()
	if err != nil {
		return nil, fmt.Errorf("amend EXIF: new IFD mapping: %w", err)
	}

	tagIndex := exif.NewTagIndex()
	if err := exif.LoadStandardTags(tagIndex); err != nil {
		return nil, fmt.Errorf("amend EXIF: load standard tags: %w", err)
	}

	identity := exifcommon.NewIfdIdentity(
		exifcommon.IfdStandardIfdIdentity.IfdTag(),
		exifcommon.IfdIdentityPart{
			Name:  exifcommon.IfdStandardIfdIdentity.Name(),
			Index: exifcommon.IfdStandardIfdIdentity.Index(),
		},
	)

	rootIb := exif.NewIfdBuilder(ifdMapping, tagIndex, identity, exifcommon.EncodeDefaultByteOrder)

	encoded := base64.StdEncoding.EncodeToString(watermarkBytes)
	if err := rootIb.SetStandardWithName(ImageDescriptionTag, encoded); err != nil {
		return nil, fmt.Errorf("amend EXIF: add standard tag: %w", err)
	}
	return rootIb, nil
}

func imageDescription(tags []exif.ExifTag) (string, bool) {
	for _, tag := range tags {
		if tag.TagName != ImageDescriptionTag {
			continue
		}
		if encoded, ok := tag.Value.(string); ok {
			return strings.TrimRight(encoded, "\x00"), true
		}
	}
	return "", false
}

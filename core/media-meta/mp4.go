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
	"math"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/security"
)

const (
	boxHeaderSize   = 8
	boxUUIDSize     = 16
	boxLargeHeadLen = 16 // 8-byte header + 64-bit largesize

	uuidBoxType = "uuid"

	ErrMalformedISO     warpnet.WarpError = "malformed ISO base media file"
	ErrMetadataTooLarge warpnet.WarpError = "media meta: metadata box too large"
)

var warpnetUUID = [16]byte{
	0x77, 0x61, 0x72, 0x70, 0x6e, 0x65, 0x74, 0x00, // "warpnet\0"
	0x6d, 0x65, 0x74, 0x61, 0x00, 0x00, 0x00, 0x01, // "meta\0\0\0\1"
}

func IsISOBaseMediaFile(b []byte) bool {
	for offset := 0; offset+boxHeaderSize <= len(b); {
		size := int(binary.BigEndian.Uint32(b[offset : offset+4]))
		boxType := string(b[offset+4 : offset+boxHeaderSize])

		if boxType == "ftyp" {
			return true
		}
		if boxType != "wide" && boxType != "free" && boxType != "skip" {
			return false
		}
		if size < boxHeaderSize {
			return false
		}
		offset += size
	}
	return false
}

func EmbedInVideo(videoBytes, watermarkBytes []byte) ([]byte, error) {
	box, err := newWarpnetBox(watermarkBytes)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(make([]byte, 0, len(videoBytes)+len(box)))
	buf.Write(videoBytes)
	buf.Write(box)
	return buf.Bytes(), nil
}

func SplitVideo(b []byte) (raw, watermarkBytes []byte, err error) {
	boxes, err := walkBoxes(b)
	if err != nil {
		return nil, nil, err
	}

	raw, encoded, found := partitionBoxes(b, boxes)
	if found > 1 {
		return nil, nil, ErrAmbiguousMetadata
	}
	if found == 0 {
		return raw, nil, nil
	}

	watermarkBytes, err = base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, nil, ErrNoMetadata
	}
	return raw, watermarkBytes, nil
}

func CloseOpenEndedBox(b []byte) ([]byte, error) {
	boxes, err := walkBoxes(b)
	if err != nil {
		return nil, err
	}
	if len(boxes) == 0 {
		return b, nil
	}

	last := boxes[len(boxes)-1]
	if binary.BigEndian.Uint32(b[last.offset:last.offset+4]) != 0 {
		return b, nil
	}
	if uint64(last.size) > math.MaxUint32 { //nolint:gosec // sizes are file offsets, never negative
		return nil, ErrMalformedISO
	}

	out := make([]byte, len(b))
	copy(out, b)
	binary.BigEndian.PutUint32(out[last.offset:last.offset+4], uint32(last.size)) //nolint:gosec
	return out, nil
}

func VerifyVideo(videoBytes []byte, nodeId, ownerId string) error {
	raw, watermarkBytes, err := SplitVideo(videoBytes)
	if err != nil {
		return err
	}
	return verify(watermarkBytes, security.ConvertToSHA256(raw), nodeId, ownerId)
}

type isoBox struct {
	offset, size int
	boxType      string
	payload      []byte
}

func walkBoxes(b []byte) ([]isoBox, error) {
	boxes := make([]isoBox, 0, 8) //nolint:mnd
	for offset := 0; offset+boxHeaderSize <= len(b); {
		size, headerSize, err := boxBounds(b, offset)
		if err != nil {
			return nil, err
		}

		boxes = append(boxes, isoBox{
			offset:  offset,
			size:    size,
			boxType: string(b[offset+4 : offset+boxHeaderSize]),
			payload: b[offset+headerSize : offset+size],
		})
		offset += size
	}
	return boxes, nil
}

func boxBounds(b []byte, offset int) (size, headerSize int, err error) {
	size = int(binary.BigEndian.Uint32(b[offset : offset+4]))
	headerSize = boxHeaderSize

	switch size {
	case 1: // a 64-bit largesize follows the header
		if offset+boxLargeHeadLen > len(b) {
			return 0, 0, ErrMalformedISO
		}
		large := binary.BigEndian.Uint64(b[offset+boxHeaderSize : offset+boxLargeHeadLen])
		if large > uint64(len(b)-offset) { //nolint:gosec // offset is bounded by len(b)
			return 0, 0, ErrMalformedISO
		}
		size = int(large) //nolint:gosec // bounded by the file length just above
		headerSize = boxLargeHeadLen
	case 0: // the box runs to the end of the file
		size = len(b) - offset
	}

	if size < headerSize || offset+size > len(b) {
		return 0, 0, ErrMalformedISO
	}
	return size, headerSize, nil
}

func isWarpnetBox(box isoBox) bool {
	return box.boxType == uuidBoxType &&
		len(box.payload) >= boxUUIDSize &&
		bytes.Equal(box.payload[:boxUUIDSize], warpnetUUID[:])
}

func newWarpnetBox(watermarkBytes []byte) ([]byte, error) {
	encoded := base64.StdEncoding.EncodeToString(watermarkBytes)

	boxSize := boxHeaderSize + boxUUIDSize + len(encoded)
	if boxSize > math.MaxUint32 {
		return nil, ErrMetadataTooLarge
	}

	buf := bytes.NewBuffer(make([]byte, 0, boxSize))

	header := make([]byte, boxHeaderSize)
	binary.BigEndian.PutUint32(header, uint32(boxSize)) //nolint:gosec
	copy(header[4:], uuidBoxType)

	buf.Write(header)
	buf.Write(warpnetUUID[:])
	buf.WriteString(encoded)
	return buf.Bytes(), nil
}

func partitionBoxes(b []byte, boxes []isoBox) (raw []byte, encoded string, found int) {
	buf := bytes.NewBuffer(make([]byte, 0, len(b)))
	for _, box := range boxes {
		if isWarpnetBox(box) {
			found++
			encoded = string(box.payload[boxUUIDSize:])
			continue
		}
		buf.Write(b[box.offset : box.offset+box.size])
	}
	return buf.Bytes(), encoded, found
}

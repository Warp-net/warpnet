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
	"encoding/binary"
	"math"

	"github.com/Warp-net/warpnet/core/warpnet"
)

const (
	chunkOffsetsHeaderSize = 8

	errUnmovableMoov warpnet.WarpError = "video: moov cannot be moved"
)

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

func MoveMoovToFront(b []byte) ([]byte, error) {
	boxes, err := walkBoxes(b)
	if err != nil {
		return nil, err
	}

	moovIdx, mdatIdx := -1, -1
	for i, box := range boxes {
		switch box.boxType {
		case "moof":
			return b, nil
		case moovType:
			if moovIdx < 0 {
				moovIdx = i
			}
		case "mdat":
			if mdatIdx < 0 {
				mdatIdx = i
			}
		}
	}
	if moovIdx < 0 || mdatIdx < 0 || moovIdx < mdatIdx {
		return b, nil
	}

	moov := boxes[moovIdx]
	from, to := boxes[mdatIdx].offset, moov.offset

	movedMoov := make([]byte, moov.size)
	copy(movedMoov, b[moov.offset:moov.offset+moov.size])
	if err := shiftChunkOffsets(movedMoov, from, to, moov.size); err != nil {
		return b, nil //nolint:nilerr // a file whose offsets cannot be moved stays as it was
	}

	out := make([]byte, 0, len(b))
	out = append(out, b[:from]...)
	out = append(out, movedMoov...)
	out = append(out, b[from:moov.offset]...)
	out = append(out, b[moov.offset+moov.size:]...)
	return out, nil
}

func shiftChunkOffsets(b []byte, from, to, shift int) error {
	boxes, err := walkBoxes(b)
	if err != nil {
		return err
	}
	for _, box := range boxes {
		switch box.boxType {
		case moovType, "trak", "mdia", "minf", "stbl":
			if err := shiftChunkOffsets(box.payload, from, to, shift); err != nil {
				return err
			}
		case stcoType:
			if err := shiftOffsets(box.payload, 4, from, to, shift); err != nil { //nolint:mnd
				return err
			}
		case co64Type:
			if err := shiftOffsets(box.payload, 8, from, to, shift); err != nil { //nolint:mnd
				return err
			}
		case "saio":
			return errUnmovableMoov
		}
	}
	return nil
}

func shiftOffsets(payload []byte, entrySize, from, to, shift int) error {
	if len(payload) < chunkOffsetsHeaderSize {
		return ErrMalformedISO
	}
	count := int(binary.BigEndian.Uint32(payload[4:chunkOffsetsHeaderSize]))
	if count > (len(payload)-chunkOffsetsHeaderSize)/entrySize {
		return ErrMalformedISO
	}

	for i := range count {
		entry := payload[chunkOffsetsHeaderSize+i*entrySize : chunkOffsetsHeaderSize+(i+1)*entrySize]
		if entrySize == 4 { //nolint:mnd
			offset := uint64(binary.BigEndian.Uint32(entry))
			if offset < uint64(from) || offset >= uint64(to) { //nolint:gosec // file offsets are never negative
				continue
			}
			offset += uint64(shift) //nolint:gosec // file offsets are never negative
			if offset > math.MaxUint32 {
				return errUnmovableMoov
			}
			binary.BigEndian.PutUint32(entry, uint32(offset))
			continue
		}
		offset := binary.BigEndian.Uint64(entry)
		if offset < uint64(from) || offset >= uint64(to) { //nolint:gosec // file offsets are never negative
			continue
		}
		binary.BigEndian.PutUint64(entry, offset+uint64(shift)) //nolint:gosec // file offsets are never negative
	}
	return nil
}

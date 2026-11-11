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
	"bytes"
	"encoding/binary"
	"math"

	"github.com/Warp-net/warpnet/core/warpnet"
)

const (
	uuidType = "uuid"
	uuidSize = 16

	ErrBoxTooLarge warpnet.WarpError = "video: box too large"
)

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

func AppendUUIDBox(b []byte, id [uuidSize]byte, payload []byte) ([]byte, error) {
	size := boxHeaderSize + uuidSize + len(payload)
	if size > math.MaxUint32 {
		return nil, ErrBoxTooLarge
	}
	out := make([]byte, 0, len(b)+size)
	out = append(out, b...)
	out = binary.BigEndian.AppendUint32(out, uint32(size)) //nolint:gosec // checked just above
	out = append(out, uuidType...)
	out = append(out, id[:]...)
	return append(out, payload...), nil
}

func CutUUIDBoxes(b []byte, id [uuidSize]byte) (rest []byte, payloads [][]byte, err error) {
	boxes, err := walkBoxes(b)
	if err != nil {
		return nil, nil, err
	}
	rest = make([]byte, 0, len(b))
	for _, box := range boxes {
		if box.boxType == uuidType && len(box.payload) >= uuidSize && bytes.Equal(box.payload[:uuidSize], id[:]) {
			payloads = append(payloads, box.payload[uuidSize:])
			continue
		}
		rest = append(rest, b[box.offset:box.offset+box.size]...)
	}
	return rest, payloads, nil
}

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
	boxHeaderSize   = 8
	boxLargeHeadLen = 16

	moovType = "moov"
	stcoType = "stco"
	co64Type = "co64"

	ErrMalformedISO warpnet.WarpError = "malformed ISO base media file"
)

type isoBox struct {
	offset, size int
	boxType      string
	payload      []byte
}

type node struct {
	boxType  string
	head     []byte
	children []*node
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
	case 1:
		if offset+boxLargeHeadLen > len(b) {
			return 0, 0, ErrMalformedISO
		}
		large := binary.BigEndian.Uint64(b[offset+boxHeaderSize : offset+boxLargeHeadLen])
		if large > uint64(len(b)-offset) { //nolint:gosec // offset is bounded by len(b)
			return 0, 0, ErrMalformedISO
		}
		size = int(large) //nolint:gosec // bounded by the file length just above
		headerSize = boxLargeHeadLen
	case 0:
		size = len(b) - offset
	}

	if size < headerSize || offset+size > len(b) {
		return 0, 0, ErrMalformedISO
	}
	return size, headerSize, nil
}

func parseNodes(b []byte) ([]*node, error) {
	boxes, err := walkBoxes(b)
	if err != nil {
		return nil, err
	}
	nodes := make([]*node, 0, len(boxes))
	for _, box := range boxes {
		n := &node{boxType: box.boxType, head: box.payload}
		headLen, isContainer := containerHeadLen(box.boxType)
		if isContainer && len(box.payload) >= headLen {
			n.head = box.payload[:headLen]
			if n.children, err = parseNodes(box.payload[headLen:]); err != nil {
				return nil, err
			}
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func containerHeadLen(boxType string) (int, bool) {
	switch boxType {
	case moovType, "trak", "mdia", "minf", "stbl", "edts":
		return 0, true
	case "stsd":
		return 8, true //nolint:mnd
	case "avc1":
		return 78, true //nolint:mnd
	}
	return 0, false
}

func (n *node) size() int {
	size := boxHeaderSize + len(n.head)
	for _, c := range n.children {
		size += c.size()
	}
	return size
}

func (n *node) appendTo(out []byte) ([]byte, error) {
	size := n.size()
	if size > math.MaxUint32 {
		return nil, ErrMalformedISO
	}
	out = binary.BigEndian.AppendUint32(out, uint32(size)) //nolint:gosec // checked just above
	out = append(out, n.boxType...)
	out = append(out, n.head...)
	for _, c := range n.children {
		var err error
		if out, err = c.appendTo(out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (n *node) find(path ...string) *node {
	current := n
	for _, boxType := range path {
		var next *node
		for _, c := range current.children {
			if c.boxType == boxType {
				next = c
				break
			}
		}
		if next == nil {
			return nil
		}
		current = next
	}
	return current
}

func (n *node) findAll(boxType string) []*node {
	found := make([]*node, 0, 2) //nolint:mnd
	for _, c := range n.children {
		if c.boxType == boxType {
			found = append(found, c)
		}
	}
	return found
}

func (n *node) remove(boxType string) {
	kept := n.children[:0]
	for _, c := range n.children {
		if c.boxType != boxType {
			kept = append(kept, c)
		}
	}
	n.children = kept
}

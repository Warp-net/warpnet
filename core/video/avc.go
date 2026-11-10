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

	"github.com/Warp-net/warpnet/core/warpnet"
)

const (
	maxParameterSets  = 31
	maxParameterSetId = 31

	nalTypeSEI = 6
	nalTypeAUD = 9

	ErrParameterSetsFull warpnet.WarpError = "video: no free H.264 parameter set id"
)

type avcConfig struct {
	profile    byte
	compat     byte
	level      byte
	lengthSize int
	sps        [][]byte
	pps        [][]byte
	tail       []byte
}

func parseAVCConfig(b []byte) (avcConfig, error) {
	r := &reader{b: b}
	if r.u8() != 1 {
		return avcConfig{}, ErrUnsupportedVideo
	}
	c := avcConfig{profile: r.u8(), compat: r.u8(), level: r.u8(), lengthSize: int(r.u8()&3) + 1}
	spsCount := int(r.u8() & 0x1f) //nolint:mnd
	for range spsCount {
		c.sps = append(c.sps, r.next(int(r.u16())))
	}
	ppsCount := int(r.u8())
	for range ppsCount {
		c.pps = append(c.pps, r.next(int(r.u16())))
	}
	if r.err != nil {
		return avcConfig{}, r.err
	}
	c.tail = r.b[r.off:]
	if len(c.sps) == 0 || len(c.pps) == 0 {
		return avcConfig{}, ErrUnsupportedVideo
	}
	return c, nil
}

func (c *avcConfig) bytes() ([]byte, error) {
	if len(c.sps) > maxParameterSets || len(c.pps) > 0xff {
		return nil, ErrParameterSetsFull
	}
	out := []byte{1, c.profile, c.compat, c.level, 0xfc | byte(c.lengthSize-1), 0xe0 | byte(len(c.sps))} //nolint:gosec // bounded above
	for _, sps := range c.sps {
		out = binary.BigEndian.AppendUint16(out, uint16(len(sps))) //nolint:gosec // a parameter set is short
		out = append(out, sps...)
	}
	out = append(out, byte(len(c.pps))) //nolint:gosec // bounded above
	for _, pps := range c.pps {
		out = binary.BigEndian.AppendUint16(out, uint16(len(pps))) //nolint:gosec // a parameter set is short
		out = append(out, pps...)
	}
	return append(out, c.tail...), nil
}

func (c *avcConfig) findFreeIds(count int) ([]int, error) {
	used := make([]bool, maxParameterSetId+1)
	for _, sps := range c.sps {
		if id, err := readSPSId(sps); err == nil && id <= maxParameterSetId {
			used[id] = true
		}
	}
	for _, pps := range c.pps {
		if id, err := readPPSId(pps); err == nil && id <= maxParameterSetId {
			used[id] = true
		}
	}
	ids := make([]int, 0, count)
	for id := range used {
		if !used[id] && len(ids) < count {
			ids = append(ids, id)
		}
	}
	if len(ids) < count || len(c.sps)+count > maxParameterSets {
		return nil, ErrParameterSetsFull
	}
	return ids, nil
}

func (c *avcConfig) addParameterSets(sps, pps []byte) error {
	spsId, err := readSPSId(sps)
	if err != nil {
		return err
	}
	ppsId, err := readPPSId(pps)
	if err != nil {
		return err
	}
	if err := addParameterSet(&c.sps, sps, spsId, readSPSId); err != nil {
		return err
	}
	return addParameterSet(&c.pps, pps, ppsId, readPPSId)
}

func addParameterSet(sets *[][]byte, set []byte, id int, readId func([]byte) (int, error)) error {
	for _, existing := range *sets {
		existingId, err := readId(existing)
		if err != nil || existingId != id {
			continue
		}
		if bytes.Equal(existing, set) {
			return nil
		}
		return ErrParameterSetsFull
	}
	*sets = append(*sets, set)
	return nil
}

func readSPSId(nal []byte) (int, error) {
	r := newBitReader(nal)
	r.skip(8 + 24) //nolint:mnd
	return r.ue()
}

func readPPSId(nal []byte) (int, error) {
	r := newBitReader(nal)
	r.skip(8) //nolint:mnd
	return r.ue()
}

type bitReader struct {
	b   []byte
	pos int
}

func newBitReader(nal []byte) *bitReader {
	rbsp := make([]byte, 0, len(nal))
	zeros := 0
	for _, v := range nal {
		if zeros >= 2 && v == 3 { //nolint:mnd
			zeros = 0
			continue
		}
		rbsp = append(rbsp, v)
		if v == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return &bitReader{b: rbsp}
}

func (r *bitReader) skip(n int) { r.pos += n }

func (r *bitReader) bit() (int, error) {
	if r.pos >= len(r.b)*8 {
		return 0, ErrMalformedISO
	}
	v := int(r.b[r.pos/8]>>(7-uint(r.pos%8))) & 1 //nolint:gosec,mnd // pos is never negative
	r.pos++
	return v, nil
}

func (r *bitReader) ue() (int, error) {
	zeros := 0
	for {
		v, err := r.bit()
		if err != nil {
			return 0, err
		}
		if v == 1 {
			break
		}
		zeros++
		if zeros > 31 { //nolint:mnd
			return 0, ErrMalformedISO
		}
	}
	value := 0
	for range zeros {
		v, err := r.bit()
		if err != nil {
			return 0, err
		}
		value = value<<1 | v
	}
	return 1<<zeros - 1 + value, nil
}

func convertNALs(sample []byte, fromLength, toLength int) ([]byte, error) {
	out := make([]byte, 0, len(sample))
	for off := 0; off < len(sample); {
		if off+fromLength > len(sample) {
			return nil, ErrMalformedISO
		}
		size := 0
		for _, v := range sample[off : off+fromLength] {
			size = size<<8 | int(v)
		}
		off += fromLength
		if size == 0 || off+size > len(sample) {
			return nil, ErrMalformedISO
		}
		nal := sample[off : off+size]
		off += size
		if nalType := nal[0] & 0x1f; nalType == nalTypeSEI || nalType == nalTypeAUD { //nolint:mnd
			continue
		}
		if size >= 1<<(8*toLength) {
			return nil, ErrUnsupportedVideo
		}
		for i := toLength - 1; i >= 0; i-- {
			out = append(out, byte(size>>(8*i))) //nolint:gosec // one byte of the length
		}
		out = append(out, nal...)
	}
	return out, nil
}

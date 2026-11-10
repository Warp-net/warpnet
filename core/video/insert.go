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
	"cmp"
	"encoding/binary"
	"math"
	"slices"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
)

const (
	insertedDependency = 0x28
	frameShare         = 60

	ErrNoRoomForFrame warpnet.WarpError = "video: no room for a frame before this keyframe"
	ErrBadFramePlace  warpnet.WarpError = "video: a frame can be inserted only before a keyframe"
)

func InsertFrames(b []byte, frames []domain.VideoFrame) ([]byte, error) {
	if len(frames) == 0 {
		return b, nil
	}
	b, err := CloseOpenEndedBox(b)
	if err != nil {
		return nil, err
	}
	m, err := parseMovie(b)
	if err != nil {
		return nil, err
	}
	t := &m.video

	sorted := slices.SortedFunc(slices.Values(frames), func(a, b domain.VideoFrame) int {
		return cmp.Compare(a.SampleIndex, b.SampleIndex)
	})
	if err := t.addParameterSets(sorted); err != nil {
		return nil, err
	}
	samples, err := t.spliceSamples(sorted)
	if err != nil {
		return nil, err
	}
	t.writeTables(samples)
	co64 := t.writeChunkOffsets(len(samples))

	moovBox := m.boxes[m.moovIndex]
	delta := m.moov.size() - moovBox.size
	if err := m.shiftOtherTracks(moovBox.offset, delta); err != nil {
		return nil, err
	}

	dataOffset := len(b) + delta + boxHeaderSize
	inserted := make([]byte, 0, len(frames)*64<<10) //nolint:mnd
	frame := 0
	for i, s := range samples {
		offset := s.offset
		switch {
		case offset < 0:
			offset = dataOffset + len(inserted)
			inserted = append(inserted, sorted[frame].Data...)
			frame++
		case offset > moovBox.offset:
			offset += delta
		}
		binary.BigEndian.PutUint64(co64.head[chunkOffsetsHeaderSize+8*i:], uint64(offset)) //nolint:gosec,mnd // file offsets are never negative
	}

	out := make([]byte, 0, len(b)+delta+boxHeaderSize+len(inserted))
	out = append(out, b[:moovBox.offset]...)
	if out, err = m.moov.appendTo(out); err != nil {
		return nil, err
	}
	out = append(out, b[moovBox.offset+moovBox.size:]...)
	mdat := &node{boxType: "mdat", head: inserted}
	return mdat.appendTo(out)
}

func (t *track) addParameterSets(frames []domain.VideoFrame) error {
	config, err := parseAVCConfig(t.avcC.head)
	if err != nil {
		return err
	}
	for _, f := range frames {
		if err := config.addParameterSets(f.SPS, f.PPS); err != nil {
			return err
		}
	}
	t.avcC.head, err = config.bytes()
	return err
}

func (t *track) spliceSamples(frames []domain.VideoFrame) ([]sample, error) {
	out := make([]sample, 0, len(t.samples)+len(frames))
	next := 0
	latestAt := -1
	for i, s := range t.samples {
		if next < len(frames) && frames[next].SampleIndex == i {
			if latestAt < 0 {
				return nil, ErrBadFramePlace
			}
			frame, err := t.placeFrame(out[latestAt], s)
			if err != nil {
				return nil, err
			}
			out[latestAt].duration -= frame.duration
			for k := latestAt + 1; k < len(out); k++ {
				out[k].dts -= int64(frame.duration)
				out[k].ptsShift += int64(frame.duration)
			}
			frame.size = len(frames[next].Data)
			out = append(out, frame)
			next++
		}
		if latestAt < 0 || s.pts() >= out[latestAt].pts() {
			latestAt = len(out)
		}
		out = append(out, s)
	}
	if next != len(frames) {
		return nil, ErrBadFramePlace
	}
	return out, nil
}

func (t *track) placeFrame(shown, keyframe sample) (sample, error) {
	if !keyframe.isSync {
		return sample{}, ErrBadFramePlace
	}
	taken := shown.duration * frameShare / 100 //nolint:mnd
	gap := keyframe.pts() - shown.pts()
	lead := gap - gap*frameShare/100 //nolint:mnd
	if taken == 0 || taken == shown.duration || lead <= 0 || lead >= gap {
		return sample{}, ErrNoRoomForFrame
	}
	dts := keyframe.dts - int64(taken)
	shift := shown.pts() + lead - dts
	switch {
	case !t.hasCTTS && shift != 0:
		return sample{}, ErrNoRoomForFrame
	case !t.isSignedCTTS && shift < 0:
		return sample{}, ErrNoRoomForFrame
	case shift > math.MaxInt32 || shift < math.MinInt32:
		return sample{}, ErrNoRoomForFrame
	}
	return sample{offset: -1, dts: dts, duration: taken, ptsShift: shift, isSync: true}, nil
}

func (t *track) writeTables(samples []sample) {
	isInserted := make([]bool, len(samples))
	for i, s := range samples {
		isInserted[i] = s.offset < 0
	}
	if stts := t.stbl.find("stts"); stts != nil {
		stts.head = writeRuns(0, samples, func(s sample) uint32 { return s.duration })
	}
	if ctts := t.stbl.find("ctts"); ctts != nil {
		version := byte(0)
		if t.isSignedCTTS {
			version = 1
		}
		ctts.head = writeRuns(version, samples, func(s sample) uint32 { return uint32(s.ptsShift) }) //nolint:gosec // a signed field
	}
	if stss := t.stbl.find("stss"); stss != nil {
		head := make([]byte, 8, 8+4*len(samples)) //nolint:mnd
		count := uint32(0)
		for i, s := range samples {
			if s.isSync {
				head = binary.BigEndian.AppendUint32(head, uint32(i+1)) //nolint:gosec // bounded by the sample count
				count++
			}
		}
		binary.BigEndian.PutUint32(head[4:], count)
		stss.head = head
	}
	if stsz := t.stbl.find("stsz"); stsz != nil {
		head := make([]byte, 12, 12+4*len(samples))                //nolint:mnd
		binary.BigEndian.PutUint32(head[8:], uint32(len(samples))) //nolint:gosec,mnd // bounded by the sample count
		for _, s := range samples {
			head = binary.BigEndian.AppendUint32(head, uint32(s.size)) //nolint:gosec // a sample fits a file
		}
		stsz.head = head
	}
	if stsc := t.stbl.find("stsc"); stsc != nil {
		stsc.head = []byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1}
	}
	if sdtp := t.stbl.find("sdtp"); sdtp != nil {
		sdtp.head = writeDependencies(sdtp.head, isInserted)
	}
	for _, sbgp := range t.stbl.findAll("sbgp") {
		sbgp.head = writeGroups(sbgp.head, isInserted)
	}
	t.stbl.remove("cslg")
}

func writeRuns(version byte, samples []sample, value func(sample) uint32) []byte {
	head := []byte{version, 0, 0, 0, 0, 0, 0, 0}
	count := uint32(0)
	for i := 0; i < len(samples); {
		v, run := value(samples[i]), 0
		for i < len(samples) && value(samples[i]) == v {
			run++
			i++
		}
		head = binary.BigEndian.AppendUint32(head, uint32(run)) //nolint:gosec // bounded by the sample count
		head = binary.BigEndian.AppendUint32(head, v)
		count++
	}
	binary.BigEndian.PutUint32(head[4:], count)
	return head
}

func writeDependencies(head []byte, isInserted []bool) []byte {
	original := len(isInserted)
	for _, inserted := range isInserted {
		if inserted {
			original--
		}
	}
	if len(head) != 4+original { //nolint:mnd
		return head
	}
	out := append(make([]byte, 0, 4+len(isInserted)), head[:4]...) //nolint:mnd
	next := 4
	for _, inserted := range isInserted {
		if inserted {
			out = append(out, insertedDependency)
			continue
		}
		out = append(out, head[next])
		next++
	}
	return out
}

func writeGroups(head []byte, isInserted []bool) []byte {
	r := &reader{b: head}
	version := r.u8()
	r.next(7) //nolint:mnd
	if version == 1 {
		r.next(4) //nolint:mnd
	}
	prefix := r.off
	groups := make([]uint32, 0, len(isInserted))
	for range r.count(8) { //nolint:mnd
		count, group := r.u32(), r.u32()
		for range min(int(count), len(isInserted)) {
			groups = append(groups, group)
		}
	}
	if r.err != nil {
		return head
	}

	out := slices.Clone(head[:prefix+4])
	runs := uint32(0)
	appendSample := func(group uint32) {
		last := len(out) - 4
		if runs > 0 && binary.BigEndian.Uint32(out[last:]) == group {
			binary.BigEndian.PutUint32(out[last-4:], binary.BigEndian.Uint32(out[last-4:])+1)
			return
		}
		out = binary.BigEndian.AppendUint32(out, 1)
		out = binary.BigEndian.AppendUint32(out, group)
		runs++
	}
	original := 0
	for _, inserted := range isInserted {
		if original >= len(groups) {
			break
		}
		appendSample(groups[original])
		if !inserted {
			original++
		}
	}
	binary.BigEndian.PutUint32(out[prefix:], runs)
	return out
}

func (t *track) writeChunkOffsets(count int) *node {
	co64 := &node{boxType: co64Type, head: make([]byte, chunkOffsetsHeaderSize+8*count)} //nolint:mnd
	binary.BigEndian.PutUint32(co64.head[4:], uint32(count))                             //nolint:gosec // bounded by the sample count
	for i, c := range t.stbl.children {
		if c.boxType == stcoType || c.boxType == co64Type {
			t.stbl.children[i] = co64
		}
	}
	return co64
}

func (m *movie) shiftOtherTracks(moovOffset, delta int) error {
	for _, trak := range m.moov.findAll("trak") {
		stbl := trak.find("mdia", "minf", "stbl")
		if stbl == nil || stbl == m.video.stbl {
			continue
		}
		for _, c := range stbl.children {
			var entrySize int
			switch c.boxType {
			case stcoType:
				entrySize = 4
			case co64Type:
				entrySize = 8
			default:
				continue
			}
			c.head = slices.Clone(c.head)
			if err := shiftOffsets(c.head, entrySize, moovOffset+1, math.MaxInt, delta); err != nil {
				return err
			}
		}
	}
	return nil
}

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
	"image"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
)

const (
	fixedOne = 0x10000

	ErrUnsupportedVideo warpnet.WarpError = "video: only MP4 files with one H.264 track can be marked"
)

type sample struct {
	offset   int
	size     int
	dts      int64
	duration uint32
	ptsShift int64
	isSync   bool
}

func (s sample) pts() int64 { return s.dts + s.ptsShift }

type track struct {
	stbl         *node
	avcC         *node
	timescale    uint32
	mediaTime    int64
	hasEmptyEdit bool
	size         image.Point
	rotation     int
	samples      []sample
	hasCTTS      bool
	isSignedCTTS bool
}

type movie struct {
	boxes     []isoBox
	moovIndex int
	moov      *node
	video     track
}

type reader struct {
	b   []byte
	off int
	err error
}

func (r *reader) next(n int) []byte {
	if r.err != nil || n < 0 || r.off+n > len(r.b) {
		r.err = ErrMalformedISO
		return make([]byte, max(n, 0))
	}
	out := r.b[r.off : r.off+n]
	r.off += n
	return out
}

func (r *reader) u8() byte    { return r.next(1)[0] }
func (r *reader) u16() uint16 { return binary.BigEndian.Uint16(r.next(2)) } //nolint:mnd
func (r *reader) u32() uint32 { return binary.BigEndian.Uint32(r.next(4)) } //nolint:mnd
func (r *reader) u64() uint64 { return binary.BigEndian.Uint64(r.next(8)) } //nolint:mnd
func (r *reader) count(entrySize int) int {
	n := int(r.u32())
	if r.err == nil && n > (len(r.b)-r.off)/entrySize {
		r.err = ErrMalformedISO
		return 0
	}
	return n
}

func Duration(b []byte) (time.Duration, error) {
	moov, err := findMoov(b)
	if err != nil || moov == nil {
		return 0, err
	}
	longest := time.Duration(0)
	if mvhd := moov.find("mvhd"); mvhd != nil {
		d, err := readMediaDuration(mvhd.head)
		if err != nil {
			return 0, err
		}
		longest = max(longest, d)
	}
	for _, trak := range moov.findAll("trak") {
		mdhd := trak.find("mdia", "mdhd")
		if mdhd == nil {
			continue
		}
		d, err := readMediaDuration(mdhd.head)
		if err != nil {
			return 0, err
		}
		longest = max(longest, d)
	}
	return longest, nil
}

func readMediaDuration(head []byte) (time.Duration, error) {
	r := &reader{b: head}
	version := r.u8()
	r.next(3) //nolint:mnd
	var timescale uint32
	var duration uint64
	if version == 1 {
		r.next(16) //nolint:mnd
		timescale = r.u32()
		duration = r.u64()
	} else {
		r.next(8) //nolint:mnd
		timescale = r.u32()
		duration = uint64(r.u32())
	}
	if r.err != nil {
		return 0, r.err
	}
	if timescale == 0 || duration == ^uint64(0) || duration == uint64(^uint32(0)) {
		return 0, nil
	}
	seconds := duration / uint64(timescale)
	rest := duration % uint64(timescale)
	if seconds > uint64(time.Duration(1<<62)/time.Second) {
		return time.Duration(1 << 62), nil
	}
	return time.Duration(seconds)*time.Second + time.Duration(rest*uint64(time.Second)/uint64(timescale)), nil //nolint:gosec // bounded above
}

func findMoov(b []byte) (*node, error) {
	boxes, err := walkBoxes(b)
	if err != nil {
		return nil, err
	}
	for _, box := range boxes {
		if box.boxType != moovType {
			continue
		}
		children, err := parseNodes(box.payload)
		if err != nil {
			return nil, err
		}
		return &node{boxType: moovType, children: children}, nil
	}
	return nil, nil
}

func parseMovie(b []byte) (movie, error) {
	boxes, err := walkBoxes(b)
	if err != nil {
		return movie{}, err
	}
	m := movie{boxes: boxes, moovIndex: -1}
	for i, box := range boxes {
		switch box.boxType {
		case "moof":
			return movie{}, ErrUnsupportedVideo
		case moovType:
			if m.moovIndex >= 0 {
				return movie{}, ErrUnsupportedVideo
			}
			m.moovIndex = i
		}
	}
	if m.moovIndex < 0 {
		return movie{}, ErrUnsupportedVideo
	}
	children, err := parseNodes(boxes[m.moovIndex].payload)
	if err != nil {
		return movie{}, err
	}
	m.moov = &node{boxType: moovType, children: children}

	videoTracks := 0
	for _, trak := range m.moov.findAll("trak") {
		if !isVideoTrack(trak) {
			continue
		}
		videoTracks++
		if m.video, err = parseTrack(trak, len(b)); err != nil {
			return movie{}, err
		}
	}
	if videoTracks != 1 {
		return movie{}, ErrUnsupportedVideo
	}
	return m, nil
}

func isVideoTrack(trak *node) bool {
	hdlr := trak.find("mdia", "hdlr")
	return hdlr != nil && len(hdlr.head) >= 12 && string(hdlr.head[8:12]) == "vide" //nolint:mnd
}

func parseTrack(trak *node, fileLen int) (track, error) {
	stbl := trak.find("mdia", "minf", "stbl")
	mdhd := trak.find("mdia", "mdhd")
	tkhd := trak.find("tkhd")
	if stbl == nil || mdhd == nil || tkhd == nil {
		return track{}, ErrUnsupportedVideo
	}
	if !isEditableTable(stbl) {
		return track{}, ErrUnsupportedVideo
	}
	stsd := stbl.find("stsd")
	if stsd == nil || len(stsd.children) != 1 || stsd.children[0].boxType != "avc1" {
		return track{}, ErrUnsupportedVideo
	}
	t := track{stbl: stbl, avcC: stsd.children[0].find("avcC")}
	if t.avcC == nil {
		return track{}, ErrUnsupportedVideo
	}

	var err error
	if t.timescale, err = readTimescale(mdhd.head); err != nil {
		return track{}, err
	}
	if t.size, t.rotation, err = readTrackHeader(tkhd.head); err != nil {
		return track{}, err
	}
	if elst := trak.find("edts", "elst"); elst != nil {
		if t.mediaTime, t.hasEmptyEdit, err = readMediaTime(elst.head); err != nil {
			return track{}, err
		}
	}
	if err := t.readSamples(fileLen); err != nil {
		return track{}, err
	}
	return t, nil
}

func isEditableTable(stbl *node) bool {
	for _, c := range stbl.children {
		switch c.boxType {
		case "stsd", "stts", "ctts", "cslg", "stss", "stsz", "stsc", stcoType, co64Type, "sdtp", "sbgp", "sgpd":
		default:
			return false
		}
	}
	return true
}

func readTimescale(head []byte) (uint32, error) {
	r := &reader{b: head}
	version := r.u8()
	r.next(3) //nolint:mnd
	if version == 1 {
		r.next(16) //nolint:mnd
	} else {
		r.next(8) //nolint:mnd
	}
	timescale := r.u32()
	if r.err == nil && timescale == 0 {
		return 0, ErrMalformedISO
	}
	return timescale, r.err
}

func readTrackHeader(head []byte) (image.Point, int, error) {
	r := &reader{b: head}
	version := r.u8()
	r.next(3) //nolint:mnd
	if version == 1 {
		r.next(32) //nolint:mnd
	} else {
		r.next(20) //nolint:mnd
	}
	r.next(16) //nolint:mnd
	matrix := make([]int32, 9)
	for i := range matrix {
		matrix[i] = int32(r.u32()) //nolint:gosec // the matrix holds signed fixed-point values
	}
	width, height := int(r.u32()>>16), int(r.u32()>>16)
	if r.err != nil {
		return image.Point{}, 0, r.err
	}
	return image.Pt(width, height), readRotation(matrix), nil
}

func readRotation(m []int32) int {
	switch {
	case m[0] == 0 && m[1] == fixedOne && m[3] == -fixedOne && m[4] == 0:
		return 90 //nolint:mnd
	case m[0] == -fixedOne && m[1] == 0 && m[3] == 0 && m[4] == -fixedOne:
		return 180 //nolint:mnd
	case m[0] == 0 && m[1] == -fixedOne && m[3] == fixedOne && m[4] == 0:
		return 270 //nolint:mnd
	}
	return 0
}

func readMediaTime(head []byte) (mediaTime int64, hasEmptyEdit bool, err error) {
	r := &reader{b: head}
	version := r.u8()
	r.next(3) //nolint:mnd
	entrySize := 12
	if version == 1 {
		entrySize = 20
	}
	for range r.count(entrySize) {
		if version == 1 {
			r.u64()
			mediaTime = int64(r.u64()) //nolint:gosec // a signed field
		} else {
			r.u32()
			mediaTime = int64(int32(r.u32())) //nolint:gosec // a signed field
		}
		r.u32()
		if r.err != nil {
			return 0, false, r.err
		}
		if mediaTime >= 0 {
			return mediaTime, hasEmptyEdit, nil
		}
		hasEmptyEdit = true
	}
	return 0, hasEmptyEdit, r.err
}

func (t *track) readSamples(fileLen int) error {
	sizes, err := readSampleSizes(t.stbl.find("stsz"))
	if err != nil {
		return err
	}
	t.samples = make([]sample, len(sizes))
	for i, size := range sizes {
		t.samples[i].size = size
	}
	if err := t.readTimes(); err != nil {
		return err
	}
	if err := t.readShifts(); err != nil {
		return err
	}
	if err := t.readSyncSamples(); err != nil {
		return err
	}
	return t.readOffsets(fileLen)
}

func readSampleSizes(stsz *node) ([]int, error) {
	if stsz == nil {
		return nil, ErrUnsupportedVideo
	}
	r := &reader{b: stsz.head}
	r.next(4) //nolint:mnd
	fixedSize := r.u32()
	count := int(r.u32())
	if fixedSize == 0 && r.err == nil && count > (len(r.b)-r.off)/4 {
		return nil, ErrMalformedISO
	}
	if count == 0 || count > 1<<20 {
		return nil, ErrUnsupportedVideo
	}
	sizes := make([]int, count)
	for i := range sizes {
		sizes[i] = int(fixedSize)
		if fixedSize == 0 {
			sizes[i] = int(r.u32())
		}
	}
	return sizes, r.err
}

func (t *track) readTimes() error {
	stts := t.stbl.find("stts")
	if stts == nil {
		return ErrUnsupportedVideo
	}
	r := &reader{b: stts.head}
	r.next(4) //nolint:mnd
	i := 0
	dts := int64(0)
	for range r.count(8) { //nolint:mnd
		count, delta := int(r.u32()), r.u32()
		for range count {
			if i >= len(t.samples) {
				return ErrMalformedISO
			}
			t.samples[i].dts, t.samples[i].duration = dts, delta
			dts += int64(delta)
			i++
		}
	}
	if r.err == nil && i != len(t.samples) {
		return ErrMalformedISO
	}
	return r.err
}

func (t *track) readShifts() error {
	ctts := t.stbl.find("ctts")
	if ctts == nil {
		return nil
	}
	t.hasCTTS = true
	r := &reader{b: ctts.head}
	t.isSignedCTTS = r.u8() == 1
	r.next(3) //nolint:mnd
	i := 0
	for range r.count(8) { //nolint:mnd
		count, raw := int(r.u32()), r.u32()
		shift := int64(raw)
		if t.isSignedCTTS {
			shift = int64(int32(raw)) //nolint:gosec // a signed field
		}
		for range count {
			if i >= len(t.samples) {
				return ErrMalformedISO
			}
			t.samples[i].ptsShift = shift
			i++
		}
	}
	if r.err == nil && i != len(t.samples) {
		return ErrMalformedISO
	}
	return r.err
}

func (t *track) readSyncSamples() error {
	stss := t.stbl.find("stss")
	if stss == nil {
		for i := range t.samples {
			t.samples[i].isSync = true
		}
		return nil
	}
	r := &reader{b: stss.head}
	r.next(4)              //nolint:mnd
	for range r.count(4) { //nolint:mnd
		number := int(r.u32())
		if number < 1 || number > len(t.samples) {
			return ErrMalformedISO
		}
		t.samples[number-1].isSync = true
	}
	return r.err
}

func (t *track) readOffsets(fileLen int) error {
	chunks, err := readChunkOffsets(t.stbl)
	if err != nil {
		return err
	}
	stsc := t.stbl.find("stsc")
	if stsc == nil {
		return ErrUnsupportedVideo
	}
	r := &reader{b: stsc.head}
	r.next(4) //nolint:mnd
	type run struct{ firstChunk, perChunk int }
	runs := make([]run, 0, 4) //nolint:mnd
	for range r.count(12) {   //nolint:mnd
		first, perChunk := int(r.u32()), int(r.u32())
		if r.u32() != 1 || first < 1 || (len(runs) > 0 && first <= runs[len(runs)-1].firstChunk) {
			return ErrUnsupportedVideo
		}
		runs = append(runs, run{first, perChunk})
	}
	if r.err != nil {
		return r.err
	}

	i := 0
	for ri, ru := range runs {
		last := len(chunks)
		if ri+1 < len(runs) {
			last = runs[ri+1].firstChunk - 1
		}
		for chunk := ru.firstChunk; chunk <= last && chunk <= len(chunks); chunk++ {
			offset := chunks[chunk-1]
			for range ru.perChunk {
				if i >= len(t.samples) {
					return ErrMalformedISO
				}
				t.samples[i].offset = offset
				offset += t.samples[i].size
				if offset > fileLen {
					return ErrMalformedISO
				}
				i++
			}
		}
	}
	if i != len(t.samples) {
		return ErrMalformedISO
	}
	return nil
}

func readChunkOffsets(stbl *node) ([]int, error) {
	box, entrySize := stbl.find(stcoType), 4
	if box == nil {
		box, entrySize = stbl.find(co64Type), 8
	}
	if box == nil {
		return nil, ErrUnsupportedVideo
	}
	r := &reader{b: box.head}
	r.next(4) //nolint:mnd
	offsets := make([]int, r.count(entrySize))
	for i := range offsets {
		if entrySize == 4 { //nolint:mnd
			offsets[i] = int(r.u32())
		} else {
			offsets[i] = int(r.u64()) //nolint:gosec // checked against the file length later
		}
		if offsets[i] < 0 {
			return nil, ErrMalformedISO
		}
	}
	return offsets, r.err
}

func (t *track) displaySize() image.Point {
	if t.rotation == 90 || t.rotation == 270 {
		return image.Pt(t.size.Y, t.size.X)
	}
	return t.size
}

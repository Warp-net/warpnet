//nolint:all
package video

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildBox(boxType string, payloads ...[]byte) []byte {
	size := boxHeaderSize
	for _, p := range payloads {
		size += len(p)
	}
	out := make([]byte, boxHeaderSize, size)
	binary.BigEndian.PutUint32(out, uint32(size))
	copy(out[4:], boxType)
	for _, p := range payloads {
		out = append(out, p...)
	}
	return out
}

func chunkOffsetsBox(boxType string, offsets ...uint64) []byte {
	payload := make([]byte, chunkOffsetsHeaderSize)
	binary.BigEndian.PutUint32(payload[4:], uint32(len(offsets)))
	for _, o := range offsets {
		if boxType == "stco" {
			payload = binary.BigEndian.AppendUint32(payload, uint32(o))
			continue
		}
		payload = binary.BigEndian.AppendUint64(payload, o)
	}
	return buildBox(boxType, payload)
}

func moovBox(stbl ...[]byte) []byte {
	return buildBox("moov",
		buildBox("mvhd", make([]byte, 12)),
		buildBox("trak", buildBox("mdia", buildBox("minf", buildBox("stbl", stbl...)))),
	)
}

func concatBoxes(boxes ...[]byte) []byte {
	var out []byte
	for _, b := range boxes {
		out = append(out, b...)
	}
	return out
}

const firstChunk = 32

func minimalMP4() []byte {
	return []byte{
		0x00, 0x00, 0x00, 0x18,
		'f', 't', 'y', 'p',
		'i', 's', 'o', 'm',
		0x00, 0x00, 0x02, 0x00,
		'i', 's', 'o', 'm',
		'm', 'p', '4', '1',
	}
}

func moovLast(offsetsBox string, trailing ...[]byte) []byte {
	chunks := buildBox("mdat", []byte("AAAABBBBCCCC"))
	moov := moovBox(chunkOffsetsBox(offsetsBox, firstChunk, firstChunk+4, firstChunk+8))
	return concatBoxes(append([][]byte{minimalMP4(), chunks, moov}, trailing...)...)
}

func topLevelTypes(t *testing.T, b []byte) []string {
	t.Helper()
	boxes, err := walkBoxes(b)
	require.NoError(t, err)
	types := make([]string, 0, len(boxes))
	for _, box := range boxes {
		types = append(types, box.boxType)
	}
	return types
}

func chunkOffsets(t *testing.T, b []byte) []uint64 {
	t.Helper()
	boxes, err := walkBoxes(b)
	require.NoError(t, err)

	var offsets []uint64
	for _, box := range boxes {
		switch box.boxType {
		case "moov", "trak", "mdia", "minf", "stbl":
			offsets = append(offsets, chunkOffsets(t, box.payload)...)
		case "stco":
			for i := 0; i < int(binary.BigEndian.Uint32(box.payload[4:8])); i++ {
				offsets = append(offsets, uint64(binary.BigEndian.Uint32(box.payload[8+i*4:])))
			}
		case "co64":
			for i := 0; i < int(binary.BigEndian.Uint32(box.payload[4:8])); i++ {
				offsets = append(offsets, binary.BigEndian.Uint64(box.payload[8+i*8:]))
			}
		}
	}
	return offsets
}

func readChunks(t *testing.T, file []byte) []string {
	t.Helper()
	var chunks []string
	for _, offset := range chunkOffsets(t, file) {
		chunks = append(chunks, string(file[offset:offset+4]))
	}
	return chunks
}

func TestMoveMoovToFront_PutsMoovBeforeTheChunks(t *testing.T) {
	for _, offsetsBox := range []string{"stco", "co64"} {
		t.Run(offsetsBox, func(t *testing.T) {
			file := moovLast(offsetsBox)
			require.Equal(t, []string{"AAAA", "BBBB", "CCCC"}, readChunks(t, file))

			moved, err := MoveMoovToFront(file)
			require.NoError(t, err)

			assert.Equal(t, []string{"ftyp", "moov", "mdat"}, topLevelTypes(t, moved))
			assert.Len(t, moved, len(file))
			assert.Equal(t, []string{"AAAA", "BBBB", "CCCC"}, readChunks(t, moved))
		})
	}
}

func TestMoveMoovToFront_LeavesBoxesAfterMoovInPlace(t *testing.T) {
	file := moovLast("stco", buildBox("free", []byte("tail")))

	moved, err := MoveMoovToFront(file)
	require.NoError(t, err)

	assert.Equal(t, []string{"ftyp", "moov", "mdat", "free"}, topLevelTypes(t, moved))
	assert.Equal(t, file[len(file)-12:], moved[len(moved)-12:])
	assert.Equal(t, []string{"AAAA", "BBBB", "CCCC"}, readChunks(t, moved))
}

func TestMoveMoovToFront_KeepsFilesItCannotOrNeedNotMove(t *testing.T) {
	stco := chunkOffsetsBox("stco", firstChunk)
	chunks := buildBox("mdat", []byte("AAAA"))

	cases := map[string][]byte{
		"moov already first": concatBoxes(minimalMP4(), moovBox(chunkOffsetsBox("stco", 999)), chunks),
		"fragmented":         concatBoxes(minimalMP4(), chunks, buildBox("moof"), moovBox(stco)),
		"no moov":            concatBoxes(minimalMP4(), chunks),
		"aux info offsets":   concatBoxes(minimalMP4(), chunks, moovBox(stco, buildBox("saio", make([]byte, 8)))),
		"broken chunk table": concatBoxes(minimalMP4(), chunks, moovBox(buildBox("stco", []byte{0, 0, 0, 0, 0, 0, 0, 9}))),
	}
	for name, file := range cases {
		t.Run(name, func(t *testing.T) {
			moved, err := MoveMoovToFront(file)
			require.NoError(t, err)
			assert.Equal(t, file, moved)
		})
	}
}

func TestMoveMoovToFront_RefusesMalformedFiles(t *testing.T) {
	_, err := MoveMoovToFront([]byte{0x00, 0x00, 0xFF, 0xFF, 'f', 't', 'y', 'p'})
	assert.ErrorIs(t, err, ErrMalformedISO)
}

func isoBoxOf(size uint32, boxType string, payload []byte) []byte {
	out := make([]byte, 0, boxHeaderSize+len(payload))
	out = binary.BigEndian.AppendUint32(out, size)
	out = append(out, boxType...)
	return append(out, payload...)
}

func TestCloseOpenEndedBoxEdgeCases(t *testing.T) {
	t.Run("rejects malformed input", func(t *testing.T) {
		_, err := CloseOpenEndedBox(isoBoxOf(1024, "ftyp", []byte("short")))
		require.ErrorIs(t, err, ErrMalformedISO)
	})

	t.Run("passes an empty file through", func(t *testing.T) {
		out, err := CloseOpenEndedBox(nil)
		require.NoError(t, err)
		require.Nil(t, out)
	})

	t.Run("leaves an already-sized last box alone", func(t *testing.T) {
		in := minimalMP4()
		out, err := CloseOpenEndedBox(in)
		require.NoError(t, err)
		require.Equal(t, in, out)
	})

	t.Run("writes the real size into an open-ended last box", func(t *testing.T) {
		in := append(minimalMP4(), isoBoxOf(0, "mdat", []byte("streamed payload"))...)

		out, err := CloseOpenEndedBox(in)
		require.NoError(t, err)
		require.Len(t, out, len(in))

		last := len(minimalMP4())
		require.Equal(t, uint32(len(in)-last), binary.BigEndian.Uint32(out[last:last+4]))
	})
}

func durationBox(boxType string, version byte, timescale uint32, duration uint64) []byte {
	payload := []byte{version, 0, 0, 0}
	if version == 1 {
		payload = append(payload, make([]byte, 16)...)
		payload = binary.BigEndian.AppendUint32(payload, timescale)
		payload = binary.BigEndian.AppendUint64(payload, duration)
	} else {
		payload = append(payload, make([]byte, 8)...)
		payload = binary.BigEndian.AppendUint32(payload, timescale)
		payload = binary.BigEndian.AppendUint32(payload, uint32(duration))
	}
	return buildBox(boxType, payload)
}

func movieWithLengths(movie, media uint64) []byte {
	return concatBoxes(minimalMP4(), buildBox("moov",
		durationBox("mvhd", 0, 1000, movie),
		buildBox("trak", buildBox("mdia", durationBox("mdhd", 1, 30000, media*30))),
	))
}

func TestDuration_TakesTheLongestOfTheMovieAndItsTracks(t *testing.T) {
	cases := map[string]struct {
		file []byte
		want time.Duration
	}{
		"59.9 s":                    {movieWithLengths(59900, 59900), 59900 * time.Millisecond},
		"60.4 s":                    {movieWithLengths(60400, 60400), 60400 * time.Millisecond},
		"61 s":                      {movieWithLengths(61000, 61000), 61 * time.Second},
		"an edit list hides length": {movieWithLengths(30000, 61000), 61 * time.Second},
		"no moov":                   {minimalMP4(), 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Duration(c.file)
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestDuration_RefusesMalformedHeaders(t *testing.T) {
	file := concatBoxes(minimalMP4(), buildBox("moov", buildBox("mvhd", []byte{0, 0, 0, 0})))
	_, err := Duration(file)
	assert.ErrorIs(t, err, ErrMalformedISO)
}

//nolint:all
package media_meta

import (
	"encoding/binary"
	"testing"

	"github.com/Warp-net/warpnet/security"
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

func TestMoveMoovToFront_SignedFileStillVerifies(t *testing.T) {
	moved, err := MoveMoovToFront(moovLast("stco"))
	require.NoError(t, err)

	meta, err := metadata("alice").Sign(security.ConvertToSHA256(moved))
	require.NoError(t, err)
	signed, err := EmbedInVideo(moved, meta)
	require.NoError(t, err)

	assert.NoError(t, VerifyVideo(signed, signerID.String(), "alice"))
}

//nolint:all
package media_meta

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

// isoBoxOf builds one ISO-BMFF box with the given 32-bit size field.
func isoBoxOf(size uint32, boxType string, payload []byte) []byte {
	out := make([]byte, 0, boxHeaderSize+len(payload))
	out = binary.BigEndian.AppendUint32(out, size)
	out = append(out, boxType...)
	return append(out, payload...)
}

func TestBoxBoundsRejectsMalformedBoxes(t *testing.T) {
	t.Run("a size smaller than the header", func(t *testing.T) {
		_, _, err := boxBounds(isoBoxOf(4, "ftyp", nil), 0)
		require.ErrorIs(t, err, ErrMalformedISO)
	})

	t.Run("a size past the end of the file", func(t *testing.T) {
		_, _, err := boxBounds(isoBoxOf(1024, "ftyp", []byte("short")), 0)
		require.ErrorIs(t, err, ErrMalformedISO)
	})

	t.Run("a truncated 64-bit largesize header", func(t *testing.T) {
		_, _, err := boxBounds(isoBoxOf(1, "ftyp", nil), 0)
		require.ErrorIs(t, err, ErrMalformedISO)
	})

	t.Run("a 64-bit largesize past the end of the file", func(t *testing.T) {
		b := isoBoxOf(1, "ftyp", binary.BigEndian.AppendUint64(nil, 1<<40))
		_, _, err := boxBounds(b, 0)
		require.ErrorIs(t, err, ErrMalformedISO)
	})

	t.Run("a valid 64-bit largesize", func(t *testing.T) {
		payload := binary.BigEndian.AppendUint64(nil, uint64(boxLargeHeadLen))
		b := isoBoxOf(1, "ftyp", payload)

		size, headerSize, err := boxBounds(b, 0)
		require.NoError(t, err)
		require.Equal(t, boxLargeHeadLen, size)
		require.Equal(t, boxLargeHeadLen, headerSize)
	})

	t.Run("an open-ended box runs to the end", func(t *testing.T) {
		b := isoBoxOf(0, "mdat", []byte("payload"))
		size, headerSize, err := boxBounds(b, 0)
		require.NoError(t, err)
		require.Equal(t, len(b), size)
		require.Equal(t, boxHeaderSize, headerSize)
	})
}

func TestSplitVideoRejectsMalformedInput(t *testing.T) {
	_, _, err := SplitVideo(isoBoxOf(1024, "ftyp", []byte("short")))
	require.ErrorIs(t, err, ErrMalformedISO)
}

func TestVerifyVideoRejectsMalformedInput(t *testing.T) {
	require.ErrorIs(t,
		VerifyVideo(isoBoxOf(1024, "ftyp", []byte("short")), "node", "owner"),
		ErrMalformedISO,
	)
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

func TestStripJPEGExifRejectsMalformedInput(t *testing.T) {
	t.Run("a segment size below the minimum", func(t *testing.T) {
		b := []byte{jpegMarker, 0xD8, jpegMarker, 0xE1, 0x00, 0x01}
		_, err := stripJPEGExif(b)
		require.ErrorIs(t, err, ErrMalformedJPEG)
	})

	t.Run("a segment running past the end", func(t *testing.T) {
		b := []byte{jpegMarker, 0xD8, jpegMarker, 0xE1, 0xFF, 0xFF, 0x00}
		_, err := stripJPEGExif(b)
		require.ErrorIs(t, err, ErrMalformedJPEG)
	})

	t.Run("standalone markers are copied verbatim", func(t *testing.T) {
		b := []byte{jpegMarker, 0xD8, jpegMarker, jpegRST0, jpegMarker, jpegTEM, 0x00}
		out, err := stripJPEGExif(b)
		require.NoError(t, err)
		require.NotEmpty(t, out)
	})

	t.Run("a non-marker byte ends the scan", func(t *testing.T) {
		b := []byte{jpegMarker, 0xD8, 0x00, 0x11, 0x22, 0x33}
		out, err := stripJPEGExif(b)
		require.NoError(t, err)
		require.NotEmpty(t, out)
	})
}

func TestVerifyImageRejectsMalformedInput(t *testing.T) {
	// extraction runs before the strip, so a file with no metadata is reported
	// as such even when its segments are also malformed
	err := VerifyImage([]byte{jpegMarker, 0xD8, jpegMarker, 0xE1, 0x00, 0x01}, "node", "owner")
	require.ErrorIs(t, err, ErrNoMetadata)
}

func TestExtractFromJPEGWithoutMetadata(t *testing.T) {
	_, err := extractFromJPEG(testJPEG(t, 0x20))
	require.ErrorIs(t, err, ErrNoMetadata)
}

//nolint:all
package media_meta

import (
	"encoding/binary"
	"testing"

	"github.com/Warp-net/warpnet/core/video"
	"github.com/stretchr/testify/require"
)

// isoBoxOf builds one ISO-BMFF box with the given 32-bit size field.
func isoBoxOf(size uint32, boxType string, payload []byte) []byte {
	out := make([]byte, 0, 8+len(payload))
	out = binary.BigEndian.AppendUint32(out, size)
	out = append(out, boxType...)
	return append(out, payload...)
}

func TestSplitVideoRejectsMalformedInput(t *testing.T) {
	_, _, err := SplitVideo(isoBoxOf(1024, "ftyp", []byte("short")))
	require.ErrorIs(t, err, video.ErrMalformedISO)
}

func TestVerifyVideoRejectsMalformedInput(t *testing.T) {
	require.ErrorIs(t,
		VerifyVideo(isoBoxOf(1024, "ftyp", []byte("short")), "node", "owner"),
		video.ErrMalformedISO,
	)
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

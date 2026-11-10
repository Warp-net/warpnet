//go:build !android && !ios && ((linux && amd64) || (linux && arm64) || (darwin && amd64) || (darwin && arm64) || (windows && amd64) || (windows && arm64))

package ffmpeg

import (
	"bytes"
	"compress/gzip"
	"io"
)

// GetFFmpeg returns a fresh copy of the FFmpeg executable for the current platform.
// The caller must write it to an executable file before running it.
func GetFFmpeg() ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	return io.ReadAll(reader)
}

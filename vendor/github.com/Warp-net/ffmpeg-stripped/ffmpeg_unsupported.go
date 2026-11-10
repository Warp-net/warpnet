//go:build android || ios || (!(linux && amd64) && !(linux && arm64) && !(darwin && amd64) && !(darwin && arm64) && !(windows && amd64) && !(windows && arm64))

package ffmpeg

// GetFFmpeg returns ErrUnsupported when no binary is available for this platform.
func GetFFmpeg() ([]byte, error) { return nil, ErrUnsupported }

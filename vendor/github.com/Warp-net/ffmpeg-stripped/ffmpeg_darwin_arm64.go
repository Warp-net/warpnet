//go:build !ios

package ffmpeg

import _ "embed"

//go:embed bin/ffmpeg-darwin-arm64.gz
var compressed []byte

//go:build !android

package ffmpeg

import _ "embed"

//go:embed bin/ffmpeg-linux-arm64.gz
var compressed []byte

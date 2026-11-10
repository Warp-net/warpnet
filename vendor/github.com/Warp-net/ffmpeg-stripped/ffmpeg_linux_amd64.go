//go:build !android

package ffmpeg

import _ "embed"

//go:embed bin/ffmpeg-linux-amd64.gz
var compressed []byte

//go:build !ios

package ffmpeg

import _ "embed"

//go:embed bin/ffmpeg-darwin-amd64.gz
var compressed []byte

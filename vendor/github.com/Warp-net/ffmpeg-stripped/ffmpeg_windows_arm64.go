package ffmpeg

import _ "embed"

//go:embed bin/ffmpeg-windows-arm64.exe.gz
var compressed []byte

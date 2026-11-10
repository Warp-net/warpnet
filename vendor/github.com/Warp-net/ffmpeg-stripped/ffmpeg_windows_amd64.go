package ffmpeg

import _ "embed"

//go:embed bin/ffmpeg-windows-amd64.exe.gz
var compressed []byte

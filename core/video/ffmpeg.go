/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package video

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	ffmpeg "github.com/Warp-net/ffmpeg-stripped"
	"github.com/Warp-net/warpnet/core/warpnet"
)

const (
	maxStderrSize = 4 << 10

	ErrChangedBinary warpnet.WarpError = "video: the unpacked ffmpeg differs from the embedded one"
)

var ErrUnsupportedPlatform = ffmpeg.ErrUnsupported

type ProcessLimiter interface {
	Acquire(ctx context.Context) error
	Release()
}

type FFmpeg struct {
	dir     string
	limiter ProcessLimiter

	mu   sync.Mutex
	path string
	sum  [sha256.Size]byte
}

func NewFFmpeg(dir string, limiter ProcessLimiter) *FFmpeg {
	return &FFmpeg{dir: dir, limiter: limiter}
}

func (f *FFmpeg) run(ctx context.Context, args ...string) error {
	if err := f.limiter.Acquire(ctx); err != nil {
		return err
	}
	defer f.limiter.Release()

	path, err := f.unpack()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, path, append([]string{"-nostdin", "-hide_banner", "-loglevel", "error"}, args...)...) //nolint:gosec // our own unpacked binary
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{w: &stderr, left: maxStderrSize}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("video: ffmpeg: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (f *FFmpeg) unpack() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.path != "" {
		err := matchesBinary(f.path, f.sum)
		if err == nil {
			return f.path, nil
		}
		if !os.IsNotExist(err) && !errors.Is(err, ErrChangedBinary) {
			return "", err
		}
	}

	binary, err := ffmpeg.GetFFmpeg()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(f.dir, 0o700); err != nil {
		return "", err
	}
	sum := sha256.Sum256(binary)
	name := "ffmpeg-" + hex.EncodeToString(sum[:6])
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(f.dir, name)
	if err := matchesBinary(path, sum); err == nil {
		f.path, f.sum = path, sum
		return path, nil
	}

	tmp, err := os.CreateTemp(f.dir, name+".*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(binary); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o700); err != nil { //nolint:gosec // ffmpeg has to stay executable
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	f.path, f.sum = path, sum
	return path, nil
}

func (f *FFmpeg) makeWorkDir() (string, error) {
	if err := os.MkdirAll(f.dir, 0o700); err != nil {
		return "", err
	}
	return os.MkdirTemp(f.dir, "work-")
}

func matchesBinary(path string, want [sha256.Size]byte) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrChangedBinary
	}
	file, err := os.Open(path) //nolint:gosec // our own data directory
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return err
	}
	if [sha256.Size]byte(digest.Sum(nil)) != want {
		return ErrChangedBinary
	}
	return nil
}

type limitedWriter struct {
	w    io.Writer
	left int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.left > 0 {
		n := min(len(p), l.left)
		_, _ = l.w.Write(p[:n])
		l.left -= n
	}
	return len(p), nil
}

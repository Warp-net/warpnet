//go:build !windows

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

package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxBinarySize = 512 << 20 // decompression bomb guard
	binaryMode    = 0o755
)

// checksumOf picks the hash of assetName out of a `shasum -a 256` listing.
func checksumOf(listing []byte, assetName string) (string, error) {
	for line := range strings.SplitSeq(string(listing), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := filepath.Base(strings.TrimPrefix(fields[1], "*")) // '*' marks binary mode
		if name != assetName {
			continue
		}
		return strings.ToLower(fields[0]), nil
	}
	return "", fmt.Errorf("selfupdate: %w: %s", ErrChecksumNotFound, assetName)
}

// extractBinary writes the binaryName entry of a .tar.gz archive to dstPath.
func extractBinary(archivePath, binaryName, dstPath string) error {
	archive, err := os.Open(archivePath) //nolint:gosec // archive was just downloaded to this path
	if err != nil {
		return fmt.Errorf("selfupdate: opening %s: %w", archivePath, err)
	}
	defer func() { _ = archive.Close() }()

	gz, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("selfupdate: reading %s: %w", archivePath, err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("selfupdate: %w: %s", ErrBinaryNotFound, binaryName)
		}
		if err != nil {
			return fmt.Errorf("selfupdate: reading %s: %w", archivePath, err)
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != binaryName {
			continue
		}
		return writeBinary(tr, dstPath)
	}
}

func writeBinary(src io.Reader, dstPath string) (err error) {
	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, binaryMode) //nolint:gosec // executable bit is required
	if err != nil {
		return fmt.Errorf("selfupdate: creating %s: %w", dstPath, err)
	}
	defer func() {
		if closeErr := dst.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("selfupdate: closing %s: %w", dstPath, closeErr)
		}
	}()

	written, err := io.Copy(dst, io.LimitReader(src, maxBinarySize))
	if err != nil {
		return fmt.Errorf("selfupdate: writing %s: %w", dstPath, err)
	}
	if written == maxBinarySize {
		return fmt.Errorf("selfupdate: %w: %s", ErrTooLarge, dstPath)
	}
	// O_CREATE keeps the mode of an already existing file.
	if err := os.Chmod(dstPath, binaryMode); err != nil { //nolint:gosec // executable bit is required
		return fmt.Errorf("selfupdate: chmod %s: %w", dstPath, err)
	}
	return nil
}

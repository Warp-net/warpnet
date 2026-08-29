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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChecksumOf(t *testing.T) {
	listing := []byte(
		"aaaa  warpnet_linux_amd64.tar.gz\n" +
			"BBBB  relay_linux_amd64.tar.gz\n",
	)

	got, err := checksumOf(listing, "relay_linux_amd64.tar.gz")
	require.NoError(t, err)
	assert.Equal(t, "bbbb", got)

	_, err = checksumOf(listing, "relay_darwin.tar.gz")
	require.ErrorIs(t, err, ErrChecksumNotFound)
}

func TestExtractBinary(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, testAsset)
	dstPath := filepath.Join(dir, testBinary)
	require.NoError(t, os.WriteFile(archivePath, tarGz(t, testBinary, []byte("payload")), 0o644))

	require.NoError(t, extractBinary(archivePath, testBinary, dstPath))

	assert.Equal(t, "payload", read(t, dstPath))
	info, err := os.Stat(dstPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(binaryMode), info.Mode().Perm(), "installed binary must be executable")
}

func TestExtractBinaryWithoutMatchingEntry(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, testAsset)
	require.NoError(t, os.WriteFile(archivePath, tarGz(t, "README.md", []byte("payload")), 0o644))

	err := extractBinary(archivePath, testBinary, filepath.Join(dir, testBinary))
	require.ErrorIs(t, err, ErrBinaryNotFound)
}

func TestExtractBinaryFromCorruptedArchive(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, testAsset)
	require.NoError(t, os.WriteFile(archivePath, []byte("not a gzip stream"), 0o644))

	err := extractBinary(archivePath, testBinary, filepath.Join(dir, testBinary))
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(dir, testBinary))
}

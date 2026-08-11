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

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutableInstallRollback(t *testing.T) {
	dir := t.TempDir()
	e := &executable{path: filepath.Join(dir, testBinary)}
	staged := e.StagePath(testBinary + newSuffix)

	require.NoError(t, os.WriteFile(e.path, []byte("old"), binaryMode))
	require.NoError(t, os.WriteFile(staged, []byte("new"), binaryMode))

	rollback, err := e.Install(staged)
	require.NoError(t, err)
	assert.Equal(t, "new", read(t, e.path))
	assert.Equal(t, "old", read(t, e.path+oldSuffix))

	rollback()
	assert.Equal(t, "old", read(t, e.path))
}

func TestExecutableStagePathSharesFilesystem(t *testing.T) {
	e := &executable{path: "/warpnet/warpnet"}

	assert.Equal(t, "/warpnet/relay.tar.gz", e.StagePath("relay.tar.gz"))
	assert.Equal(t, filepath.Dir(e.path), filepath.Dir(e.StagePath("anything")))
}

func TestFailureMarker(t *testing.T) {
	m := newFailureMarker(filepath.Join(t.TempDir(), testBinary))
	failed := semver.MustParse("0.7.548")

	assert.False(t, m.Has(failed), "no marker yet")

	m.Set(failed)
	assert.True(t, m.Has(failed))
	assert.False(t, m.Has(semver.MustParse("0.7.549")), "only the recorded version is skipped")

	m.Clear()
	assert.False(t, m.Has(failed))
	assert.NoFileExists(t, m.path)
	m.Clear() // clearing twice must stay silent
}

func TestFailureMarkerDropsMalformedContent(t *testing.T) {
	m := newFailureMarker(filepath.Join(t.TempDir(), testBinary))
	require.NoError(t, os.WriteFile(m.path, []byte("not a version"), markerMode))

	assert.False(t, m.Has(semver.MustParse("0.7.548")))
	assert.NoFileExists(t, m.path)
}

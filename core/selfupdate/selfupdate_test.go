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
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAsset    = "relay_test.tar.gz"
	testChecksum = "relay_test_checksums.txt"
	testBinary   = "relay"
	testVersion  = "0.7.547"
)

func tarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "./", Typeflag: tar.TypeDir, Mode: 0o755,
	}))
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "./" + name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(content)),
	}))
	_, err := tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	return buf.Bytes()
}

// releaseServer serves a GitHub-shaped release with the given tag and archive.
func releaseServer(t *testing.T, tag string, archive, sums []byte) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/latest", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"tag_name":%q,"assets":[
			{"name":%q,"browser_download_url":%q},
			{"name":%q,"browser_download_url":%q}
		]}`, tag, testAsset, srv.URL+"/"+testAsset, testChecksum, srv.URL+"/"+testChecksum)
	})
	mux.HandleFunc("/"+testAsset, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/"+testChecksum, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(sums)
	})

	return srv
}

// updaterFixture returns an updater whose executable is a temporary file, along
// with that path and a pointer to the restart flag.
func updaterFixture(t *testing.T, latest string, archive, sums []byte) (*SelfUpdater, string, *bool) {
	t.Helper()

	execPath := filepath.Join(t.TempDir(), "relay")
	require.NoError(t, os.WriteFile(execPath, []byte("current binary"), 0o755))

	srv := releaseServer(t, latest, archive, sums)

	restarted := false
	u := NewSelfUpdater(
		context.Background(),
		semver.MustParse(testVersion),
		Artifact{AssetName: testAsset, ChecksumName: testChecksum, BinaryName: testBinary},
	)
	u.apiURL = srv.URL + "/latest"
	u.execPath = execPath
	u.restartF = func(_ string, shutdownF func()) error {
		restarted = true
		if shutdownF != nil {
			shutdownF()
		}
		return nil
	}
	u.fatalF = func(format string, args ...any) {
		t.Logf("fatal: "+format, args...)
	}

	return u, execPath, &restarted
}

var errRestartFailed = errors.New("exec: no such file or directory")

// failingRestart makes the process replacement fail, as an unusable binary does.
func failingRestart(u *SelfUpdater) {
	u.restartF = func(_ string, shutdownF func()) error {
		if shutdownF != nil {
			shutdownF()
		}
		return errRestartFailed
	}
}

func sumsFor(archive []byte) []byte {
	sum := sha256.Sum256(archive)
	return fmt.Appendf(nil, "%s  %s\n", hex.EncodeToString(sum[:]), testAsset)
}

func TestSelfUpdaterInstallsNewerRelease(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	u, execPath, restarted := updaterFixture(t, "v0.7.548", archive, sumsFor(archive))

	stopped := false
	require.NoError(t, u.checkAndUpdate(func() { stopped = true }))

	installed, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, "new binary", string(installed))
	assert.True(t, *restarted)
	assert.True(t, stopped, "node must be stopped before the process is replaced")

	previous, err := os.ReadFile(execPath + oldSuffix)
	require.NoError(t, err)
	assert.Equal(t, "current binary", string(previous), "previous binary must be kept for rollback")

	// archive and extracted binary must not be left behind
	assert.NoFileExists(t, filepath.Join(filepath.Dir(execPath), testAsset))
	assert.NoFileExists(t, execPath+newSuffix)
}

func TestSelfUpdaterSkipsSameOrOlderRelease(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))

	for _, latest := range []string{"v0.7.547", "v0.7.546"} {
		t.Run(latest, func(t *testing.T) {
			u, execPath, restarted := updaterFixture(t, latest, archive, sumsFor(archive))

			require.NoError(t, u.checkAndUpdate(nil))

			kept, err := os.ReadFile(execPath)
			require.NoError(t, err)
			assert.Equal(t, "current binary", string(kept))
			assert.False(t, *restarted)
		})
	}
}

func TestSelfUpdaterRejectsWrongChecksum(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	sums := sumsFor([]byte("something else"))
	u, execPath, restarted := updaterFixture(t, "v0.7.548", archive, sums)

	err := u.checkAndUpdate(nil)
	require.ErrorIs(t, err, ErrChecksumMismatch)

	kept, readErr := os.ReadFile(execPath)
	require.NoError(t, readErr)
	assert.Equal(t, "current binary", string(kept), "binary must stay untouched")
	assert.False(t, *restarted)
	assert.NoFileExists(t, filepath.Join(filepath.Dir(execPath), testAsset))
}

func TestSelfUpdaterRejectsArchiveWithoutBinary(t *testing.T) {
	archive := tarGz(t, "README.md", []byte("no binary here"))
	u, _, restarted := updaterFixture(t, "v0.7.548", archive, sumsFor(archive))

	require.ErrorIs(t, u.checkAndUpdate(nil), ErrBinaryNotFound)
	assert.False(t, *restarted)
}

func TestSelfUpdaterRejectsMissingAsset(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	u, _, restarted := updaterFixture(t, "v0.7.548", archive, sumsFor(archive))
	u.artifact.AssetName = "relay_other_platform.tar.gz"

	require.ErrorIs(t, u.checkAndUpdate(nil), ErrAssetNotFound)
	assert.False(t, *restarted)
}

func TestSelfUpdaterMarksVersionThatFailsToStart(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("unusable binary"))
	u, execPath, _ := updaterFixture(t, "v0.7.548", archive, sumsFor(archive))
	failingRestart(u)

	require.NoError(t, u.checkAndUpdate(nil))

	kept, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, "current binary", string(kept), "previous binary must be restored")

	marker, err := os.ReadFile(execPath + failedSuffix)
	require.NoError(t, err)
	assert.Equal(t, "0.7.548", string(marker))
}

func TestSelfUpdaterSkipsVersionMarkedAsFailed(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	u, execPath, restarted := updaterFixture(t, "v0.7.548", archive, sumsFor(archive))
	require.NoError(t, os.WriteFile(execPath+failedSuffix, []byte("0.7.548"), markerMode))

	require.NoError(t, u.checkAndUpdate(nil))

	kept, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, "current binary", string(kept))
	assert.False(t, *restarted)
}

func TestSelfUpdaterRetriesAfterMarkedVersionSuperseded(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	u, execPath, restarted := updaterFixture(t, "v0.7.549", archive, sumsFor(archive))
	require.NoError(t, os.WriteFile(execPath+failedSuffix, []byte("0.7.548"), markerMode))

	require.NoError(t, u.checkAndUpdate(nil))

	installed, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, "new binary", string(installed))
	assert.True(t, *restarted)
	assert.NoFileExists(t, execPath+failedSuffix, "stale marker must be dropped")
}

func TestSelfUpdaterIgnoresMalformedMarker(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	u, execPath, restarted := updaterFixture(t, "v0.7.548", archive, sumsFor(archive))
	require.NoError(t, os.WriteFile(execPath+failedSuffix, []byte("not a version"), markerMode))

	require.NoError(t, u.checkAndUpdate(nil))

	installed, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, "new binary", string(installed))
	assert.True(t, *restarted)
	assert.NoFileExists(t, execPath+failedSuffix)
}

func TestChecksum(t *testing.T) {
	sums := []byte(
		"aaaa  warpnet_linux_amd64.tar.gz\n" +
			"BBBB  relay_linux_amd64.tar.gz\n",
	)

	got, err := checksum(sums, "relay_linux_amd64.tar.gz")
	require.NoError(t, err)
	assert.Equal(t, "bbbb", got)

	_, err = checksum(sums, "relay_darwin.tar.gz")
	require.ErrorIs(t, err, ErrChecksumNotFound)
}

func TestSwapBinaryRestoresPrevious(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "relay")
	newPath := execPath + newSuffix

	require.NoError(t, os.WriteFile(execPath, []byte("old"), 0o755))
	require.NoError(t, os.WriteFile(newPath, []byte("new"), 0o755))

	restore, err := swapBinary(execPath, newPath)
	require.NoError(t, err)

	installed, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, "new", string(installed))

	restore()

	rolledBack, err := os.ReadFile(execPath)
	require.NoError(t, err)
	assert.Equal(t, "old", string(rolledBack))
}

func TestArtifactSupport(t *testing.T) {
	assert.False(t, Artifact{}.isSupported())
	assert.False(t, Artifact{AssetName: testAsset}.isSupported())
	assert.True(t, Artifact{
		AssetName: testAsset, ChecksumName: testChecksum, BinaryName: testBinary,
	}.isSupported())
}

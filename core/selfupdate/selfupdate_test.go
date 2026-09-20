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
	"runtime"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAsset    = "relay_test.tar.gz"
	testChecksum = "relay_test_checksums.txt"
	testBinary   = "relay"
	testVersion  = "0.7.547"

	approverWait = 2 * time.Second
)

var errRestartFailed = errors.New("exec: no such file or directory")

// fakeBinary installs like the real executable, but hands the process over to
// nobody: replacing the test process image would end the test run.
type fakeBinary struct {
	*executable

	restartErr error
	restarted  bool
}

func (f *fakeBinary) Restart(shutdownF func()) error {
	f.restarted = true
	if shutdownF != nil {
		shutdownF()
	}
	return f.restartErr
}

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

func sumsFor(archive []byte) []byte {
	sum := sha256.Sum256(archive)
	return fmt.Appendf(nil, "%s  %s\n", hex.EncodeToString(sum[:]), testAsset)
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

// updaterFixture returns an updater whose binary is a temporary file served by a
// local release server.
func updaterFixture(t *testing.T, latest string, archive, sums []byte) (*SelfUpdater, *fakeBinary) {
	t.Helper()

	path := filepath.Join(t.TempDir(), testBinary)
	require.NoError(t, os.WriteFile(path, []byte("current binary"), 0o755))

	gh := newGitHubReleases(context.Background(), semver.MustParse(testVersion))
	gh.apiURL = releaseServer(t, latest, archive, sums).URL + "/latest"

	binary := &fakeBinary{executable: &executable{path: path}}
	u := NewSelfUpdater(
		context.Background(),
		semver.MustParse(testVersion),
		Artifact{AssetName: testAsset, ChecksumName: testChecksum, BinaryName: testBinary},
		false,
	)
	u.releases, u.assets = gh, gh
	u.binary = binary
	u.failures = newFailureMarker(path)

	return u, binary
}

func TestSelfUpdaterInstallsNewerRelease(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	u, binary := updaterFixture(t, "v0.7.548", archive, sumsFor(archive))

	stopped := false
	require.NoError(t, u.checkAndUpdate(func() { stopped = true }))

	assert.Equal(t, "new binary", read(t, binary.path))
	assert.True(t, binary.restarted)
	assert.True(t, stopped, "node must be stopped before the process is replaced")
	assert.Equal(t, "current binary", read(t, binary.path+oldSuffix), "previous binary must be kept for rollback")

	// staged files must not be left behind
	assert.NoFileExists(t, binary.StagePath(testAsset))
	assert.NoFileExists(t, binary.StagePath(testBinary+newSuffix))
}

func TestSelfUpdaterSkipsSameOrOlderRelease(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))

	for _, latest := range []string{"v0.7.547", "v0.7.546"} {
		t.Run(latest, func(t *testing.T) {
			u, binary := updaterFixture(t, latest, archive, sumsFor(archive))

			require.NoError(t, u.checkAndUpdate(nil))

			assert.Equal(t, "current binary", read(t, binary.path))
			assert.False(t, binary.restarted)
		})
	}
}

func TestSelfUpdaterRejectsWrongChecksum(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	u, binary := updaterFixture(t, "v0.7.548", archive, sumsFor([]byte("something else")))

	require.ErrorIs(t, u.checkAndUpdate(nil), ErrChecksumMismatch)

	assert.Equal(t, "current binary", read(t, binary.path), "binary must stay untouched")
	assert.False(t, binary.restarted)
	assert.NoFileExists(t, binary.StagePath(testAsset))
}

func TestSelfUpdaterRejectsArchiveWithoutBinary(t *testing.T) {
	archive := tarGz(t, "README.md", []byte("no binary here"))
	u, binary := updaterFixture(t, "v0.7.548", archive, sumsFor(archive))

	require.ErrorIs(t, u.checkAndUpdate(nil), ErrBinaryNotFound)
	assert.False(t, binary.restarted)
}

func TestSelfUpdaterRejectsMissingAsset(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	u, binary := updaterFixture(t, "v0.7.548", archive, sumsFor(archive))
	u.artifact.AssetName = "relay_other_platform.tar.gz"

	require.ErrorIs(t, u.checkAndUpdate(nil), ErrAssetNotFound)
	assert.False(t, binary.restarted)
}

func TestSelfUpdaterMarksVersionThatFailsToStart(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("unusable binary"))
	u, binary := updaterFixture(t, "v0.7.548", archive, sumsFor(archive))
	binary.restartErr = errRestartFailed

	require.ErrorIs(t, u.checkAndUpdate(nil), ErrRestartFailed)

	assert.Equal(t, "current binary", read(t, binary.path), "previous binary must be restored")
	assert.Equal(t, "0.7.548", read(t, binary.path+failedSuffix))
}

func TestSelfUpdaterSkipsVersionMarkedAsFailed(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	u, binary := updaterFixture(t, "v0.7.548", archive, sumsFor(archive))
	require.NoError(t, os.WriteFile(binary.path+failedSuffix, []byte("0.7.548"), markerMode))

	require.NoError(t, u.checkAndUpdate(nil))

	assert.Equal(t, "current binary", read(t, binary.path))
	assert.False(t, binary.restarted)
}

func TestSelfUpdaterRetriesAfterMarkedVersionSuperseded(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	u, binary := updaterFixture(t, "v0.7.549", archive, sumsFor(archive))
	require.NoError(t, os.WriteFile(binary.path+failedSuffix, []byte("0.7.548"), markerMode))

	require.NoError(t, u.checkAndUpdate(nil))

	assert.Equal(t, "new binary", read(t, binary.path))
	assert.True(t, binary.restarted)
	assert.NoFileExists(t, binary.path+failedSuffix, "stale marker must be dropped")
}

func TestSelfUpdaterIgnoresMalformedMarker(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	u, binary := updaterFixture(t, "v0.7.548", archive, sumsFor(archive))
	require.NoError(t, os.WriteFile(binary.path+failedSuffix, []byte("not a version"), markerMode))

	require.NoError(t, u.checkAndUpdate(nil))

	assert.Equal(t, "new binary", read(t, binary.path))
	assert.True(t, binary.restarted)
	assert.NoFileExists(t, binary.path+failedSuffix)
}

func TestSelfUpdaterAsksBeforeInstalling(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	u, binary := updaterFixture(t, "v0.7.548", archive, sumsFor(archive))
	u.isApprovalRequired = true

	errs := make(chan error, 1)
	go func() { errs <- u.checkAndUpdate(nil) }()

	pending := waitPendingUpdate(t, u)
	assert.Equal(t, testVersion, pending.CurrentVersion)
	assert.Equal(t, "0.7.548", pending.NewVersion)
	assert.Equal(t, "current binary", read(t, binary.path), "nothing is installed while the answer is out")

	u.AnswerUpdate(true)
	require.NoError(t, <-errs)

	assert.Equal(t, "new binary", read(t, binary.path))
	assert.True(t, binary.restarted)
	assert.Empty(t, u.GetPendingUpdate().NewVersion, "an answered release must stop waiting")
}

func TestSelfUpdaterKeepsBinaryWhenDeclined(t *testing.T) {
	archive := tarGz(t, testBinary, []byte("new binary"))
	u, binary := updaterFixture(t, "v0.7.548", archive, sumsFor(archive))
	u.isApprovalRequired = true

	errs := make(chan error, 1)
	go func() { errs <- u.checkAndUpdate(nil) }()

	waitPendingUpdate(t, u)
	u.AnswerUpdate(false)
	require.NoError(t, <-errs)

	assert.Equal(t, "current binary", read(t, binary.path))
	assert.False(t, binary.restarted)
	assert.NoFileExists(t, binary.path+oldSuffix)

	require.NoError(t, u.checkAndUpdate(nil))
	assert.Empty(t, u.GetPendingUpdate().NewVersion, "the same release was offered twice")
	assert.Equal(t, "current binary", read(t, binary.path))
}

func TestSelfUpdaterAsksAgainAboutANewerRelease(t *testing.T) {
	u, _ := updaterFixture(t, "v0.7.548", nil, nil)
	u.isApprovalRequired = true

	verdicts := make(chan bool, 1)
	go func() { verdicts <- u.isAllowed(semver.MustParse("0.7.548")) }()
	waitPendingUpdate(t, u)
	u.AnswerUpdate(false)
	require.False(t, <-verdicts)

	go func() { verdicts <- u.isAllowed(semver.MustParse("0.7.549")) }()
	waitPendingUpdate(t, u)
	u.AnswerUpdate(true)
	assert.True(t, <-verdicts)
}

func TestSelfUpdaterRefusesWhenShuttingDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	u, _ := updaterFixture(t, "v0.7.548", nil, nil)
	u.ctx = ctx
	u.isApprovalRequired = true

	verdicts := make(chan bool, 1)
	go func() { verdicts <- u.isAllowed(semver.MustParse("0.7.548")) }()

	waitPendingUpdate(t, u)
	cancel()

	select {
	case isAllowed := <-verdicts:
		assert.False(t, isAllowed, "a node shutting down must not install anything")
	case <-time.After(approverWait):
		t.Fatal("shutdown left the check waiting")
	}
}

func TestSelfUpdaterDropsUnexpectedAnswer(t *testing.T) {
	u, _ := updaterFixture(t, "v0.7.548", nil, nil)
	u.isApprovalRequired = true
	require.NotPanics(t, func() { u.AnswerUpdate(true) })

	verdicts := make(chan bool, 1)
	go func() { verdicts <- u.isAllowed(semver.MustParse("0.7.548")) }()

	waitPendingUpdate(t, u)
	select {
	case <-verdicts:
		t.Fatal("the dropped answer was served to the next release")
	case <-time.After(100 * time.Millisecond):
	}

	u.AnswerUpdate(false)
	select {
	case isAllowed := <-verdicts:
		assert.False(t, isAllowed)
	case <-time.After(approverWait):
		t.Fatal("the answer never reached the check")
	}
}

func waitPendingUpdate(t *testing.T, u *SelfUpdater) domain.UpdateInfo {
	t.Helper()
	deadline := time.Now().Add(approverWait)
	for time.Now().Before(deadline) {
		if info := u.GetPendingUpdate(); info.NewVersion != "" {
			return info
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no release is waiting for an answer")
	return domain.UpdateInfo{}
}

func TestMemberArtifact(t *testing.T) {
	a := MemberArtifact()
	switch runtime.GOOS {
	case "linux":
		require.True(t, a.isSupported())
		assert.Equal(t, "warpnet_linux_"+runtime.GOARCH+".tar.gz", a.AssetName)
		assert.Equal(t, "warpnet_linux_"+runtime.GOARCH+"_checksums.txt", a.ChecksumName)
		assert.Equal(t, "warpnet", a.BinaryName)
	case "darwin":
		assert.False(t, a.isSupported(), "the signed macOS build is not replaced in place")
	case "windows":
		require.True(t, a.isSupported())
		assert.Equal(t, "warpnet_windows_amd64.zip", a.AssetName)
		assert.Equal(t, "warpnet_windows_amd64_checksums.txt", a.ChecksumName)
		assert.Equal(t, "warpnet.exe", a.BinaryName)
	default:
		assert.False(t, a.isSupported(), "no release publishes a member asset for %s", runtime.GOOS)
	}
}

func TestArtifactSupport(t *testing.T) {
	assert.False(t, Artifact{}.isSupported())
	assert.False(t, Artifact{AssetName: testAsset}.isSupported())
	assert.True(t, Artifact{
		AssetName: testAsset, ChecksumName: testChecksum, BinaryName: testBinary,
	}.isSupported())
}

func read(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

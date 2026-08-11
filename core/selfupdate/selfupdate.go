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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const (
	ErrAssetNotFound    warpnet.WarpError = "release asset not found"
	ErrChecksumNotFound warpnet.WarpError = "checksum not found"
	ErrChecksumMismatch warpnet.WarpError = "checksum mismatch"
	ErrBinaryNotFound   warpnet.WarpError = "binary not found in archive"
	ErrUnexpectedStatus warpnet.WarpError = "unexpected response status"
	ErrTooLarge         warpnet.WarpError = "downloaded data too large"
	ErrNoReleaseTag     warpnet.WarpError = "release has no tag"
)

const (
	// latestReleaseAPI resolves to the newest published Warpnet release.
	latestReleaseAPI = "https://api.github.com/repos/Warp-net/warpnet/releases/latest"

	checkInterval   = time.Hour
	initialDelay    = time.Minute
	downloadTimeout = 10 * time.Minute

	maxMetadataSize = 1 << 20   // release JSON and checksum listings
	maxArchiveSize  = 128 << 20 // compressed release asset
	maxBinarySize   = 512 << 20 // decompression bomb guard

	binaryMode = 0o755
	markerMode = 0o600

	newSuffix    = ".new"
	oldSuffix    = ".old"
	failedSuffix = ".failed"
)

// Artifact points at the release asset carrying a replacement for the running
// binary. A zero Artifact means the running platform has no published asset and
// self-update stays off.
type Artifact struct {
	AssetName    string // archive attached to the release
	ChecksumName string // SHA-256 listing of AssetName
	BinaryName   string // executable inside the archive
}

func (a Artifact) isSupported() bool {
	return a.AssetName != "" && a.ChecksumName != "" && a.BinaryName != ""
}

// RelayArtifact returns the relay asset for the running platform. Releases
// publish the relay for linux/amd64 only.
func RelayArtifact() Artifact {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return Artifact{}
	}
	return Artifact{
		AssetName:    "relay_linux_amd64.tar.gz",
		ChecksumName: "relay_linux_amd64_checksums.txt",
		BinaryName:   "relay",
	}
}

type SelfUpdater struct {
	ctx      context.Context
	current  *semver.Version
	artifact Artifact
	client   *http.Client
	apiURL   string
	execPath string
	interval time.Duration
	stopChan chan struct{}
	restartF func(execPath string, shutdownF func()) error
	fatalF   func(format string, args ...any)
}

func NewSelfUpdater(ctx context.Context, current *semver.Version, a Artifact) *SelfUpdater {
	return &SelfUpdater{
		ctx:      ctx,
		current:  current,
		artifact: a,
		client:   &http.Client{Timeout: downloadTimeout},
		apiURL:   latestReleaseAPI,
		interval: checkInterval,
		stopChan: make(chan struct{}),
		restartF: restart,
		fatalF:   log.Fatalf,
	}
}

// Run starts the background update loop. shutdownF releases node resources right
// before the process is replaced by the new binary; it may be nil.
func (u *SelfUpdater) Run(shutdownF func()) {
	if u == nil {
		return
	}
	if u.current == nil {
		log.Errorln("selfupdate: current version is unknown, service disabled")
		return
	}
	if !u.artifact.isSupported() {
		log.Infof("selfupdate: no release asset for %s/%s, service disabled", runtime.GOOS, runtime.GOARCH)
		return
	}
	execPath, err := os.Executable()
	if err != nil {
		log.Errorf("selfupdate: fail resolving own executable: %v", err)
		return
	}
	u.execPath = execPath

	log.Infof("selfupdate: service started, current version %s", u.current)

	go func() {
		timer := time.NewTimer(initialDelay)
		defer timer.Stop()

		for {
			select {
			case <-u.ctx.Done():
				return
			case <-u.stopChan:
				return
			case <-timer.C:
				if err := u.checkAndUpdate(shutdownF); err != nil {
					log.Errorf("selfupdate: %v", err)
				}
				timer.Reset(u.interval)
			}
		}
	}()
}

func (u *SelfUpdater) Close() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("selfupdate: close recovered from panic: %v", r)
		}
	}()
	if u == nil || u.stopChan == nil {
		return
	}
	close(u.stopChan)
	log.Infoln("selfupdate: closed")
}

func (u *SelfUpdater) checkAndUpdate(shutdownF func()) error {
	rel, err := u.latestRelease()
	if err != nil {
		return err
	}

	latest, err := semver.NewVersion(strings.TrimSpace(rel.TagName))
	if err != nil {
		return fmt.Errorf("selfupdate: parsing release tag %s: %w", rel.TagName, err)
	}
	if !latest.GreaterThan(u.current) {
		log.Debugf("selfupdate: version %s is up to date", u.current)
		return nil
	}
	if u.isFailedVersion(latest) {
		log.Warnf("selfupdate: version %s failed to start before, skipping", latest)
		return nil
	}

	assetURL, err := rel.assetURL(u.artifact.AssetName)
	if err != nil {
		return err
	}
	checksumURL, err := rel.assetURL(u.artifact.ChecksumName)
	if err != nil {
		return err
	}

	if u.current.Major() != latest.Major() {
		// PSK is derived from the major version: after the restart this node
		// shares a network only with peers running the new major.
		log.Warnf(
			"selfupdate: major version change %d -> %d, private network key rotates",
			u.current.Major(), latest.Major(),
		)
	}
	log.Infof("selfupdate: updating %s -> %s", u.current, latest)

	restore, err := u.install(assetURL, checksumURL)
	if err != nil {
		return err
	}
	u.forgetFailedVersion() // an earlier failure is superseded by this release
	log.Infof("selfupdate: version %s installed, restarting...", latest)

	if err := u.restartF(u.execPath, shutdownF); err != nil {
		// The node is already stopped here: put the previous binary back, mark the
		// release as unusable so the next process start does not retry it, and let
		// the supervisor bring the process up again.
		restore()
		u.rememberFailedVersion(latest)
		u.fatalF("selfupdate: fail restarting, previous binary restored: %v", err)
	}
	return nil
}

// isFailedVersion reports whether v is the release that already failed to
// replace the running process. Without the marker a failed release is retried
// on every process start, and a binary that cannot be executed turns into a
// download loop.
func (u *SelfUpdater) isFailedVersion(v *semver.Version) bool {
	data, err := os.ReadFile(u.execPath + failedSuffix) //nolint:gosec // path sits next to the running executable
	if err != nil {
		return false
	}
	failed, err := semver.NewVersion(strings.TrimSpace(string(data)))
	if err != nil {
		log.Errorf("selfupdate: malformed failure marker: %s", data)
		u.forgetFailedVersion()
		return false
	}
	return failed.Equal(v)
}

func (u *SelfUpdater) rememberFailedVersion(v *semver.Version) {
	if err := os.WriteFile(u.execPath+failedSuffix, []byte(v.String()), markerMode); err != nil {
		log.Errorf("selfupdate: fail writing failure marker: %v", err)
	}
}

func (u *SelfUpdater) forgetFailedVersion() {
	if err := os.Remove(u.execPath + failedSuffix); err != nil && !os.IsNotExist(err) {
		log.Errorf("selfupdate: fail removing failure marker: %v", err)
	}
}

// install puts the released binary in place of the running one and returns a
// func restoring the previous binary.
func (u *SelfUpdater) install(assetURL, checksumURL string) (func(), error) {
	newPath, err := u.fetchBinary(assetURL, checksumURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(newPath) }() // no-op once installed

	return swapBinary(u.execPath, newPath)
}

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type release struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

func (r release) assetURL(name string) (string, error) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a.URL, nil
		}
	}
	return "", fmt.Errorf("selfupdate: %w: %s", ErrAssetNotFound, name)
}

func (u *SelfUpdater) latestRelease() (rel release, err error) {
	body, err := u.get(u.apiURL, maxMetadataSize)
	if err != nil {
		return rel, err
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return rel, fmt.Errorf("selfupdate: decoding release: %w", err)
	}
	if strings.TrimSpace(rel.TagName) == "" {
		return rel, fmt.Errorf("selfupdate: %w: %s", ErrNoReleaseTag, u.apiURL)
	}
	return rel, nil
}

// fetchBinary downloads the release archive next to the running executable,
// verifies its checksum and extracts the new binary, returning its path.
func (u *SelfUpdater) fetchBinary(assetURL, checksumURL string) (string, error) {
	sums, err := u.get(checksumURL, maxMetadataSize)
	if err != nil {
		return "", err
	}
	want, err := checksum(sums, u.artifact.AssetName)
	if err != nil {
		return "", err
	}

	archivePath := filepath.Join(filepath.Dir(u.execPath), u.artifact.AssetName)
	defer func() { _ = os.Remove(archivePath) }()

	got, err := u.downloadTo(assetURL, archivePath)
	if err != nil {
		return "", err
	}
	if got != want {
		return "", fmt.Errorf(
			"selfupdate: %w: %s: got %s, want %s", ErrChecksumMismatch, u.artifact.AssetName, got, want,
		)
	}

	binPath := u.execPath + newSuffix
	if err := extractBinary(archivePath, u.artifact.BinaryName, binPath); err != nil {
		_ = os.Remove(binPath)
		return "", err
	}
	return binPath, nil
}

func (u *SelfUpdater) do(url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(u.ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: building request %s: %w", url, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", warpnet.WarpnetName+"/"+u.current.String())

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: requesting %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("selfupdate: %w: %s: %s", ErrUnexpectedStatus, url, resp.Status)
	}
	return resp, nil
}

func (u *SelfUpdater) get(url string, limit int64) ([]byte, error) {
	resp, err := u.do(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("selfupdate: reading %s: %w", url, err)
	}
	if int64(len(data)) == limit {
		return nil, fmt.Errorf("selfupdate: %w: %s", ErrTooLarge, url)
	}
	return data, nil
}

// downloadTo streams url into path and returns the SHA-256 of what was written.
func (u *SelfUpdater) downloadTo(url, path string) (_ string, err error) {
	resp, err := u.do(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	f, err := os.Create(path) //nolint:gosec // path sits next to the running executable
	if err != nil {
		return "", fmt.Errorf("selfupdate: creating %s: %w", path, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("selfupdate: closing %s: %w", path, closeErr)
		}
	}()

	h := sha256.New()
	written, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxArchiveSize))
	if err != nil {
		return "", fmt.Errorf("selfupdate: downloading %s: %w", url, err)
	}
	if written == maxArchiveSize {
		return "", fmt.Errorf("selfupdate: %w: %s", ErrTooLarge, url)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// checksum picks the hash of assetName out of a `shasum -a 256` listing.
func checksum(sums []byte, assetName string) (string, error) {
	for line := range strings.SplitSeq(string(sums), "\n") {
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
	f, err := os.Open(archivePath) //nolint:gosec // archive was written by downloadTo
	if err != nil {
		return fmt.Errorf("selfupdate: opening %s: %w", archivePath, err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
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

// swapBinary installs newPath as execPath, keeping the previous binary next to
// it. Replacing the path of a running executable is safe: the running process
// keeps the old inode. The returned func puts the previous binary back.
func swapBinary(execPath, newPath string) (func(), error) {
	oldPath := execPath + oldSuffix
	_ = os.Remove(oldPath)

	if err := os.Rename(execPath, oldPath); err != nil {
		return nil, fmt.Errorf("selfupdate: moving current binary aside: %w", err)
	}
	if err := os.Rename(newPath, execPath); err != nil {
		_ = os.Rename(oldPath, execPath)
		return nil, fmt.Errorf("selfupdate: installing new binary: %w", err)
	}

	return func() {
		if err := os.Rename(oldPath, execPath); err != nil {
			log.Errorf("selfupdate: fail restoring previous binary: %v", err)
		}
	}, nil
}

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
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
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
	ErrRestartFailed    warpnet.WarpError = "fail restarting"
)

const (
	checkInterval = time.Hour
	initialDelay  = time.Minute
)

// architectures a release publishes an asset for
const (
	archAMD64 = "amd64"
	archARM64 = "arm64"
)

// ReleaseSource resolves the newest published release.
type ReleaseSource interface {
	Latest() (Release, error)
}

// AssetFetcher retrieves release assets.
type AssetFetcher interface {
	// Read returns a small asset, such as a checksum listing.
	Read(url string) ([]byte, error)
	// Download streams an asset into dstPath and returns its SHA-256.
	Download(url, dstPath string) (checksum string, err error)
}

// BinaryReplacer owns the binary of the running process.
type BinaryReplacer interface {
	// StagePath returns a scratch path on the filesystem holding the binary.
	StagePath(name string) string
	// Install puts the binary at path in place of the running one and returns a
	// func rolling back to the previous binary.
	Install(path string) (rollback func(), err error)
	// Restart hands the process over to the installed binary. On success it does
	// not return.
	Restart(shutdownF func()) error
}

// FailureRegistry remembers the release that could not replace the binary.
type FailureRegistry interface {
	Has(v *semver.Version) bool
	Set(v *semver.Version)
	Clear()
}

// UpdateApprover asks the owner of the node whether a release may replace the
// running binary. An unattended node - a relay - has no approver and installs
// every release on its own.
type UpdateApprover interface {
	IsUpdateAllowed(info domain.UpdateInfo) bool
}

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
	if runtime.GOOS != "linux" || runtime.GOARCH != archAMD64 {
		return Artifact{}
	}
	return Artifact{
		AssetName:    "relay_linux_amd64.tar.gz",
		ChecksumName: "relay_linux_amd64_checksums.txt",
		BinaryName:   "relay",
	}
}

// MemberArtifact returns the member asset for the running platform. Self-update
// covers linux (amd64 and arm64) and windows/amd64. macOS is deliberately left
// out: its signed build is published by a separate pipeline, and replacing the
// binary underneath it invalidates the signature.
func MemberArtifact() Artifact {
	switch runtime.GOOS {
	case "linux":
		if runtime.GOARCH != archAMD64 && runtime.GOARCH != archARM64 {
			return Artifact{}
		}
		return Artifact{
			AssetName:    "warpnet_linux_" + runtime.GOARCH + ".tar.gz",
			ChecksumName: "warpnet_linux_" + runtime.GOARCH + "_checksums.txt",
			BinaryName:   warpnet.WarpnetName,
		}
	case "windows":
		if runtime.GOARCH != archAMD64 {
			return Artifact{}
		}
		return Artifact{
			AssetName:    "warpnet_windows_amd64.zip",
			ChecksumName: "warpnet_windows_amd64_checksums.txt",
			BinaryName:   warpnet.WarpnetName + ".exe",
		}
	}
	return Artifact{}
}

// SelfUpdater keeps the running binary in sync with the newest release.
type SelfUpdater struct {
	ctx      context.Context
	current  *semver.Version
	artifact Artifact
	releases ReleaseSource
	assets   AssetFetcher
	binary   BinaryReplacer
	failures FailureRegistry
	approver UpdateApprover
	declined *semver.Version
	interval time.Duration
	stopChan chan struct{}
}

// NewSelfUpdater builds the update service. approver may be nil: the release is
// then installed without asking anyone.
func NewSelfUpdater(
	ctx context.Context,
	current *semver.Version,
	a Artifact,
	approver UpdateApprover,
) *SelfUpdater {
	gh := newGitHubReleases(ctx, current)

	u := &SelfUpdater{
		ctx:      ctx,
		current:  current,
		artifact: a,
		releases: gh,
		assets:   gh,
		approver: approver,
		interval: checkInterval,
		stopChan: make(chan struct{}),
	}

	binary, err := currentExecutable()
	if err != nil {
		log.Errorf("selfupdate: fail resolving own executable: %v", err)
		return u // Run reports the service as disabled
	}
	u.binary = binary
	u.failures = newFailureMarker(binary.Path())

	return u
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
	if u.binary == nil {
		log.Errorln("selfupdate: no binary to replace, service disabled")
		return
	}

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
				u.tick(shutdownF)
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

func (u *SelfUpdater) tick(shutdownF func()) {
	err := u.checkAndUpdate(shutdownF)
	switch {
	case err == nil:
	case errors.Is(err, ErrRestartFailed):
		// The node is already stopped and the previous binary is back in place:
		// only the supervisor can bring this process up again.
		log.Fatalf("selfupdate: %v", err)
	default:
		log.Errorf("selfupdate: %v", err)
	}
}

func (u *SelfUpdater) checkAndUpdate(shutdownF func()) error {
	rel, err := u.releases.Latest()
	if err != nil {
		return err
	}
	if !rel.Version.GreaterThan(u.current) {
		log.Debugf("selfupdate: version %s is up to date", u.current)
		return nil
	}
	if u.failures.Has(rel.Version) {
		log.Warnf("selfupdate: version %s failed to start before, skipping", rel.Version)
		return nil
	}
	if !u.isAllowed(rel.Version) {
		return nil
	}
	if u.current.Major() != rel.Version.Major() {
		// PSK is derived from the major version: after the restart this node
		// shares a network only with peers running the new major.
		log.Warnf(
			"selfupdate: major version change %d -> %d, private network key rotates",
			u.current.Major(), rel.Version.Major(),
		)
	}
	log.Infof("selfupdate: updating %s -> %s", u.current, rel.Version)

	rollback, err := u.install(rel)
	if err != nil {
		return err
	}
	u.failures.Clear() // an earlier failure is superseded by this release
	log.Infof("selfupdate: version %s installed, restarting...", rel.Version)

	if err := u.binary.Restart(shutdownF); err != nil {
		rollback()
		u.failures.Set(rel.Version)
		return fmt.Errorf("%w, previous binary restored: %w", ErrRestartFailed, err)
	}
	return nil
}

// isAllowed asks the owner of the node to allow the release. A release turned
// down is not offered again until the process restarts, so a declined update
// doesn't ask again on every check.
func (u *SelfUpdater) isAllowed(next *semver.Version) bool {
	if u.approver == nil {
		return true
	}
	if u.declined != nil && u.declined.Equal(next) {
		log.Debugf("selfupdate: version %s is declined, skipping", next)
		return false
	}

	isAllowed := u.approver.IsUpdateAllowed(domain.UpdateInfo{
		CurrentVersion: u.current.String(),
		NewVersion:     next.String(),
	})
	if !isAllowed {
		u.declined = next
		log.Infof("selfupdate: version %s is declined by the user", next)
	}
	return isAllowed
}

// install puts the released binary in place of the running one and returns a
// func restoring the previous binary.
func (u *SelfUpdater) install(rel Release) (func(), error) {
	path, err := u.stage(rel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(path) }() // no-op once installed

	return u.binary.Install(path)
}

// stage downloads the release archive onto the filesystem of the running binary,
// checks it against the published SHA-256 and extracts the new binary from it.
func (u *SelfUpdater) stage(rel Release) (string, error) {
	assetURL, err := rel.AssetURL(u.artifact.AssetName)
	if err != nil {
		return "", err
	}
	checksumURL, err := rel.AssetURL(u.artifact.ChecksumName)
	if err != nil {
		return "", err
	}

	listing, err := u.assets.Read(checksumURL)
	if err != nil {
		return "", err
	}
	want, err := checksumOf(listing, u.artifact.AssetName)
	if err != nil {
		return "", err
	}

	archivePath := u.binary.StagePath(u.artifact.AssetName)
	defer func() { _ = os.Remove(archivePath) }()

	got, err := u.assets.Download(assetURL, archivePath)
	if err != nil {
		return "", err
	}
	if got != want {
		return "", fmt.Errorf(
			"selfupdate: %w: %s: got %s, want %s", ErrChecksumMismatch, u.artifact.AssetName, got, want,
		)
	}

	binaryPath := u.binary.StagePath(u.artifact.BinaryName + newSuffix)
	if err := extractBinary(archivePath, u.artifact.BinaryName, binaryPath); err != nil {
		_ = os.Remove(binaryPath)
		return "", err
	}
	return binaryPath, nil
}

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
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/security"
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
	ErrNoPendingUpdate  warpnet.WarpError = "no release is waiting for an answer"
	ErrReleaseUnsigned  warpnet.WarpError = "release carries no signature"
	ErrMalformedKey     warpnet.WarpError = "malformed release signing key"
	ErrNoReleaseSource  warpnet.WarpError = "no forge to read releases from"
)

// releaseSigningKey is the hex-encoded ed25519 public key whose private half
// signs the checksum listing of every published release. Empty turns the check
// off: the binary then trusts whatever the release host serves it. A build can
// also carry its own key with
// -ldflags "-X github.com/Warp-net/warpnet/core/selfupdate.releaseSigningKey=<hex>".
var releaseSigningKey = ""

const (
	checkInterval = time.Hour
	initialDelay  = time.Minute
)

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

// Artifact points at the release asset carrying a replacement for the running
// binary. A zero Artifact means the running platform has no published asset and
// self-update stays off.
type Artifact struct {
	AssetName     string // archive attached to the release
	ChecksumName  string // SHA-256 listing of AssetName
	SignatureName string // ed25519 signature of ChecksumName
	BinaryName    string // executable inside the archive
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
		AssetName:     "relay_linux_amd64.tar.gz",
		ChecksumName:  "relay_linux_amd64_checksums.txt",
		SignatureName: "relay_linux_amd64_checksums.txt.sig",
		BinaryName:    "relay",
	}
}

func MemberArtifact() Artifact {
	switch runtime.GOOS {
	case "linux":
		if runtime.GOARCH != archAMD64 && runtime.GOARCH != archARM64 {
			return Artifact{}
		}
		return Artifact{
			AssetName:     "warpnet_linux_" + runtime.GOARCH + ".tar.gz",
			ChecksumName:  "warpnet_linux_" + runtime.GOARCH + "_checksums.txt",
			SignatureName: "warpnet_linux_" + runtime.GOARCH + "_checksums.txt.sig",
			BinaryName:    warpnet.WarpnetName,
		}
	case "windows":
		if runtime.GOARCH != archAMD64 {
			return Artifact{}
		}
		return Artifact{
			AssetName:     "warpnet_windows_amd64.zip",
			ChecksumName:  "warpnet_windows_amd64_checksums.txt",
			SignatureName: "warpnet_windows_amd64_checksums.txt.sig",
			BinaryName:    warpnet.WarpnetName + ".exe",
		}
	}
	return Artifact{}
}

// SelfUpdater keeps the running binary in sync with the newest release. A node
// with an owner to ask - a member node - holds every release until the frontend
// answers for it; an unattended one installs it right away.
type SelfUpdater struct {
	ctx                context.Context
	current            *semver.Version
	artifact           Artifact
	releases           ReleaseSource
	assets             AssetFetcher
	binary             BinaryReplacer
	failures           FailureRegistry
	interval           time.Duration
	stopChan           chan struct{}
	isApprovalRequired bool
	mx                 sync.RWMutex
	verdicts           chan bool
	pending            string
	declined           string
}

func NewSelfUpdater(
	ctx context.Context,
	current *semver.Version,
	a Artifact,
	isApprovalRequired bool,
) *SelfUpdater {
	assets := newAssetClient(ctx, current)

	u := &SelfUpdater{
		ctx:      ctx,
		current:  current,
		artifact: a,
		releases: forgeSources{
			newForgeReleases(assets, codebergReleaseAPI),
			newForgeReleases(assets, githubReleaseAPI),
		},
		assets:             assets,
		interval:           checkInterval,
		stopChan:           make(chan struct{}),
		isApprovalRequired: isApprovalRequired,
		verdicts:           make(chan bool),
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
	key, err := releaseKey()
	if err != nil {
		log.Errorf("selfupdate: %v, service disabled", err)
		return
	}
	if key == nil {
		log.Warnln("selfupdate: releases are not signed, the release host is trusted as it is")
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

// GetPendingUpdate returns the version waiting for the owner of the node to
// allow it, alongside the one it replaces. An empty newVersion means nothing is
// waiting.
func (u *SelfUpdater) GetPendingUpdate() (currentVersion, newVersion string) {
	u.mx.RLock()
	defer u.mx.RUnlock()
	if u.pending == "" {
		return "", ""
	}
	return u.current.String(), u.pending
}

// AnswerUpdate hands the owner's verdict to the waiting check.
func (u *SelfUpdater) AnswerUpdate(isAllowed bool) error {
	select {
	case u.verdicts <- isAllowed:
		return nil
	default:
		return ErrNoPendingUpdate
	}
}

func (u *SelfUpdater) isAllowed(next *semver.Version) bool {
	if !u.isApprovalRequired {
		return true
	}
	if !u.startAsking(next) {
		log.Debugf("selfupdate: version %s is declined, skipping", next)
		return false
	}

	log.Infof("selfupdate: waiting for permission to update %s -> %s", u.current, next)

	var isAllowed bool
	select {
	case <-u.ctx.Done():
	case isAllowed = <-u.verdicts:
	}

	u.stopAsking(isAllowed)
	if !isAllowed {
		log.Infof("selfupdate: version %s is not allowed", next)
	}
	return isAllowed
}

func (u *SelfUpdater) startAsking(next *semver.Version) bool {
	u.mx.Lock()
	defer u.mx.Unlock()
	if u.declined == next.String() {
		return false
	}
	u.pending = next.String()
	return true
}

func (u *SelfUpdater) stopAsking(isAllowed bool) {
	u.mx.Lock()
	defer u.mx.Unlock()
	if !isAllowed {
		u.declined = u.pending
	}
	u.pending = ""
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

// verifyListing proves the checksum listing was published by whoever holds the
// release signing key. The checksums bind the archive, so a listing that is
// signed makes the release host untrusted plumbing; without a key nothing is
// proved and the host is trusted as before.
func (u *SelfUpdater) verifyListing(rel Release, listing []byte) error {
	key, err := releaseKey()
	if err != nil {
		return err
	}
	if key == nil {
		return nil
	}

	signatureURL, err := rel.AssetURL(u.artifact.SignatureName)
	if err != nil {
		return fmt.Errorf("selfupdate: %w: %w", ErrReleaseUnsigned, err)
	}
	signature, err := u.assets.Read(signatureURL)
	if err != nil {
		return err
	}
	if err := security.VerifySignature(key, listing, strings.TrimSpace(string(signature))); err != nil {
		return fmt.Errorf("selfupdate: %s: %w", u.artifact.ChecksumName, err)
	}
	return nil
}

func releaseKey() (ed25519.PublicKey, error) {
	if releaseSigningKey == "" {
		return nil, nil
	}
	key, err := hex.DecodeString(releaseSigningKey)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: %w: %w", ErrMalformedKey, err)
	}
	if len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("selfupdate: %w: %d bytes", ErrMalformedKey, len(key))
	}
	return key, nil
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
	if err := u.verifyListing(rel, listing); err != nil {
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

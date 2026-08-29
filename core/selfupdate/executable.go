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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Masterminds/semver/v3"
	log "github.com/sirupsen/logrus"
)

const (
	newSuffix    = ".new"
	oldSuffix    = ".old"
	failedSuffix = ".failed"

	markerMode = 0o600
)

// executable is the binary of the running process.
type executable struct {
	path string
}

func currentExecutable() (*executable, error) {
	path, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("selfupdate: resolving own executable: %w", err)
	}
	return &executable{path: path}, nil
}

func (e *executable) Path() string {
	return e.path
}

// StagePath returns a path next to the running binary. Staged files share its
// filesystem, so installing them is a rename and never a copy.
func (e *executable) StagePath(name string) string {
	return filepath.Join(filepath.Dir(e.path), name)
}

// Install puts the binary at path in place of the running one, keeping the
// previous binary next to it. Replacing the path of a running executable is
// safe: the running process keeps the old inode. The returned func puts the
// previous binary back.
func (e *executable) Install(path string) (func(), error) {
	previous := e.path + oldSuffix
	_ = os.Remove(previous)

	if err := os.Rename(e.path, previous); err != nil {
		return nil, fmt.Errorf("selfupdate: moving current binary aside: %w", err)
	}
	if err := os.Rename(path, e.path); err != nil {
		_ = os.Rename(previous, e.path)
		return nil, fmt.Errorf("selfupdate: installing new binary: %w", err)
	}

	return func() {
		if err := os.Rename(previous, e.path); err != nil {
			log.Errorf("selfupdate: fail restoring previous binary: %v", err)
		}
	}, nil
}

// Restart releases node resources and replaces the process image with the
// installed binary. The PID survives, so a containerized node stays PID 1 and no
// supervisor has to be involved. On success it does not return.
func (e *executable) Restart(shutdownF func()) error {
	if shutdownF != nil {
		shutdownF()
	}
	if err := syscall.Exec(e.path, os.Args, os.Environ()); err != nil { //nolint:gosec // this node's own binary
		return fmt.Errorf("selfupdate: exec %s: %w", e.path, err)
	}
	return nil
}

// failureMarker records, next to the binary, the release that could not replace
// it. Without the marker a failed release is retried on every process start, and
// a binary that cannot be executed turns into a download loop.
type failureMarker struct {
	path string
}

func newFailureMarker(binaryPath string) failureMarker {
	return failureMarker{path: binaryPath + failedSuffix}
}

func (m failureMarker) Has(v *semver.Version) bool {
	data, err := os.ReadFile(m.path) //nolint:gosec // path sits next to the running executable
	if err != nil {
		return false
	}
	failed, err := semver.NewVersion(strings.TrimSpace(string(data)))
	if err != nil {
		log.Errorf("selfupdate: malformed failure marker: %s", data)
		m.Clear()
		return false
	}
	return failed.Equal(v)
}

func (m failureMarker) Set(v *semver.Version) {
	if err := os.WriteFile(m.path, []byte(v.String()), markerMode); err != nil {
		log.Errorf("selfupdate: fail writing failure marker: %v", err)
	}
}

func (m failureMarker) Clear() {
	if err := os.Remove(m.path); err != nil && !os.IsNotExist(err) {
		log.Errorf("selfupdate: fail removing failure marker: %v", err)
	}
}

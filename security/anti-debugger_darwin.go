//go:build darwin

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

package security

import (
	"os"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func init() {
	// macOS injects libraries via DYLD_* rather than LD_PRELOAD.
	_ = os.Unsetenv("DYLD_INSERT_LIBRARIES")
	_ = os.Unsetenv("LD_PRELOAD")
	disableCoreDumps()
	denyDebugger()
}

func disableCoreDumps() {
	// A core dump would flush the in-memory DB encryption key to disk;
	// RLIMIT_CORE=0 is the macOS equivalent of PR_SET_DUMPABLE=0.
	if err := setCoreDumpLimit(0); err != nil {
		log.Fatalf("failed to disable core dumps: %v", err)
	}
}

func setCoreDumpLimit(limit uint64) error {
	return unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: limit, Max: limit})
}

func denyDebugger() {
	// PT_DENY_ATTACH makes later ptrace attaches fail and terminates the
	// process if it was launched under a debugger. There is no /proc on
	// macOS, so this replaces the TracerPid scan used on Linux.
	if err := unix.PtraceDenyAttach(); err != nil {
		log.Fatalf("failed to deny debugger attach: %v", err)
	}
}

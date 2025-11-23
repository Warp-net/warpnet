//go:build !windows && !darwin

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
	"bytes"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func init() {
	_ = os.Unsetenv("LD_PRELOAD")
	// enableCoreDumps()
	disableCoreDumps()
	mustNotGDBAttached()
}

func enableCoreDumps() {
	err := unix.Prctl(unix.PR_SET_DUMPABLE, 1, 0, 0, 0)
	if err != nil {
		log.Fatalf("failed to enable core dumps: %v", err)
	}
}

func disableCoreDumps() {
	//tick := time.NewTicker(10 * time.Second)
	//defer tick.Stop()
	//<-tick.C
	err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0)
	if err != nil {
		log.Fatalf("failed to disable core dumps: %v", err)
	}
}

func mustNotGDBAttached() {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return
	}
	if !bytes.Contains(data, []byte("TracerPid:\t0")) {
		output, err := os.ReadFile("/proc/self/cmdline")
		if err == nil {
			cmd := strings.ToLower(string(output))
			if strings.Contains(cmd, "gdb") || strings.Contains(cmd, "lldb") {
				panic("gdb detected")
			}
		}
	}
	return
}

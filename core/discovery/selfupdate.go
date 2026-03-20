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

WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"fmt"
	"os"

	"github.com/creativeprojects/go-selfupdate"
	log "github.com/sirupsen/logrus"
)

const warpnetRepository = "Warp-net/warpnet"

// selfUpdate detects whether a newer release of the bootstrap node is available
// on GitHub and, if so, replaces the running binary with the updated version.
// After a successful update the process exits so the supervisor / container
// runtime can restart it with the new binary.
func selfUpdate(ctx context.Context, currentVersion string) error {
	log.Infof("discovery: self-update: checking for updates (current version %s)", currentVersion)

	latest, found, err := selfupdate.DetectLatest(ctx, selfupdate.ParseSlug(warpnetRepository))
	if err != nil {
		return fmt.Errorf("discovery: self-update: failed to detect latest release: %w", err)
	}
	if !found {
		return fmt.Errorf("discovery: self-update: no release found for %s", warpnetRepository)
	}

	if latest.LessOrEqual(currentVersion) {
		log.Infof("discovery: self-update: current version %s is already up to date", currentVersion)
		return nil
	}

	log.Infof("discovery: self-update: newer version %s found, updating...", latest.Version())

	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		return fmt.Errorf("discovery: self-update: failed to get executable path: %w", err)
	}

	if err := selfupdate.UpdateTo(ctx, latest.AssetURL, latest.AssetName, exe); err != nil {
		return fmt.Errorf("discovery: self-update: failed to update binary: %w", err)
	}

	log.Infof("discovery: self-update: successfully updated to version %s, restarting...", latest.Version())
	os.Exit(0)
	return nil
}

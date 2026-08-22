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
	"errors"
	"fmt"
	"io/fs"
	"strconv"

	"github.com/Masterminds/semver/v3"
)

var (
	ErrPSKNetworkRequired = errors.New("psk: network required")
	ErrPSKVersionRequired = errors.New("psk: version required")
)

type FileSystem interface {
	ReadDir(name string) ([]fs.DirEntry, error)
	ReadFile(name string) ([]byte, error)
	Open(name string) (fs.File, error)
}

type PSK []byte

func (s PSK) String() string {
	return fmt.Sprintf("%x", []byte(s))
}

// GeneratePSK - Preshared Secret Key is public for Warpnet goals - it's just separate networks and versions
func GeneratePSK(network string, v *semver.Version) (PSK, error) {
	if network == "" {
		return nil, ErrPSKNetworkRequired
	}
	if v == nil {
		return nil, ErrPSKVersionRequired
	}

	if network == "mainnet" {
		network = "warpnet"
	}
	majorStr := strconv.FormatUint(v.Major(), 10)

	seed := append([]byte(network), []byte(majorStr)...)
	return ConvertToSHA256(seed), nil
}

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

package ratelimit

type Settings struct {
	NetworkLowWater      int `json:"network_low_water"`
	NetworkHighWater     int `json:"network_high_water"`
	DiscoveryBurst       int `json:"discovery_burst"`
	DiscoveryPerTenSec   int `json:"discovery_per_ten_sec"`
	StreamReadBurst      int `json:"stream_read_burst"`
	StreamReadPerMinute  int `json:"stream_read_per_minute"`
	StreamWriteBurst     int `json:"stream_write_burst"`
	StreamWritePerMinute int `json:"stream_write_per_minute"`
}

var Defaults = Settings{
	NetworkLowWater:      20,
	NetworkHighWater:     50,
	DiscoveryBurst:       32,
	DiscoveryPerTenSec:   2,
	StreamReadBurst:      60,
	StreamReadPerMinute:  300,
	StreamWriteBurst:     30,
	StreamWritePerMinute: 120,
}

func (s Settings) WithDefaults() Settings {
	if s.NetworkLowWater <= 0 {
		s.NetworkLowWater = Defaults.NetworkLowWater
	}
	if s.NetworkHighWater <= 0 {
		s.NetworkHighWater = Defaults.NetworkHighWater
	}
	if s.DiscoveryBurst <= 0 {
		s.DiscoveryBurst = Defaults.DiscoveryBurst
	}
	if s.DiscoveryPerTenSec <= 0 {
		s.DiscoveryPerTenSec = Defaults.DiscoveryPerTenSec
	}
	if s.StreamReadBurst <= 0 {
		s.StreamReadBurst = Defaults.StreamReadBurst
	}
	if s.StreamReadPerMinute <= 0 {
		s.StreamReadPerMinute = Defaults.StreamReadPerMinute
	}
	if s.StreamWriteBurst <= 0 {
		s.StreamWriteBurst = Defaults.StreamWriteBurst
	}
	if s.StreamWritePerMinute <= 0 {
		s.StreamWritePerMinute = Defaults.StreamWritePerMinute
	}
	return s
}

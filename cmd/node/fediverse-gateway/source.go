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

package main

// warpnetUser is the minimal user shape the gateway renders into an actor.
type warpnetUser struct {
	ID                string
	PreferredUsername string
	DisplayName       string
	Summary           string
}

// warpnetSource resolves the Warpnet user a given actor handle maps to.
//
// nodeSource (nodeclient.go) is the node-agnostic implementation: it resolves
// ANY requested handle live from the Warpnet network via PUBLIC_GET_USER.
// staticSource is a single-user dev fallback, used only when the network is
// unreachable.
type warpnetSource interface {
	GetUser(preferredUsername string) (warpnetUser, bool)
}

type staticSource struct {
	user warpnetUser
}

func (s staticSource) GetUser(preferredUsername string) (warpnetUser, bool) {
	if preferredUsername != s.user.PreferredUsername {
		return warpnetUser{}, false
	}
	return s.user, true
}

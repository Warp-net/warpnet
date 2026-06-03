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

// warpnetSource yields the Warpnet user a given actor handle maps to.
//
// SKELETON: staticSource returns a single operator-configured user so the
// Phase-1 milestone (discover + follow from Mastodon) is reachable without any
// node wiring. Phase 2/3 replace this with a node-backed implementation that
// reads via the PUBLIC_GET_USER route — either a libp2p stream client like
// warpdroid/node, or an embedded member node.
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

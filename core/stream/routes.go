/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

package stream

import (
	"github.com/Warp-net/warpnet/core/warpnet"
	"strings"
)

type WarpRoute string

func (r WarpRoute) ProtocolID() warpnet.WarpProtocolID {
	return warpnet.WarpProtocolID(r)
}

func (r WarpRoute) String() string {
	return string(r)
}

func (r WarpRoute) IsPrivate() bool {
	return strings.Contains(string(r), "private")
}

func (r WarpRoute) IsGet() bool {
	return strings.Contains(string(r), "get")
}

func IsValidRoute(route WarpRoute) bool { // TODO
	if !strings.HasPrefix(route.String(), "/") {
		return false
	}
	if !(strings.Contains(route.String(), "get") ||
		strings.Contains(route.String(), "delete") ||
		strings.Contains(route.String(), "post")) {
		return false
	}
	if !(strings.Contains(route.String(), "private") ||
		strings.Contains(route.String(), "public")) {
		return false
	}
	return true
}

type WarpRoutes []WarpRoute

func (rs WarpRoutes) FromRoutesToPrIDs() []warpnet.WarpProtocolID {
	prIDs := make([]warpnet.WarpProtocolID, 0, len(rs))
	for _, r := range rs {
		prIDs = append(prIDs, r.ProtocolID())
	}
	return prIDs
}

func FromPrIDToRoute(prID warpnet.WarpProtocolID) WarpRoute {
	return WarpRoute(prID)
}

func FromProtocolIDToRoutes(prIDs []warpnet.WarpProtocolID) WarpRoutes {
	rs := make(WarpRoutes, 0, len(prIDs))
	for _, p := range prIDs {
		rs = append(rs, WarpRoute(p))
	}
	return rs
}

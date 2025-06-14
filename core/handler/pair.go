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

package handler

import (
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

func StreamNodesPairingHandler(serverAuthInfo domain.AuthNodeInfo) middleware.WarpHandler {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var clientInfo domain.AuthNodeInfo
		if err := json.JSON.Unmarshal(buf, &clientInfo); err != nil {
			log.Errorf("pair: unmarshaling from stream: %v", err)
			return nil, err
		}
		tokenMatch := serverAuthInfo.Identity.Token == clientInfo.Identity.Token
		if !tokenMatch {
			log.Errorf("pair: token does not match server identity")
			return nil, warpnet.WarpError("token mismatch")
		}
		return event.Accepted, nil
	}
}

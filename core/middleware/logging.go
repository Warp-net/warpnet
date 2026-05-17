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

package middleware

import (
	"runtime/debug"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	log "github.com/sirupsen/logrus"
)

func (p *WarpMiddleware) LoggingMiddleware(next warpnet.StreamHandler) warpnet.StreamHandler {
	return func(s warpnet.WarpStream) {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("middleware: panic: %v %s", r, debug.Stack())
			}
		}() //#nosec

		log.Debugf("middleware: server stream opened: %s %s\n", s.Protocol(), s.Conn().RemotePeer())
		before := time.Now()
		next(s)
		log.Debugf(
			"middleware: server stream closed: %s %s, elapsed: %s\n",
			s.Protocol(),
			s.Conn().RemotePeer(),
			time.Since(before).String(),
		)
	}
}

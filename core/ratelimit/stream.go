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

import "github.com/Warp-net/warpnet/event"

var (
	delivery  = PerMinute(200, 1200)
	media     = PerMinute(150, 600)
	messaging = PerMinute(60, 300)
	upload    = PerMinute(10, 30)
	report    = PerMinute(10, 30)
	pairing   = PerMinute(5, 15)
	gateway   = PerMinute(600, 6000)
)

// StreamLimits is what a peer may spend on a node's routes: the budgets the
// owner sets for reads and writes, and the ones a route carries of its own.
type StreamLimits struct {
	read, write Limit
	routes      map[string]Limit
}

func NewStreamLimits(s Settings) StreamLimits {
	s = s.WithDefaults()
	read := PerMinute(int64(s.StreamReadBurst), int64(s.StreamReadPerMinute))
	write := PerMinute(int64(s.StreamWriteBurst), int64(s.StreamWritePerMinute))

	return StreamLimits{
		read:  read,
		write: write,
		routes: map[string]Limit{
			event.PUBLIC_GET_IMAGE: media,
			event.PUBLIC_GET_VIDEO: media,

			event.PRIVATE_POST_UPLOAD_IMAGE: upload,
			event.PRIVATE_POST_UPLOAD_VIDEO: upload,

			event.PUBLIC_POST_TIMELINE:          delivery,
			event.PUBLIC_POST_MODERATION_RESULT: delivery,

			event.PUBLIC_POST_CHAT:    messaging,
			event.PUBLIC_POST_MESSAGE: messaging,

			event.PUBLIC_POST_IS_FOLLOWING:       read,
			event.PUBLIC_POST_IS_FOLLOWER:        read,
			event.PUBLIC_POST_VIEW:               read,
			event.PRIVATE_POST_NOTIFICATION_READ: read,

			event.PUBLIC_POST_REPORT: report,

			event.PRIVATE_POST_PAIR:          pairing,
			event.PUBLIC_POST_NODE_CHALLENGE: pairing,
		},
	}
}

func (l StreamLimits) Route(path string) (Limit, bool) {
	limit, ok := l.routes[path]
	return limit, ok
}

func (l StreamLimits) Read() Limit    { return l.read }
func (l StreamLimits) Write() Limit   { return l.write }
func (l StreamLimits) Gateway() Limit { return gateway }

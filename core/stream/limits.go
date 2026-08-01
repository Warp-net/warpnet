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

package stream

import (
	"time"

	"github.com/docker/go-units"
)

const (
	// MaxInboundSize is the inbound ceiling for every request, whatever the
	// route. It has to cover the largest thing any handler accepts: a video
	// upload, whose payload is base64 (so 4/3 of the file) plus the signed
	// envelope around it. handler.maxVideoSize is set below that headroom so
	// an oversized video gets a clear error instead of being truncated.
	MaxInboundSize = units.MiB * 50

	// IODeadline bounds a single request/response exchange. It is generous
	// because the same ceiling covers a streamed archive import, which parses
	// and stores a tweet with photos before replying.
	IODeadline = 10 * time.Minute
)

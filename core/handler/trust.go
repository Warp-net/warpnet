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

package handler

import (
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

// maxTrustBatch is one page of tweets' worth of authors.
const maxTrustBatch = 100

const ErrTrustBatchTooLarge = warpnet.WarpError("trust: too many user ids")

// TrustCircle places users in the owner's trust circle.
type TrustCircle interface {
	Trust(userId string) domain.Trust
}

// StreamGetTrustHandler answers from memory only: it never dials anyone on
// the request path.
func StreamGetTrustHandler(circle TrustCircle) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		var ev event.GetTrustEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if len(ev.UserIds) > maxTrustBatch {
			return nil, ErrTrustBatchTooLarge
		}
		users := make(map[domain.ID]domain.Trust, len(ev.UserIds))
		for _, userId := range ev.UserIds {
			if userId == "" {
				continue
			}
			users[userId] = circle.Trust(userId)
		}
		return event.GetTrustResponse{Users: users}, nil
	}
}

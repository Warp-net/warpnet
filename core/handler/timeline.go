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
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

type TimelineFetcher interface {
	GetTimeline(string, *uint64, *string) ([]domain.Tweet, string, error)
}

func StreamTimelineHandler(repo TimelineFetcher) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetTimelineEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty user id")
		}

		timeline, cursor, err := repo.GetTimeline(ev.UserId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}

		if timeline == nil {
			timeline = []domain.Tweet{}
		}
		return event.TweetsResponse{
			Cursor: cursor,
			Tweets: timeline,
			UserId: ev.UserId,
		}, nil
	}
}

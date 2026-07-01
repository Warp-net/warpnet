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

type BookmarkStorer interface {
	Bookmark(userId, tweetId, ownerUserId string) error
	Unbookmark(userId, tweetId string) error
	List(userId string, limit *uint64, cursor *string) ([]domain.Bookmark, string, error)
}

func StreamBookmarkHandler(repo BookmarkStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.BookmarkEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("bookmark: empty user id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("bookmark: empty tweet id")
		}
		if ev.OwnerUserId == "" {
			return nil, warpnet.WarpError("bookmark: empty owner user id")
		}

		if err := repo.Bookmark(ev.UserId, ev.TweetId, ev.OwnerUserId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

func StreamUnbookmarkHandler(repo BookmarkStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UnbookmarkEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("unbookmark: empty user id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("unbookmark: empty tweet id")
		}

		if err := repo.Unbookmark(ev.UserId, ev.TweetId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

func StreamGetBookmarksHandler(repo BookmarkStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetBookmarksEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("bookmarks: empty user id")
		}

		bms, cur, err := repo.List(ev.UserId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		items := make([]event.BookmarkItem, 0, len(bms))
		for _, bm := range bms {
			items = append(items, event.BookmarkItem{
				UserId:      bm.UserId,
				TweetId:     bm.TweetId,
				OwnerUserId: bm.OwnerUserId,
				CreatedAt:   bm.CreatedAt,
			})
		}
		return event.GetBookmarksResponse{Items: items, Cursor: cur}, nil
	}
}

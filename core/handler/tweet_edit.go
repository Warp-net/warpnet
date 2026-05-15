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

// EditTweetStorer is the narrow surface the edit handler needs from tweet-repo.
type EditTweetStorer interface {
	Get(userID, tweetID string) (domain.Tweet, error)
	Update(tweet domain.Tweet) error
}

// TweetEditsAppender records an immutable edit revision.
type TweetEditsAppender interface {
	Append(edit domain.TweetEdit) (domain.TweetEdit, error)
	List(tweetId string, limit *uint64, cursor *string) ([]domain.TweetEdit, string, error)
}

func StreamEditTweetHandler(repo EditTweetStorer, editsRepo TweetEditsAppender) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.EditTweetEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("edit tweet: empty user id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("edit tweet: empty tweet id")
		}
		if ev.Text == "" {
			return nil, warpnet.WarpError("edit tweet: empty text")
		}

		existing, err := repo.Get(ev.UserId, ev.TweetId)
		if err != nil {
			return nil, err
		}
		if existing.UserId != ev.UserId {
			return nil, warpnet.WarpError("edit tweet: only the author can edit their own tweet")
		}
		if existing.Text == ev.Text {
			// No-op edit — return the existing tweet without recording a revision.
			return event.EditTweetResponse(existing), nil
		}

		// Append the *previous* text as a revision so the user can see what
		// the tweet looked like before this edit.
		if _, err := editsRepo.Append(domain.TweetEdit{
			OriginalTweetId: existing.Id,
			UserId:          existing.UserId,
			Text:            existing.Text,
		}); err != nil {
			return nil, err
		}

		updated := existing
		updated.Text = ev.Text
		if err := repo.Update(updated); err != nil {
			return nil, err
		}
		// Re-fetch so the response carries the storage-canonical UpdatedAt.
		out, err := repo.Get(existing.UserId, existing.Id)
		if err != nil {
			return nil, err
		}
		return event.EditTweetResponse(out), nil
	}
}

func StreamGetTweetEditsHandler(editsRepo TweetEditsAppender) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetTweetEditsEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("get tweet edits: empty tweet id")
		}
		edits, cur, err := editsRepo.List(ev.TweetId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		return event.TweetEditsResponse{Edits: edits, Cursor: cur}, nil
	}
}

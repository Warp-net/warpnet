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

// QuoteStorer is the slice of tweet-repo the quote handlers need.
type QuoteStorer interface {
	Get(userID, tweetID string) (domain.Tweet, error)
	Create(userId string, tweet domain.Tweet) (domain.Tweet, error)
	Delete(userID, tweetID string) error
	AppendQuoting(quotedId string, quoteTweet domain.Tweet) error
	Quoting(tweetId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error)
}

func StreamNewQuoteHandler(repo QuoteStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var q event.NewQuoteEvent
		if err := json.Unmarshal(buf, &q); err != nil {
			return nil, err
		}
		if q.UserId == "" {
			return nil, warpnet.WarpError("quote: empty user id")
		}
		if q.QuotedTweetId == nil || *q.QuotedTweetId == "" {
			return nil, warpnet.WarpError("quote: empty quoted tweet id")
		}
		if q.QuotedUserId == nil || *q.QuotedUserId == "" {
			return nil, warpnet.WarpError("quote: empty quoted user id")
		}
		if q.Text == "" {
			return nil, warpnet.WarpError("quote: empty text")
		}

		created, err := repo.Create(q.UserId, q)
		if err != nil {
			return nil, err
		}
		// Index for the Quoting list. Best-effort: a missing index
		// entry only affects discovery, not the created tweet itself.
		if err := repo.AppendQuoting(*q.QuotedTweetId, created); err != nil {
			return created, err
		}
		return created, nil
	}
}

func StreamDeleteQuoteHandler(repo QuoteStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.DeleteQuoteEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("delete quote: empty user id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("delete quote: empty tweet id")
		}
		// Author-only delete — mirrors the tweet-delete handler.
		existing, err := repo.Get(ev.UserId, ev.TweetId)
		if err != nil {
			return nil, err
		}
		if existing.UserId != ev.UserId {
			return nil, warpnet.WarpError("delete quote: only the author can delete their own quote")
		}
		if err := repo.Delete(ev.UserId, ev.TweetId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

func StreamGetQuotingHandler(repo QuoteStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetQuotingEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("get quoting: empty tweet id")
		}
		tweets, cur, err := repo.Quoting(ev.TweetId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		return event.GetQuotingResponse{Cursor: cur, Tweets: tweets, UserId: ev.OwnerUserId}, nil
	}
}

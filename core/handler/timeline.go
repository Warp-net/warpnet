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
	log "github.com/sirupsen/logrus"
)

const ErrForeignTweetAuthor = warpnet.WarpError("timeline: tweet did not come from its author's node")

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

func StreamTimelineNewTweetHandler(
	authRepo OwnerTweetStorer,
	tweetRepo TweetsStorer,
	timelineRepo TimelineUpdater,
	followRepo TweetFollowChecker,
	userRepo TweetUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.NewTweetEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}

		author, err := userRepo.Get(ev.UserId)
		if err != nil || author.NodeId == "" {
			log.Warnf("timeline: dropping tweet claiming to be from %s", ev.UserId)
			return nil, ErrForeignTweetAuthor
		}

		if s == nil || s.Conn() == nil || author.NodeId != s.Conn().RemotePeer().String() {
			log.Warnf("timeline: dropping tweet claiming to be from %s", ev.UserId)
			return nil, ErrForeignTweetAuthor
		}

		if ev.Moderation != nil && !ev.Moderation.IsOk {
			return nil, tweetRepo.Blocklist(ev.Id)
		}
		if err := validateTweetEvent(ev); err != nil {
			return nil, err
		}

		owner := authRepo.GetOwner()
		if owner.UserId == ev.UserId {
			return event.Accepted, nil
		}
		if followRepo == nil || !followRepo.IsFollowing(owner.UserId, ev.UserId) {
			return event.Accepted, nil
		}

		tweet, err := tweetRepo.Create(ev.UserId, ev)
		if err != nil {
			return nil, err
		}
		if tweet.Id == "" {
			return tweet, warpnet.WarpError("timeline handler: empty tweet id")
		}
		if err := timelineRepo.AddTweetToTimeline(owner.UserId, tweet); err != nil {
			log.Infof("fail adding tweet to timeline: %v", err)
		}
		return tweet, nil
	}
}

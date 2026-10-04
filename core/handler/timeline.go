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
	"math/big"
	"strings"
	"unicode/utf8"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const (
	timelineTweetCharLimit      = 280
	timelineSponsoredPriceLimit = 1_000_000_000_000
	timelinePollMinOptions      = 2
	timelinePollMaxOptions      = 4
	timelinePollOptionRuneLimit = 25
)

type TimelineFetcher interface {
	GetTimeline(string, *uint64, *string) ([]domain.Tweet, string, error)
}

type TimelineAuthStorer interface {
	GetOwner() domain.Owner
}

type TimelineTweetStorer interface {
	Blocklist(tweetId string) error
	Create(_ string, tweet domain.Tweet) (domain.Tweet, error)
	Delete(userID, tweetID string) error
}

type TimelineStorer interface {
	AddTweetToTimeline(userId string, tweet domain.Tweet) error
	DeleteTweetFromTimeline(userID, tweetID string) error
}

type TimelineFollowChecker interface {
	IsFollowing(ownerId, authorId string) bool
}

type TimelineUserFetcher interface {
	Get(userId string) (user domain.User, err error)
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
	authRepo TimelineAuthStorer,
	tweetRepo TimelineTweetStorer,
	timelineRepo TimelineStorer,
	followRepo TimelineFollowChecker,
	userRepo TimelineUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.NewTweetEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}

		author, _ := userRepo.Get(ev.UserId)
		if err := warpnet.VerifyAuthorship(s, author.NodeId); err != nil {
			return nil, err
		}

		if ev.Moderation != nil && !ev.Moderation.IsOk {
			return nil, tweetRepo.Blocklist(ev.Id)
		}
		if err := validateTimelineTweet(ev); err != nil {
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

func StreamTimelineDeleteTweetHandler(
	authRepo TimelineAuthStorer,
	tweetRepo TimelineTweetStorer,
	timelineRepo TimelineStorer,
	userRepo TimelineUserFetcher,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.DeleteTweetEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}

		author, _ := userRepo.Get(ev.UserId)
		if err := warpnet.VerifyAuthorship(s, author.NodeId); err != nil {
			return nil, err
		}

		owner := authRepo.GetOwner()
		if owner.UserId == ev.UserId {
			return event.Accepted, nil
		}
		if err := tweetRepo.Delete(ev.UserId, ev.TweetId); err != nil {
			return nil, err
		}
		if err := timelineRepo.DeleteTweetFromTimeline(owner.UserId, ev.TweetId); err != nil {
			log.Errorf("timeline: delete tweet: %v", err)
		}
		return event.Accepted, nil
	}
}

func validateTimelineTweet(ev event.NewTweetEvent) error {
	if ev.UserId == "" {
		return warpnet.WarpError("empty user id")
	}
	if ev.Text == "" && !ev.IsSponsored() {
		return warpnet.WarpError("empty tweet text")
	}
	if utf8.RuneCountInString(ev.Text) > timelineTweetCharLimit {
		return warpnet.WarpError("tweet text is too long")
	}
	if ev.IsSponsored() {
		if !ev.Price.IsPositive() {
			return warpnet.WarpError("sponsored tweet: price must be positive")
		}
		if ev.Price.Units.Cmp(big.NewInt(timelineSponsoredPriceLimit)) > 0 {
			return warpnet.WarpError("sponsored tweet: price is above 1000000 USDT")
		}
		if ev.Poll != nil {
			return warpnet.WarpError("sponsored tweet: poll is not allowed")
		}
	}
	return validateTimelinePoll(ev.Poll)
}

func validateTimelinePoll(p *domain.Poll) error {
	if p == nil {
		return nil
	}
	if len(p.Options) < timelinePollMinOptions {
		return warpnet.WarpError("poll: too few options")
	}
	if len(p.Options) > timelinePollMaxOptions {
		return warpnet.WarpError("poll: too many options")
	}
	if p.ExpiresAt.IsZero() {
		return warpnet.WarpError("poll: empty expiration time")
	}
	for _, opt := range p.Options {
		if strings.TrimSpace(opt) == "" {
			return warpnet.WarpError("poll: empty option")
		}
		if utf8.RuneCountInString(opt) > timelinePollOptionRuneLimit {
			return warpnet.WarpError("poll: option is too long")
		}
	}
	return nil
}

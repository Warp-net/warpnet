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
	"errors"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type RetweetStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

type RetweetedUserFetcher interface {
	GetBatch(retweetersIds ...string) (users []domain.User, err error)
	Get(userId string) (users domain.User, err error)
}

type OwnerReTweetStorer interface {
	GetOwner() domain.Owner
}

type ReTweetsStorer interface {
	Get(userID, tweetID string) (tweet domain.Tweet, err error)
	NewRetweet(tweet domain.Tweet) (_ domain.Tweet, err error)
	UnRetweet(retweetedByUserID, tweetId string) error
	RetweetsCount(tweetId string) (uint64, error)
	Retweeters(tweetId string, limit *uint64, cursor *string) (_ []string, cur string, err error)
}

type RetweetTimelineUpdater interface {
	AddTweetToTimeline(userId string, tweet domain.Tweet) error
}

func StreamNewReTweetHandler(
	userRepo RetweetedUserFetcher,
	tweetRepo ReTweetsStorer,
	timelineRepo RetweetTimelineUpdater,
	streamer RetweetStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var retweetEvent event.NewRetweetEvent
		err := json.Unmarshal(buf, &retweetEvent)
		if err != nil {
			return nil, err
		}
		if retweetEvent.RetweetedBy == nil {
			return nil, warpnet.WarpError("retweeted by unknown")
		}
		if retweetEvent.Id == "" {
			return nil, warpnet.WarpError("empty retweet id")
		}

		retweet, err := tweetRepo.NewRetweet(retweetEvent)
		if err != nil {
			return nil, err
		}

		ownerId := streamer.NodeInfo().OwnerId
		if ownerId == *retweetEvent.RetweetedBy { // I'm a retweeter
			// owner retweeted it
			if err = timelineRepo.AddTweetToTimeline(ownerId, retweet); err != nil {
				log.Infof("fail adding retweet to timeline: %v", err)
			}

		}
		if ownerId == retweetEvent.UserId { // my own tweet retweet
			return retweet, nil
		}

		tweetOwner, err := userRepo.Get(retweetEvent.UserId)
		if err != nil {
			return nil, err
		}

		retweetDataResp, err := streamer.GenericStream(
			tweetOwner.NodeId,
			event.PUBLIC_POST_RETWEET,
			event.NewRetweetEvent(retweet),
		)
		if err != nil && !errors.Is(err, warpnet.ErrNodeIsOffline) {
			return nil, err
		}
		var possibleError event.ErrorResponse
		if _ = json.Unmarshal(retweetDataResp, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other retweet error response: %s", possibleError.Message)
		}

		return retweet, nil
	}
}

func StreamUnretweetHandler(
	tweetRepo ReTweetsStorer,
	userRepo RetweetedUserFetcher,
	streamer RetweetStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UnretweetEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		if ev.RetweeterId == "" {
			return nil, warpnet.WarpError("empty retweeter id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("empty tweet id")
		}

		retweetedBy := ev.RetweeterId

		tweet, err := tweetRepo.Get(retweetedBy, ev.TweetId)
		if err != nil {
			return nil, err
		}
		err = tweetRepo.UnRetweet(retweetedBy, ev.TweetId)
		if err != nil {
			return nil, err
		}

		ownerId := streamer.NodeInfo().OwnerId
		if tweet.UserId == ownerId {
			// tweet belongs to owner, unretweet themself
			return event.Accepted, nil
		}

		tweetOwner, err := userRepo.Get(tweet.UserId)
		if err != nil {
			return nil, err
		}

		unretweetDataResp, err := streamer.GenericStream(
			tweetOwner.NodeId,
			event.PUBLIC_POST_UNRETWEET,
			ev,
		)
		if err != nil && !errors.Is(err, warpnet.ErrNodeIsOffline) {
			return nil, err
		}
		var possibleError event.ErrorResponse
		if _ = json.Unmarshal(unretweetDataResp, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other unretweet error response: %s", possibleError.Message)
		}

		return event.Accepted, nil
	}
}

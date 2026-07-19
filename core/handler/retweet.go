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
	"github.com/Warp-net/warpnet/database"
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
	IncrSharedRetweetsCount(tweetId string) error
	DecrSharedRetweetsCount(tweetId string) error
}

type RetweetTimelineUpdater interface {
	AddTweetToTimeline(userId string, tweet domain.Tweet) error
}

func StreamNewReTweetHandler(
	userRepo RetweetedUserFetcher,
	tweetRepo ReTweetsStorer,
	timelineRepo RetweetTimelineUpdater,
	notifyRepo ModerationNotifier,
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
			log.Errorf("retweet handler failed: %v", err)
			return nil, err
		}

		// A quote is a regular tweet authored by the retweeter that
		// references another tweet through QuotedTweetId / QuotedUserId.
		// For plain retweets the wire's UserId is the source author; for
		// quotes it's the retweeter (the comment author), and the source
		// author lives in QuotedUserId. Pick the right id for routing /
		// notification accordingly.
		isQuote := retweetEvent.QuotedTweetId != nil && *retweetEvent.QuotedTweetId != ""
		sourceAuthorId := retweetEvent.UserId
		if isQuote && retweetEvent.QuotedUserId != nil && *retweetEvent.QuotedUserId != "" {
			sourceAuthorId = *retweetEvent.QuotedUserId
		}

		ownNodeInfo := streamer.NodeInfo()
		ownerId := ownNodeInfo.OwnerId
		isOwnerRetweeter := ownerId == *retweetEvent.RetweetedBy
		if isOwnerRetweeter {
			// owner retweeted it
			if err = timelineRepo.AddTweetToTimeline(ownerId, retweet); err != nil {
				log.Infof("fail adding retweet to timeline: %v", err)
			}
		}

		isOwnTweetRetweet := ownerId == sourceAuthorId // my own tweet retweet
		if isOwnTweetRetweet {                         //nolint:nestif
			if !isOwnerRetweeter {
				notifyUsername := *retweetEvent.RetweetedBy
				retweeter, retweeterErr := userRepo.Get(*retweetEvent.RetweetedBy)
				if retweeterErr == nil {
					notifyUsername = retweeter.Username
				}
				notifyText := notifyUsername + " retweeted your tweet"
				if isQuote {
					notifyText = notifyUsername + " quoted your tweet"
				}
				if err := notifyRepo.Add(domain.Notification{
					Type:   domain.NotificationRetweetType,
					Text:   notifyText,
					UserId: ownerId,
				}); err != nil {
					log.Errorf("retweet handler: adding notification: %v", err)
				}
			}
			return retweet, nil
		}

		tweetOwner, err := userRepo.Get(sourceAuthorId)
		if errors.Is(err, database.ErrUserNotFound) {
			return retweet, nil
		}
		if err != nil {
			return nil, err
		}

		if ownNodeInfo.ID.String() == tweetOwner.NodeId {
			return retweet, nil
		}

		retweetDataResp, err := streamer.GenericStream(
			tweetOwner.NodeId,
			event.PUBLIC_POST_RETWEET,
			event.NewRetweetEvent(retweet),
		)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return retweet, nil
		}
		if err != nil {
			return nil, err
		}

		// The source tweet's node has now stored (and counted) the forwarded
		// retweet and owns the network-wide retweets counter. Revert the CRDT
		// bump NewRetweet made here so one retweet isn't counted on both nodes.
		// NewRetweet keys the counter by the source id it received (retweetEvent.Id).
		if derr := tweetRepo.DecrSharedRetweetsCount(retweetEvent.Id); derr != nil {
			log.Errorf("retweet handler: revert shared retweets count: %v", derr)
		}

		var possibleError event.ResponseError
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
			log.Errorf("unretweet handler failed: %v", err)
			return nil, err
		}

		ownNodeInfo := streamer.NodeInfo()
		ownerId := ownNodeInfo.OwnerId
		isOwnTweetUnretweet := tweet.UserId == ownerId
		if isOwnTweetUnretweet {
			// tweet belongs to owner, unretweet themself
			return event.Accepted, nil
		}

		tweetOwner, err := userRepo.Get(tweet.UserId)
		if errors.Is(err, database.ErrUserNotFound) {
			return event.Accepted, nil
		}
		if err != nil {
			return nil, err
		}

		if ownNodeInfo.ID.String() == tweetOwner.NodeId {
			return event.Accepted, nil
		}

		unretweetDataResp, err := streamer.GenericStream(
			tweetOwner.NodeId,
			event.PUBLIC_POST_UNRETWEET,
			ev,
		)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return event.Accepted, nil
		}
		if err != nil {
			return nil, err
		}

		// Mirror StreamNewReTweetHandler: the source tweet's node owns the
		// network-wide retweets counter and has applied the unretweet, so revert
		// the CRDT decrement UnRetweet made here to keep the count single-owned.
		if ierr := tweetRepo.IncrSharedRetweetsCount(ev.TweetId); ierr != nil {
			log.Errorf("unretweet handler: revert shared retweets count: %v", ierr)
		}

		var possibleError event.ResponseError
		if _ = json.Unmarshal(unretweetDataResp, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other unretweet error response: %s", possibleError.Message)
		}

		return event.Accepted, nil
	}
}

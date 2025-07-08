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
	"fmt"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
	"time"
)

const defaultModerationModel = "llama2"

type ModerationStreamer interface {
	GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error)
}

type HandlerModerator interface {
	Moderate(content string) (bool, string, error)
	Close()
}

type ModerationBroadcaster interface {
	PublishUpdateToFollowers(ownerId, dest string, bt []byte) (err error)
}

// StreamModerateHandler receive event from pubsub via loopback
func StreamModerateHandler(streamer ModerationStreamer, moderator HandlerModerator) warpnet.WarpHandlerFunc {
	return func(buf []byte, _ warpnet.WarpStream) (any, error) {
		var ev event.ModerationEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if moderator == nil {
			return nil, errors.New("moderator is not initialized")
		}
		if streamer == nil {
			return nil, errors.New("streamer is not initialized")
		}

		if ev.ObjectID != nil {
			log.Infof("moderation: request received, object ID: %s", *ev.ObjectID)
		} else {
			log.Infoln("moderation: request received")
		}

		var result event.ModerationResultEvent
		switch ev.Type {
		case event.Tweet:
			result, err = handleTweet(ev, streamer, moderator)
		case event.User:
			result, err = handleUser(ev, streamer, moderator)
		//case event.Reply: // TODO
		//case event.Image:
		//case event.Other:
		default:
			return nil, errors.New("moderation: unknown event type")
		}
		if err != nil {
			return nil, err
		}

		result.Type = ev.Type
		return streamer.GenericStream(ev.NodeID, event.PUBLIC_POST_MODERATION_RESULT, result)
	}
}

func handleTweet(
	ev event.ModerationEvent,
	streamer ModerationStreamer,
	moderator HandlerModerator,
) (resp event.ModerationResultEvent, err error) {
	if ev.ObjectID == nil {
		return resp, errors.New("moderation: no object id provided")
	}
	getTweetEvent := event.GetTweetEvent{
		TweetId: *ev.ObjectID,
		UserId:  ev.UserID,
	}

	bt, err := streamer.GenericStream(
		ev.NodeID,
		event.PUBLIC_GET_TWEET,
		getTweetEvent,
	)
	if err != nil {
		return resp, err
	}

	var tweet domain.Tweet
	if err := json.Unmarshal(bt, &tweet); err != nil {
		return resp, err
	}

	result, reason, err := moderator.Moderate(tweet.Text)
	if err != nil {
		return resp, err
	}

	resp = event.ModerationResultEvent{
		NodeID:   ev.NodeID,
		UserID:   ev.UserID,
		ObjectID: ev.ObjectID,
	}
	if result {
		resp.Result = event.OK
	} else {
		resp.Result = event.FAIL
		resp.Reason = &reason
	}

	return resp, nil
}

func handleUser(
	ev event.ModerationEvent,
	streamer ModerationStreamer,
	moderator HandlerModerator,
) (resp event.ModerationResultEvent, err error) {
	if moderator == nil {
		return resp, errors.New("moderation: moderator is not initialized")
	}
	if streamer == nil {
		return resp, errors.New("moderation: streamer is not initialized")
	}
	getUserEvent := event.GetUserEvent{
		UserId: ev.UserID,
	}

	bt, err := streamer.GenericStream(
		ev.NodeID,
		event.PUBLIC_GET_USER,
		getUserEvent,
	)
	if err != nil {
		return resp, err
	}

	var user domain.User
	if err := json.Unmarshal(bt, &user); err != nil {
		return resp, err
	}

	text := fmt.Sprintf("%s: %s", user.Username, user.Bio)
	result, reason, err := moderator.Moderate(text)
	if err != nil {
		return resp, err
	}

	log.Infof("moderation: request received, object ID: %s", *ev.ObjectID)

	resp = event.ModerationResultEvent{
		NodeID: ev.NodeID,
		UserID: ev.UserID,
	}

	if result {
		resp.Result = event.OK
	} else {
		resp.Result = event.FAIL
		resp.Reason = &reason
	}
	return resp, nil
}

type UserUpdater interface {
	Update(userId string, newUser domain.User) (updatedUser domain.User, err error)
}

type TweetUpdater interface {
	Get(userID, tweetID string) (tweet domain.Tweet, err error)
	Create(_ string, tweet domain.Tweet) (domain.Tweet, error)
}

type TimelineTweetRemover interface {
	DeleteTweetFromTimeline(userID, tweetID string, createdAt time.Time) error
}

func StreamModerationResultHandler(
	broadcaster ModerationBroadcaster,
	userRepo UserUpdater,
	tweetRepo TweetUpdater,
	timelineRepo TimelineTweetRemover,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.ModerationResultEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		log.Infof("moderation: result received, result: %s", ev.Result.String())

		var (
			updatedAt          = time.Now()
			isModerationPassed = ev.Result == event.OK
		)

		switch ev.Type {
		case event.Tweet:
			if ev.ObjectID == nil {
				return nil, errors.New("moderation: no object id provided")
			}
			tweetModeration := &domain.TweetModeration{
				IsModerated: true,
				IsOk:        isModerationPassed,
				Reason:      ev.Reason,
				TimeAt:      updatedAt,
				Model:       defaultModerationModel,
			}

			if ev.ObjectID == nil {
				return nil, errors.New("moderation: no object id provided")
			}
			tweet, err := tweetRepo.Get(ev.UserID, *ev.ObjectID)
			if err != nil {
				return nil, err
			}

			tweet.Moderation = tweetModeration
			tweet.UpdatedAt = &updatedAt

			_, err = tweetRepo.Create(ev.UserID, tweet)
			if err != nil {
				return nil, err
			}
			if isModerationPassed {
				return event.Accepted, nil
			}

			err = timelineRepo.DeleteTweetFromTimeline(ev.UserID, *ev.ObjectID, tweet.CreatedAt)

			deleteEvent := event.DeleteTweetEvent{
				TweetId: *ev.ObjectID,
				UserId:  ev.UserID,
			}
			bt, _ := json.Marshal(deleteEvent)
			err = broadcaster.PublishUpdateToFollowers(ev.UserID, event.PRIVATE_DELETE_TWEET, bt)
			return event.Accepted, err
		case event.User:
			_, err = userRepo.Update(ev.UserID, domain.User{
				UpdatedAt: &updatedAt,
				Moderation: &domain.UserModeration{
					IsModerated: true,
					IsOk:        isModerationPassed,
					Reason:      ev.Reason,
					Strikes:     1, // TODO incr
					TimeAt:      updatedAt,
					Model:       defaultModerationModel,
				},
			})
			return event.Accepted, err

		default:
			return nil, errors.New("moderation: unknown event type")
		}
	}
}

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
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const (
	ErrNoObjectID warpnet.WarpError = "no object id found"
	ErrNoUserID   warpnet.WarpError = "no user id found"
)

type ModerationNotifier interface {
	Add(not domain.Notification) error
}

type ModerationTweetUpdater interface {
	Update(tweet domain.Tweet) error
}

type ModerationTimelelineDeleter interface {
	DeleteTweetFromTimeline(userID, tweetID string) error
}

type ModerationOwnerFetcher interface {
	GetOwner() domain.Owner
}

func StreamModerationResultHandler(
	notifyRepo ModerationNotifier,
	tweetRepo ModerationTweetUpdater,
	authRepo ModerationOwnerFetcher,
	timelineRepo ModerationTimelelineDeleter,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.ModerationResultEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		notificationText := "moderation result: "
		if ev.ObjectID != nil {
			notificationText += *ev.ObjectID + ": "
		}
		if ev.Reason != nil {
			notificationText += *ev.Reason
		}
		log.Infof("moderation: result received: %s", notificationText)

		switch ev.Type {
		case domain.ModerationTweetType:
			if ev.ObjectID == nil {
				return nil, ErrNoObjectID
			}
			if ev.UserID == "" {
				return nil, ErrNoUserID
			}

			moderatorId := ""
			if s.Conn() != nil {
				moderatorId = s.Conn().RemotePeer().String()
			}

			tweet := domain.Tweet{
				Id:     *ev.ObjectID,
				UserId: ev.UserID,
				Moderation: &domain.TweetModeration{
					ModeratorID: moderatorId,
					Model:       ev.Model,
					IsOk:        ev.Result,
					Reason:      ev.Reason,
					TimeAt:      time.Now(),
				},
			}

			if err := tweetRepo.Update(tweet); err != nil {
				log.Errorf("moderation: failed to update tweet: %v", err)
			}
			if err := timelineRepo.DeleteTweetFromTimeline(ev.UserID, *ev.ObjectID); err != nil {
				log.Errorf("moderation: failed to delete timeline: %v", err)
			}
		default:
			log.Errorf("moderation handler: unknown event type %s", ev.Type.String())
			return event.Accepted, nil
		}

		if ev.Result == domain.OK {
			log.Infof("moderation handler: OK")
			return event.Accepted, nil
		}

		owner := authRepo.GetOwner()
		if ev.UserID == owner.UserId {
			log.Infoln("moderation handler: adding notification")
			err := notifyRepo.Add(domain.Notification{
				Type:   domain.NotificationModerationType,
				Text:   notificationText,
				UserId: owner.UserId,
				IsRead: false,
			})
			if err != nil {
				log.Errorf("moderation handler: adding notification result: %v", err)
			}
		}

		return event.Accepted, nil
	}
}

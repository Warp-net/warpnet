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
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const defaultModerationModel = "llama2"

type ModerationBroadcaster interface {
	PublishUpdateToFollowers(ownerId, dest string, bt []byte) (err error)
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

func StreamModerationResultHandler() warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.ModerationResultEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		log.Infof("moderation: result received, result: %s", ev.Result.String())

		switch ev.Type {
		case event.Tweet:
			if ev.ObjectID == nil {
				return nil, errors.New("moderation: no object id provided")
			}

			return event.Accepted, err
		case event.User:

			return event.Accepted, err

		default:
			return nil, errors.New("moderation: unknown event type")
		}
	}
}

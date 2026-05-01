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
	"strings"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type ViewsStorer interface {
	RecordView(tweetId, viewerId string) (uint64, error)
	GetViewsCount(tweetId string) (uint64, error)
}

func StreamViewHandler(repo ViewsStorer, userRepo LikedUserFetcher, streamer LikeStreamer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.ViewEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("view: empty tweet id")
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("view: empty user id")
		}
		if ev.ViewerId == "" {
			return nil, warpnet.WarpError("view: empty viewer id")
		}

		tweetId := strings.TrimPrefix(ev.TweetId, domain.RetweetPrefix)

		// Author's own views are not counted.
		if ev.ViewerId == ev.UserId {
			count, err := repo.GetViewsCount(tweetId)
			if errors.Is(err, database.ErrViewsNotFound) {
				return event.ViewsCountResponse{Count: 0}, nil
			}
			if err != nil {
				return nil, err
			}
			return event.ViewsCountResponse{Count: count}, nil
		}

		count, err := repo.RecordView(tweetId, ev.ViewerId)
		if err != nil {
			log.Errorf("view handler failed: %v", err)
			return nil, err
		}

		// Forward the view to the tweet author's node so its CRDT
		// counter receives the increment too. Failures are non-fatal.
		if ev.UserId != streamer.NodeInfo().OwnerId {
			forwardViewToAuthor(ev, userRepo, streamer)
		}

		return event.ViewsCountResponse{Count: count}, nil
	}
}

func forwardViewToAuthor(ev event.ViewEvent, userRepo LikedUserFetcher, streamer LikeStreamer) {
	author, err := userRepo.Get(ev.UserId)
	if errors.Is(err, database.ErrUserNotFound) {
		return
	}
	if err != nil {
		log.Errorf("view handler: lookup author %s: %v", ev.UserId, err)
		return
	}

	viewResp, err := streamer.GenericStream(
		author.NodeId,
		event.PUBLIC_POST_VIEW,
		event.ViewEvent{
			TweetId:  ev.TweetId,
			UserId:   ev.UserId,
			ViewerId: ev.ViewerId,
		},
	)
	if errors.Is(err, warpnet.ErrNodeIsOffline) {
		return
	}
	if err != nil {
		log.Errorf("view handler: forwarding to author failed: %v", err)
		return
	}

	var possibleError event.ResponseError
	if _ = json.Unmarshal(viewResp, &possibleError); possibleError.Message != "" {
		log.Errorf("view handler: remote response error: %s", possibleError.Message)
	}
}

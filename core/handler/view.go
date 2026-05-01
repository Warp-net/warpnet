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

		// Author's node is the sole authority for incrementing the view
		// counter. The CRDT stats store replicates the value to every
		// other node, so non-author nodes only forward the event and
		// read back the count.
		if ev.UserId == streamer.NodeInfo().OwnerId {
			count, err := repo.RecordView(tweetId, ev.ViewerId)
			if err != nil {
				log.Errorf("view handler failed: %v", err)
				return nil, err
			}
			return event.ViewsCountResponse{Count: count}, nil
		}

		count := forwardViewToAuthor(ev, userRepo, streamer)
		if count == 0 {
			// Fall back to whatever the CRDT has propagated locally.
			if local, err := repo.GetViewsCount(tweetId); err == nil {
				count = local
			}
		}
		return event.ViewsCountResponse{Count: count}, nil
	}
}

func forwardViewToAuthor(ev event.ViewEvent, userRepo LikedUserFetcher, streamer LikeStreamer) uint64 {
	author, err := userRepo.Get(ev.UserId)
	if errors.Is(err, database.ErrUserNotFound) {
		return 0
	}
	if err != nil {
		log.Errorf("view handler: lookup author %s: %v", ev.UserId, err)
		return 0
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
		return 0
	}
	if err != nil {
		log.Errorf("view handler: forwarding to author failed: %v", err)
		return 0
	}

	var possibleError event.ResponseError
	if _ = json.Unmarshal(viewResp, &possibleError); possibleError.Message != "" {
		log.Errorf("view handler: remote response error: %s", possibleError.Message)
		return 0
	}

	var resp event.ViewsCountResponse
	if err := json.Unmarshal(viewResp, &resp); err != nil {
		log.Errorf("view handler: parse remote response: %v", err)
		return 0
	}
	return resp.Count
}

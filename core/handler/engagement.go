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

// LikersLister is the narrow surface the likers handler needs.
type LikersLister interface {
	Likers(tweetId string, limit *uint64, cursor *string) ([]string, string, error)
}

// RetweetersLister is the narrow surface the retweeters handler needs.
type RetweetersLister interface {
	Retweeters(tweetId string, limit *uint64, cursor *string) ([]string, string, error)
}

// EngagementStreamer forwards the lookup to the tweet author's node when
// the canonical engagement record lives elsewhere — mirrors the like.go
// propagation pattern.
type EngagementStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

func StreamGetTweetLikersHandler(
	repo LikersLister,
	userRepo LikedUserFetcher,
	streamer EngagementStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetTweetLikersEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("likers: empty tweet id")
		}

		if out, ok, err := forwardToOwner(ev.OwnerUserId, streamer, userRepo, event.PUBLIC_GET_TWEET_LIKERS, ev); err != nil {
			return nil, err
		} else if ok {
			return out, nil
		}

		ids, cur, err := repo.Likers(ev.TweetId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		users := hydrateUsers(userRepo, ids)
		return event.UsersResponse{Cursor: cur, Users: users}, nil
	}
}

func StreamGetTweetRetweetersHandler(
	repo RetweetersLister,
	userRepo LikedUserFetcher,
	streamer EngagementStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetTweetRetweetersEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("retweeters: empty tweet id")
		}

		if out, ok, err := forwardToOwner(ev.OwnerUserId, streamer, userRepo, event.PUBLIC_GET_TWEET_RETWEETERS, ev); err != nil {
			return nil, err
		} else if ok {
			return out, nil
		}

		ids, cur, err := repo.Retweeters(ev.TweetId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		users := hydrateUsers(userRepo, ids)
		return event.UsersResponse{Cursor: cur, Users: users}, nil
	}
}

// forwardToOwner attempts to fetch the engagement list from the tweet
// author's node when the caller isn't the owner. Returns (remote response,
// true) when the remote answered with a non-empty page, (zero, false)
// otherwise — caller then falls through to the local index.
func forwardToOwner(
	ownerUserId string,
	streamer EngagementStreamer,
	userRepo LikedUserFetcher,
	path stream.WarpRoute,
	ev any,
) (event.UsersResponse, bool, error) {
	if ownerUserId == "" || streamer == nil {
		return event.UsersResponse{}, false, nil
	}
	ownNode := streamer.NodeInfo()
	if ownerUserId == ownNode.OwnerId {
		return event.UsersResponse{}, false, nil
	}
	owner, err := userRepo.Get(ownerUserId)
	if errors.Is(err, database.ErrUserNotFound) {
		// Owner is not yet known locally — fall through to the local index.
		return event.UsersResponse{}, false, nil
	}
	if err != nil {
		return event.UsersResponse{}, false, err
	}
	if owner.NodeId == ownNode.ID.String() {
		return event.UsersResponse{}, false, nil
	}

	resp, fwdErr := streamer.GenericStream(owner.NodeId, path, ev)
	switch {
	case errors.Is(fwdErr, warpnet.ErrNodeIsOffline):
		return event.UsersResponse{}, false, nil
	case fwdErr != nil:
		return event.UsersResponse{}, false, fwdErr
	}
	var out event.UsersResponse
	if err := json.Unmarshal(resp, &out); err != nil {
		// Remote answered with something we can't parse — degrade to the
		// local index rather than failing the read.
		log.Warnf("engagement: forward to %s: unmarshal response: %v", owner.NodeId, err)
		return event.UsersResponse{}, false, nil
	}
	if out.Cursor == "" && len(out.Users) == 0 {
		return event.UsersResponse{}, false, nil
	}
	return out, true, nil
}

func hydrateUsers(userRepo LikedUserFetcher, ids []string) []domain.User {
	if len(ids) == 0 {
		return nil
	}
	users, err := userRepo.GetBatch(ids...)
	if err != nil || len(users) == 0 {
		// Fall back to a per-id loop so a single missing record doesn't
		// drop the whole list.
		users = make([]domain.User, 0, len(ids))
		for _, id := range ids {
			u, err := userRepo.Get(id)
			if err != nil {
				continue
			}
			users = append(users, u)
		}
	}
	return users
}

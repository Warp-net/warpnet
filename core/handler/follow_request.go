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
)

// FollowRequestStorer is the slice of FollowRequestsRepo the handlers need.
type FollowRequestStorer interface {
	Add(targetUserId, followerId string) error
	Remove(targetUserId, followerId string) error
	List(targetUserId string, limit *uint64, cursor *string) ([]string, string, error)
}

// FollowGranter promotes an authorized follow-request into a real follow.
// Implemented by the existing FollowRepo (Follow method).
type FollowGranter interface {
	Follow(followerId, followingId string) error
}

func StreamGetFollowRequestsHandler(repo FollowRequestStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetFollowRequestsEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("follow requests: empty user id")
		}
		ids, cur, err := repo.List(ev.UserId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		out := make([]domain.ID, 0, len(ids))
		for _, id := range ids {
			out = append(out, domain.ID(id))
		}
		return event.GetFollowRequestsResponse{FollowerIds: out, Cursor: cur}, nil
	}
}

func StreamAuthorizeFollowRequestHandler(
	repo FollowRequestStorer,
	follow FollowGranter,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.FollowRequestActionEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("authorize follow: empty user id")
		}
		if ev.FollowerId == "" {
			return nil, warpnet.WarpError("authorize follow: empty follower id")
		}
		// Promote the request into a real follow. The follower's identity
		// was vetted on the inbound follow handler; authorization only
		// flips the locked-account gate.
		if err := follow.Follow(ev.FollowerId, ev.UserId); err != nil {
			return nil, err
		}
		if err := repo.Remove(ev.UserId, ev.FollowerId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

func StreamRejectFollowRequestHandler(repo FollowRequestStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.FollowRequestActionEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("reject follow: empty user id")
		}
		if ev.FollowerId == "" {
			return nil, warpnet.WarpError("reject follow: empty follower id")
		}
		if err := repo.Remove(ev.UserId, ev.FollowerId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

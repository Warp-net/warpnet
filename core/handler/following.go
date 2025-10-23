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
	"strings"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type FollowNodeStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error)
}

type FollowingAuthStorer interface {
	GetOwner() domain.Owner
}

type FollowingUserStorer interface {
	Get(userId string) (user domain.User, err error)
	List(limit *uint64, cursor *string) ([]domain.User, string, error)
	Create(user domain.User) (domain.User, error)
}

type FollowingBroadcaster interface {
	SubscribeUserUpdate(userId string) (err error)
	UnsubscribeUserUpdate(userId string) (err error)
}

type FollowingStorer interface {
	Follow(fromUserId, toUserId string) error
	Unfollow(fromUserId, toUserId string) error
	GetFollowers(userId string, limit *uint64, cursor *string) ([]string, string, error)
	GetFollowings(userId string, limit *uint64, cursor *string) ([]string, string, error)
	IsFollowing(ownerId, otherUserId string) bool
	IsFollower(ownerId, otherUserId string) bool
}

func StreamFollowHandler(
	broadcaster FollowingBroadcaster,
	followRepo FollowingStorer,
	authRepo FollowingAuthStorer,
	userRepo FollowingUserStorer,
	streamer FollowNodeStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.NewFollowEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.FollowerId == "" || ev.FollowingId == "" {
			return nil, warpnet.WarpError("empty follower or following id")
		}
		if ev.FollowerId == ev.FollowingId {
			return event.Accepted, nil
		}

		ownerUserId := authRepo.GetOwner().UserId
		isMeFollowed := ownerUserId == ev.FollowingId

		if isMeFollowed {
			err := followRepo.Follow(ev.FollowerId, ownerUserId)
			if err != nil && !errors.Is(err, database.ErrAlreadyFollowed) {
				return nil, err
			}
			return event.Accepted, nil
		}

		// I follow someone
		err = followRepo.Follow(ownerUserId, ev.FollowingId)
		if errors.Is(err, database.ErrAlreadyFollowed) {
			return event.Accepted, nil
		}
		if err != nil {
			return nil, err
		}

		if err := broadcaster.SubscribeUserUpdate(ev.FollowingId); err != nil {
			return nil, err
		}

		followingUser, err := userRepo.Get(ev.FollowingId)
		if err != nil {
			return nil, err
		}

		// inform about me following someone now
		followDataResp, err := streamer.GenericStream(
			followingUser.NodeId,
			event.PUBLIC_POST_FOLLOW,
			event.NewFollowEvent{
				FollowingId: ev.FollowingId,
				FollowerId:  ev.FollowerId,
			},
		)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return nil, warpnet.ErrUserIsOffline
		}
		if err != nil {
			return nil, err
		}

		return event.Accepted, validateResponse(followDataResp)
	}
}

func StreamIsFollowingHandler(
	followRepo FollowingStorer,
	authRepo FollowingAuthStorer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetIsFollowingEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		ownerUserId := authRepo.GetOwner().UserId

		isFollowing := followRepo.IsFollowing(ownerUserId, ev.UserId)
		return event.IsFollowingResponse{IsFollowing: isFollowing}, nil
	}
}

func StreamIsFollowerHandler(
	followRepo FollowingStorer,
	authRepo FollowingAuthStorer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetIsFollowingEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		ownerUserId := authRepo.GetOwner().UserId

		isFollower := followRepo.IsFollower(ownerUserId, ev.UserId)
		return event.IsFollowerResponse{IsFollower: isFollower}, nil
	}
}

func StreamUnfollowHandler(
	broadcaster FollowingBroadcaster,
	followRepo FollowingStorer,
	authRepo FollowingAuthStorer,
	userRepo FollowingUserStorer,
	streamer FollowNodeStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.NewUnfollowEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.FollowerId == "" || ev.FollowingId == "" {
			return nil, warpnet.WarpError("empty follower or following id")
		}
		if ev.FollowerId == ev.FollowingId {
			return event.Accepted, nil
		}

		ownerUserId := authRepo.GetOwner().UserId
		isMeUnfollowed := ownerUserId == ev.FollowingId

		if isMeUnfollowed {
			err = followRepo.Unfollow(ev.FollowingId, ev.FollowerId)
			if err != nil {
				return nil, err
			}
			return event.Accepted, nil
		}

		err = followRepo.Unfollow(ownerUserId, ev.FollowingId)
		if err != nil {
			return nil, err
		}

		if err := broadcaster.UnsubscribeUserUpdate(ev.FollowingId); err != nil {
			log.Infoln("unfollow unsubscribe:", err)
		}

		followingUser, err := userRepo.Get(ev.FollowingId)
		if err != nil {
			return nil, err
		}

		unfollowDataResp, err := streamer.GenericStream(
			followingUser.NodeId,
			event.PUBLIC_POST_UNFOLLOW,
			event.NewUnfollowEvent{
				FollowingId: followingUser.Id,
				FollowerId:  ownerUserId,
			},
		)
		if err != nil && !errors.Is(err, warpnet.ErrNodeIsOffline) {
			return nil, err
		}

		return event.Accepted, validateResponse(unfollowDataResp)
	}
}

func validateResponse(resp []byte) error {
	if strings.Contains(string(resp), "Accepted") {
		return nil
	}

	var errorResp event.ErrorResponse
	err := json.Unmarshal(resp, &errorResp)
	if err != nil {
		return fmt.Errorf("followings: validate: unmarshal: %w", err)
	}
	if errorResp.Message != "" {
		return fmt.Errorf("followings: validate: message: %s", errorResp.Message)
	}
	return nil
}

func StreamGetFollowersHandler(
	authRepo FollowingAuthStorer,
	userRepo FollowingUserStorer,
	followRepo FollowingStorer,
	streamer FollowNodeStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetFollowersEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty user id")
		}
		owner := authRepo.GetOwner()
		if ev.UserId == owner.UserId {
			followers, cursor, err := followRepo.GetFollowers(ev.UserId, ev.Limit, ev.Cursor)
			if err != nil {
				return nil, err
			}

			return event.FollowersResponse{
				Cursor:      cursor,
				FollowingId: ev.UserId,
				Followers:   followers,
			}, nil
		}

		user, err := userRepo.Get(ev.UserId)
		if err != nil {
			return nil, err
		}
		followersData, err := streamer.GenericStream(user.NodeId, event.PUBLIC_GET_FOLLOWERS, buf)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			followers, cursor, err := followRepo.GetFollowers(ev.UserId, ev.Limit, ev.Cursor)
			if err != nil {
				return nil, err
			}

			return event.FollowersResponse{
				Cursor:      cursor,
				FollowingId: ev.UserId,
				Followers:   followers,
			}, nil
		}
		if err != nil {
			return nil, err
		}
		var possibleError event.ErrorResponse
		if _ = json.Unmarshal(followersData, &possibleError); possibleError.Message != "" {
			return nil, fmt.Errorf("unmarshal other followers error response: %s", possibleError.Message)
		}

		var followersResp event.FollowersResponse
		if err := json.Unmarshal(followersData, &followersResp); err != nil {
			return nil, err
		}
		return followersResp, nil
	}
}

func StreamGetFollowingsHandler(
	authRepo FollowingAuthStorer,
	userRepo FollowingUserStorer,
	followRepo FollowingStorer,
	streamer FollowNodeStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetFollowingsEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty user id")
		}
		owner := authRepo.GetOwner()
		if ev.UserId == owner.UserId {
			followings, cursor, err := followRepo.GetFollowings(ev.UserId, ev.Limit, ev.Cursor)
			if err != nil {
				return nil, err
			}

			return event.FollowingsResponse{
				Cursor:     cursor,
				FollowerId: ev.UserId,
				Followings: followings,
			}, nil
		}

		user, err := userRepo.Get(ev.UserId)
		if err != nil {
			return nil, err
		}
		followingsData, err := streamer.GenericStream(user.NodeId, event.PUBLIC_GET_FOLLOWINGS, buf)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			followings, cursor, err := followRepo.GetFollowings(ev.UserId, ev.Limit, ev.Cursor)
			if err != nil {
				return nil, err
			}

			return event.FollowingsResponse{
				Cursor:     cursor,
				FollowerId: ev.UserId,
				Followings: followings,
			}, nil
		}
		if err != nil {
			return nil, err
		}
		var possibleError event.ErrorResponse
		if _ = json.Unmarshal(followingsData, &possibleError); possibleError.Message != "" {
			return nil, fmt.Errorf("unmarshal other followings error response: %s", possibleError.Message)
		}

		var followingsResp event.FollowingsResponse
		if err := json.Unmarshal(followingsData, &followingsResp); err != nil {
			return nil, err
		}
		return followingsResp, nil
	}
}

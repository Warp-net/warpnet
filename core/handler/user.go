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

type UserStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

type UserTweetsCounter interface {
	TweetsCount(userID string) (uint64, error)
}

type UserFollowsCounter interface {
	GetFollowersCount(userId string) (uint64, error)
	GetFolloweesCount(userId string) (uint64, error)
	GetFollowers(userId string, limit *uint64, cursor *string) ([]domain.Following, string, error)
	GetFollowees(userId string, limit *uint64, cursor *string) ([]domain.Following, string, error)
}

type UserFetcher interface {
	Create(user domain.User) (domain.User, error)
	Get(userId string) (user domain.User, err error)
	List(limit *uint64, cursor *string) ([]domain.User, string, error)
	WhoToFollow(limit *uint64, cursor *string) ([]domain.User, string, error)
	Update(userId string, newUser domain.User) (updatedUser domain.User, err error)
	CreateWithTTL(user domain.User, ttl time.Duration) (domain.User, error)
}

type UserAuthStorer interface {
	GetOwner() domain.Owner
}

func StreamGetUserHandler(
	tweetRepo UserTweetsCounter,
	followRepo UserFollowsCounter,
	repo UserFetcher,
	authRepo UserAuthStorer,
	streamer UserStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetUserEvent
		err := json.JSON.Unmarshal(buf, &ev)
		if err != nil {
			return nil, fmt.Errorf("get user: event unmarshal: %v %s", err, buf)
		}

		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty user id")
		}

		ownerId := authRepo.GetOwner().UserId

		var u domain.User
		if ev.UserId == ownerId {
			u, err = repo.Get(ownerId)
			if err != nil {
				return nil, err
			}
			followersCount, err := followRepo.GetFollowersCount(u.Id)
			if err != nil {
				return nil, err
			}
			followeesCount, err := followRepo.GetFolloweesCount(u.Id)
			if err != nil {
				return nil, err
			}
			tweetsCount, err := tweetRepo.TweetsCount(u.Id)
			if err != nil {
				return nil, err
			}

			u.TweetsCount = tweetsCount
			u.FollowersCount = followersCount
			u.FolloweesCount = followeesCount

			return u, nil
		}

		otherUser, err := repo.Get(ev.UserId)
		if err != nil {
			return nil, err
		}

		otherUserData, err := streamer.GenericStream(
			otherUser.NodeId,
			event.PUBLIC_GET_USER,
			ev,
		)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			u, err = repo.Get(otherUser.Id)
			if err != nil {
				return nil, err
			}
			u.IsOffline = true
			return u, nil
		}
		if err != nil {
			return nil, err
		}

		var possibleError event.ErrorResponse
		if _ = json.JSON.Unmarshal(otherUserData, &possibleError); possibleError.Message != "" {
			return nil, fmt.Errorf("unmarshal other user error response: %s", possibleError.Message)
		}

		if err = json.JSON.Unmarshal(otherUserData, &u); err != nil {
			return nil, fmt.Errorf("get other user: response unmarshal: %v %s", err, otherUserData)
		}
		_, err = repo.Update(u.Id, u)
		return u, err
	}
}

func StreamGetUsersHandler(
	userRepo UserFetcher,
	streamer UserStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetAllUsersEvent
		err := json.JSON.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty user id")
		}

		users, cursor, err := userRepo.List(ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		if len(users) != 0 {
			go usersRefreshBackground(userRepo, ev, streamer)

			return event.UsersResponse{
				Cursor: cursor,
				Users:  users,
			}, err
		}

		usersRefreshBackground(userRepo, ev, streamer)

		users, cursor, _ = userRepo.List(ev.Limit, ev.Cursor)

		return event.UsersResponse{
			Cursor: cursor,
			Users:  users,
		}, nil
	}
}

func usersRefreshBackground(
	userRepo UserFetcher,
	ev event.GetAllUsersEvent,
	streamer UserStreamer,
) {
	if streamer.NodeInfo().OwnerId == ev.UserId {
		return
	}
	otherUser, err := userRepo.Get(ev.UserId)
	if err != nil {
		log.Errorf("get users handler: get user %v", err)
		return
	}

	usersDataResp, err := streamer.GenericStream(
		otherUser.NodeId,
		event.PUBLIC_GET_USERS,
		ev,
	)
	if err != nil {
		log.Errorf("get users handler: stream %v", err)
		return
	}

	var possibleError event.ErrorResponse
	if _ = json.JSON.Unmarshal(usersDataResp, &possibleError); possibleError.Message != "" {
		log.Errorf("unmarshal other users error response: %s", possibleError.Message)
		return
	}

	var usersResp event.UsersResponse
	if err := json.JSON.Unmarshal(usersDataResp, &usersResp); err != nil {
		log.Errorf("ummarshal users response:%v %s", err, usersDataResp)
		return
	}

	for _, user := range usersResp.Users {
		_, _ = userRepo.Create(user)
		_, _ = userRepo.Update(user.Id, user)
	}
}

func StreamGetWhoToFollowHandler(
	authRepo UserAuthStorer,
	userRepo UserFetcher,
	followRepo UserFollowsCounter,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetAllUsersEvent
		err := json.JSON.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty profile id")
		}

		owner := authRepo.GetOwner()

		profile, err := userRepo.Get(ev.UserId)
		if err != nil {
			log.Errorf("get who to follow handler: get user %v", err)
			profile = domain.User{
				Id:       owner.UserId,
				Username: owner.Username,
				Network:  warpnet.WarpnetName,
				NodeId:   owner.NodeId,
			}
		}

		users, cursor, err := userRepo.WhoToFollow(ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}

		followeesLimit := uint64(80) // TODO limit?
		followees, _, err := followRepo.GetFollowees(authRepo.GetOwner().UserId, &followeesLimit, nil)
		if err != nil {
			log.Errorf("get who to follow handler: get followers %v", err)
		}

		followedUsers := map[string]struct{}{}
		for _, follow := range followees {
			followedUsers[follow.Followee] = struct{}{}
		}

		whotofollow := make([]domain.User, 0, len(users))
		for _, user := range users {
			if user.Id == owner.UserId {
				continue
			}
			// if profile from Warpnet - don't show other network recommendations
			if profile.Id != owner.UserId && profile.Network != user.Network {
				continue
			}
			if _, ok := followedUsers[user.Id]; ok {
				continue
			}
			whotofollow = append(whotofollow, user)
		}

		return event.UsersResponse{
			Cursor: cursor,
			Users:  whotofollow,
		}, nil
	}
}

func StreamUpdateProfileHandler(authRepo UserAuthStorer, userRepo UserFetcher) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.NewUserEvent
		err := json.JSON.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		owner := authRepo.GetOwner()

		updatedUser, err := userRepo.Update(owner.UserId, ev)
		if err != nil {
			log.Errorln("failed to update user data", err)
			return nil, err
		}
		return updatedUser, nil
	}
}

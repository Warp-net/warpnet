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

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type LikeTweetsStorer interface {
	Get(userID, tweetID string) (tweet domain.Tweet, err error)
	List(string, *uint64, *string) ([]domain.Tweet, string, error)
	Create(_ string, tweet domain.Tweet) (domain.Tweet, error)
	Delete(userID, tweetID string) error
}

type LikedUserFetcher interface {
	GetBatch(userIds ...string) (users []domain.User, err error)
	Get(userId string) (users domain.User, err error)
}

type LikeStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

type LikesStorer interface {
	Like(tweetId, userId string) (likesNum uint64, err error)
	Unlike(tweetId, userId string) (likesNum uint64, err error)
	LikesCount(tweetId string) (likesNum uint64, err error)
	Likers(tweetId string, limit *uint64, cursor *string) (likers []string, cur string, err error)
}

func StreamLikeHandler(
	repo LikesStorer,
	userRepo LikedUserFetcher,
	streamer LikeStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.LikeEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.OwnerId == "" {
			return nil, warpnet.WarpError("like: empty owner id")
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("like: empty user id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("like: empty tweet id")
		}

		tweetId := strings.TrimPrefix(ev.TweetId, domain.RetweetPrefix)
		num, err := repo.Like(tweetId, ev.OwnerId) // store my like
		if err != nil {
			return nil, err
		}

		isOwnTweetLike := ev.OwnerId == ev.UserId
		if isOwnTweetLike { // own tweet like
			return event.LikesCountResponse{num}, nil
		}

		isSomeoneLiked := ev.OwnerId != streamer.NodeInfo().OwnerId
		if isSomeoneLiked { // likes exchange finished
			return event.LikesCountResponse{num}, nil
		}

		likedUser, err := userRepo.Get(ev.UserId)
		if errors.Is(err, database.ErrUserNotFound) {
			return event.LikesCountResponse{num}, nil
		}
		if err != nil {
			return nil, err
		}

		likeDataResp, err := streamer.GenericStream(
			likedUser.NodeId,
			event.PUBLIC_POST_LIKE,
			event.LikeEvent{
				TweetId: ev.TweetId,
				OwnerId: ev.OwnerId,
				UserId:  ev.UserId,
			},
		)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return event.LikesCountResponse{num}, nil
		}
		if err != nil {
			return nil, err
		}

		var possibleError event.ErrorResponse
		if _ = json.Unmarshal(likeDataResp, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other like error response: %s", possibleError.Message)
		}

		return event.LikesCountResponse{num}, nil
	}
}

func StreamUnlikeHandler(repo LikesStorer, userRepo LikedUserFetcher, streamer LikeStreamer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UnlikeEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}

		if ev.UserId == "" {
			return nil, warpnet.WarpError("empty user id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("empty tweet id")
		}

		tweetId := strings.TrimPrefix(ev.TweetId, domain.RetweetPrefix)
		num, err := repo.Unlike(tweetId, ev.OwnerId)
		if err != nil {
			return nil, err
		}

		isOwnTweetDislike := ev.OwnerId == ev.UserId
		if isOwnTweetDislike { // own tweet like
			return event.LikesCountResponse{num}, nil
		}

		isSomeoneDisliked := ev.OwnerId != streamer.NodeInfo().OwnerId
		if isSomeoneDisliked { // likes exchange finished
			return event.LikesCountResponse{num}, nil
		}

		unlikedUser, err := userRepo.Get(ev.UserId)
		if errors.Is(err, database.ErrUserNotFound) {
			return event.LikesCountResponse{num}, nil
		}
		if err != nil {
			return nil, err
		}

		unlikeDataResp, err := streamer.GenericStream(
			unlikedUser.NodeId,
			event.PUBLIC_POST_UNLIKE,
			event.UnlikeEvent{
				TweetId: ev.TweetId,
				UserId:  ev.UserId,
				OwnerId: ev.OwnerId,
			},
		)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return event.LikesCountResponse{num}, nil
		}
		if err != nil {
			return nil, err
		}

		var possibleError event.ErrorResponse
		if _ = json.Unmarshal(unlikeDataResp, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other unlike error response: %s", possibleError.Message)
		}

		return event.LikesCountResponse{num}, nil
	}
}

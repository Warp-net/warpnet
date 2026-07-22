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
	Like(tweetId, userId string, isTransitive bool) (likesNum uint64, err error)
	Unlike(tweetId, userId string, isTransitive bool) (likesNum uint64, err error)
	LikesCount(tweetId string) (likesNum uint64, err error)
	Likers(tweetId string, limit *uint64, cursor *string) (likers []string, cur string, err error)
	SetLiked(userId, tweetId, ownerUserId string) error
	RemoveLiked(userId, tweetId string) error
}

func StreamLikeHandler(
	repo LikesStorer,
	userRepo LikedUserFetcher,
	notifyRepo ModerationNotifier,
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
		ownNodeInfo := streamer.NodeInfo()
		// The network-wide (CRDT) like counter is bumped only on the liker's
		// own node, so a like observed on both the liker's and the author's
		// node is counted once.
		num, err := repo.Like(tweetId, ev.OwnerId, ev.OwnerId == ownNodeInfo.OwnerId) // store my like
		if err != nil {
			log.Errorf("like handler failed: %v", err)
			return nil, err
		}
		// Best-effort "tweets I liked" index; the like itself already
		// succeeded, so an index failure must not fail the request.
		if err := repo.SetLiked(ev.OwnerId, tweetId, ev.UserId); err != nil {
			log.Warnf("like handler: liked index: %v", err)
		}

		isOwnTweetLike := ev.OwnerId == ev.UserId
		if isOwnTweetLike { // own tweet like
			return event.LikesCountResponse{Count: num}, nil
		}

		isSomeoneLikedMe := ev.OwnerId != ownNodeInfo.OwnerId
		if isSomeoneLikedMe { // likes exchange finished
			notifyUsername := ev.OwnerId
			liker, likerErr := userRepo.Get(ev.OwnerId)
			if likerErr == nil {
				notifyUsername = liker.Username
			}
			if err := notifyRepo.Add(domain.Notification{
				Type:    domain.NotificationLikeType,
				Text:    notifyUsername + " liked your tweet",
				UserId:  ev.UserId,
				ActorId: ev.OwnerId,
			}); err != nil {
				log.Errorf("like handler: adding notification: %v", err)
			}
			return event.LikesCountResponse{Count: num}, nil
		}

		likedUser, err := userRepo.Get(ev.UserId)
		if errors.Is(err, database.ErrUserNotFound) {
			return event.LikesCountResponse{Count: num}, nil
		}
		if err != nil {
			return nil, err
		}

		if likedUser.NodeId == ownNodeInfo.ID.String() {
			return event.LikesCountResponse{Count: num}, nil
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
			return event.LikesCountResponse{Count: num}, nil
		}
		if err != nil {
			return nil, err
		}

		var possibleError event.ResponseError
		if _ = json.Unmarshal(likeDataResp, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other like error response: %s", possibleError.Message)
		}

		return event.LikesCountResponse{Count: num}, nil
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
		ownNodeInfo := streamer.NodeInfo()
		// Mirror the like path: only the unliker's own node adjusts the
		// network-wide (CRDT) counter.
		num, err := repo.Unlike(tweetId, ev.OwnerId, ev.OwnerId == ownNodeInfo.OwnerId)
		if err != nil {
			log.Errorf("unlike handler failed: %v", err)
			return nil, err
		}
		if err := repo.RemoveLiked(ev.OwnerId, tweetId); err != nil {
			log.Warnf("unlike handler: liked index: %v", err)
		}

		isOwnTweetDislike := ev.OwnerId == ev.UserId
		if isOwnTweetDislike { // own tweet like
			return event.LikesCountResponse{Count: num}, nil
		}

		isSomeoneDislikedMe := ev.OwnerId != ownNodeInfo.OwnerId
		if isSomeoneDislikedMe { // likes exchange finished
			return event.LikesCountResponse{Count: num}, nil
		}

		unlikedUser, err := userRepo.Get(ev.UserId)
		if errors.Is(err, database.ErrUserNotFound) {
			return event.LikesCountResponse{Count: num}, nil
		}
		if err != nil {
			return nil, err
		}

		if unlikedUser.NodeId == ownNodeInfo.ID.String() {
			return event.LikesCountResponse{Count: num}, nil
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
			return event.LikesCountResponse{Count: num}, nil
		}
		if err != nil {
			return nil, err
		}

		var possibleError event.ResponseError
		if _ = json.Unmarshal(unlikeDataResp, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other unlike error response: %s", possibleError.Message)
		}

		return event.LikesCountResponse{Count: num}, nil
	}
}

type LikedTweetsLister interface {
	Liked(userId string, limit *uint64, cursor *string) ([]domain.LikedTweet, string, error)
}

// StreamGetLikesHandler returns one page of the local user's "tweets I
// liked" index, newest first. Same reference-only wire shape as bookmarks:
// clients hydrate each tweet via PUBLIC_GET_TWEET using OwnerUserId.
func StreamGetLikesHandler(repo LikedTweetsLister) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetLikesEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("likes: empty user id")
		}

		liked, cur, err := repo.Liked(ev.UserId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		items := make([]event.BookmarkItem, 0, len(liked))
		for _, lt := range liked {
			items = append(items, event.BookmarkItem{
				UserId:      lt.UserId,
				TweetId:     lt.TweetId,
				OwnerUserId: lt.OwnerUserId,
				CreatedAt:   lt.CreatedAt,
			})
		}
		return event.GetLikesResponse{Items: items, Cursor: cur}, nil
	}
}

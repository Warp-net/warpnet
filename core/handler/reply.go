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

type ReplyTweetStorer interface {
	Get(userID, tweetID string) (tweet domain.Tweet, err error)
}

type ReplyStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

type ReplyUserFetcher interface {
	Get(userId string) (user domain.User, err error)
}

type ReplyStorer interface {
	GetReply(rootID, replyID string) (tweet domain.Tweet, err error)
	GetRepliesTree(rootID, parentId string, limit *uint64, cursor *string) ([]domain.ReplyNode, string, error)
	AddReply(reply domain.Tweet) (domain.Tweet, error)
	DeleteReply(rootID, parentID, replyID string) error
}

func StreamNewReplyHandler(
	replyRepo ReplyStorer,
	userRepo ReplyUserFetcher,
	notifyRepo ModerationNotifier,
	streamer ReplyStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.NewReplyEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.Text == "" {
			return nil, warpnet.WarpError("empty reply body")
		}
		if ev.ParentId == nil {
			return nil, warpnet.WarpError("empty parent ID")
		}

		rootId := strings.TrimPrefix(ev.RootId, domain.RetweetPrefix)
		parentId := strings.TrimPrefix(*ev.ParentId, domain.RetweetPrefix)

		reply, err := replyRepo.AddReply(domain.Tweet{
			CreatedAt: ev.CreatedAt,
			Id:        ev.Id,
			ParentId:  &parentId,
			RootId:    rootId,
			Text:      ev.Text,
			UserId:    ev.UserId,
			Username:  ev.Username,
		})
		if err != nil {
			log.Errorf("reply handler failed: %v", err)
			return nil, err
		}

		parentUser, err := userRepo.Get(ev.ParentUserId)
		if errors.Is(err, database.ErrUserNotFound) {
			return reply, nil
		}
		if err != nil {
			return nil, err
		}

		ownNodeInfo := streamer.NodeInfo()
		isOwnTweetReply := ev.ParentUserId == ownNodeInfo.OwnerId
		if isOwnTweetReply {
			if ev.UserId != ownNodeInfo.OwnerId {
				if err := notifyRepo.Add(domain.Notification{
					Type:   domain.NotificationReplyType,
					Text:   ev.Username + " replied to your tweet",
					UserId: ev.ParentUserId,
				}); err != nil {
					log.Errorf("reply handler: adding notification: %v", err)
				}
			}
			return reply, nil
		}
		if ownNodeInfo.ID.String() == parentUser.NodeId {
			return reply, nil
		}

		replyDataResp, err := streamer.GenericStream(
			parentUser.NodeId,
			event.PUBLIC_POST_REPLY,
			event.NewReplyEvent{
				CreatedAt:    reply.CreatedAt,
				Id:           reply.Id,
				ParentId:     ev.ParentId,
				ParentUserId: ev.ParentUserId,
				RootId:       ev.RootId,
				Text:         ev.Text,
				UserId:       ev.UserId,
				Username:     ev.Username,
			},
		)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return reply, nil
		}
		if err != nil {
			return nil, err
		}

		var possibleError event.ResponseError
		if _ = json.Unmarshal(replyDataResp, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other reply error response: %s", possibleError.Message)
		}

		return reply, nil
	}
}

func StreamGetReplyHandler(
	repo ReplyStorer,
	authRepo OwnerTweetStorer,
	userRepo TweetUserFetcher,
	streamer TweetStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetReplyEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.RootId == "" {
			return nil, warpnet.WarpError("empty root id")
		}
		if ev.ReplyId == "" {
			return nil, warpnet.WarpError("empty reply id")
		}

		rootId := strings.TrimPrefix(ev.RootId, domain.RetweetPrefix)
		id := strings.TrimPrefix(ev.ReplyId, domain.RetweetPrefix)

		owner := authRepo.GetOwner()

		isMyOwnReply := ev.UserId == owner.UserId
		if isMyOwnReply {
			return repo.GetReply(rootId, id)
		}

		otherUser, err := userRepo.Get(ev.UserId)
		if errors.Is(err, database.ErrUserNotFound) {
			return repo.GetReply(rootId, id)
		}
		if err != nil {
			return nil, err
		}

		if owner.NodeId == otherUser.NodeId {
			return repo.GetReply(rootId, id)
		}

		getTweetResp, err := streamer.GenericStream(
			otherUser.NodeId,
			event.PUBLIC_GET_REPLY,
			ev,
		)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return repo.GetReply(rootId, id)
		}
		if err != nil {
			return nil, err
		}

		var possibleError event.ResponseError
		if _ = json.Unmarshal(getTweetResp, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other get reply error response: %s", possibleError.Message)
			return repo.GetReply(rootId, id)
		}

		var reply domain.Tweet
		if err = json.Unmarshal(getTweetResp, &reply); err != nil {
			return repo.GetReply(rootId, id)
		}

		return reply, nil
	}
}

func StreamDeleteReplyHandler(
	tweetRepo ReplyTweetStorer,
	userRepo ReplyUserFetcher,
	replyRepo ReplyStorer,
	streamer ReplyStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.DeleteReplyEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.ReplyId == "" {
			return nil, warpnet.WarpError("empty reply id")
		}
		if ev.RootId == "" {
			return nil, warpnet.WarpError("empty root id")
		}

		rootId := strings.TrimPrefix(ev.RootId, domain.RetweetPrefix)

		reply, err := replyRepo.GetReply(rootId, ev.ReplyId)
		if err != nil {
			return nil, err
		}

		var parentTweet domain.Tweet
		if reply.ParentId == nil {
			parentTweet, err = tweetRepo.Get(reply.UserId, rootId)
			if err != nil {
				return nil, err
			}
		} else {
			parentId := strings.TrimPrefix(*reply.ParentId, domain.RetweetPrefix)
			parentTweet, err = replyRepo.GetReply(rootId, parentId)
			if err != nil {
				return nil, err
			}
		}

		if err = replyRepo.DeleteReply(rootId, parentTweet.Id, ev.ReplyId); err != nil {
			log.Errorf("delete reply handler failed: %v", err)
			return nil, err
		}

		ownNodeInfo := streamer.NodeInfo()
		parentUser, err := userRepo.Get(parentTweet.UserId)
		if errors.Is(err, database.ErrUserNotFound) {
			return event.Accepted, nil
		}
		if err != nil {
			return nil, err
		}

		if ownNodeInfo.ID.String() == parentUser.NodeId {
			return event.Accepted, nil
		}

		replyDataResp, err := streamer.GenericStream(
			parentUser.NodeId,
			event.PUBLIC_DELETE_REPLY,
			event.DeleteReplyEvent{
				ReplyId: ev.ReplyId,
				RootId:  rootId,
				UserId:  ev.UserId,
			},
		)
		if errors.Is(err, warpnet.ErrNodeIsOffline) {
			return event.Accepted, nil
		}
		if err != nil {
			return nil, err
		}

		var possibleError event.ResponseError
		if _ = json.Unmarshal(replyDataResp, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other delete reply error response: %s", possibleError.Message)
		}

		return event.Accepted, nil
	}
}

// StreamGetRepliesHandler answers /public/get/replies requests.
//
// ev.RootId is the root tweet of the thread; ev.ParentId is the parent
// TWEET id selecting which subtree of replies to return (NOT a user id —
// it gets compared against tweet/reply ids in the repo). Clients send
// an empty ParentId for "give me the top-level replies of the thread",
// which we normalise to RootId so the repo lookup matches the first
// tier of replies. Replies are served straight from the local store:
// any reply we know about (because the author's node pushed it to us
// via gossip, or because we cached an earlier fetch) is returned;
// otherwise the response is empty.
//
// Note on routing: this used to try to forward the request to the
// "parent user" by treating ParentId as a user id and looking it up in
// userRepo. That can't work — ParentId is a tweet id — so the lookup
// always returned ErrUserNotFound and we silently fell back to local
// storage anyway. The dead code is removed; proper remote-fetch
// routing would need a RootUserId in GetAllRepliesEvent to identify the
// author of the root tweet, which clients don't currently send.
func StreamGetRepliesHandler(repo ReplyStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetAllRepliesEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.RootId == "" {
			return nil, warpnet.WarpError("empty root id")
		}
		// Top-level replies on a thread have no parent — clients send an
		// empty parent_id in that case. Treat it as the root itself so
		// the repo returns the first-tier replies hanging off RootId.
		if ev.ParentId == "" {
			ev.ParentId = ev.RootId
		}

		rootId := strings.TrimPrefix(ev.RootId, domain.RetweetPrefix)
		parentId := strings.TrimPrefix(ev.ParentId, domain.RetweetPrefix)

		replies, cursor, err := repo.GetRepliesTree(rootId, parentId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		return event.RepliesResponse{
			Cursor:  cursor,
			Replies: replies,
			UserId:  &parentId,
		}, nil
	}
}

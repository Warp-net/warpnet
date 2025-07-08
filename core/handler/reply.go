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
	"strings"
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

		parentUser, err := userRepo.Get(ev.ParentUserId)
		if err != nil {
			return nil, err
		}

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
			return nil, err
		}

		if parentUser.NodeId == streamer.NodeInfo().ID.String() {
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
		if err != nil && !errors.Is(err, warpnet.ErrNodeIsOffline) {
			return nil, err
		}

		var possibleError event.ErrorResponse
		if _ = json.Unmarshal(replyDataResp, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other reply error response: %s", possibleError.Message)
		}

		return reply, nil
	}
}

func StreamGetReplyHandler(repo ReplyStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetReplyEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.RootId == "" {
			return nil, warpnet.WarpError("empty root id")
		}

		rootId := strings.TrimPrefix(ev.RootId, domain.RetweetPrefix)
		id := strings.TrimPrefix(ev.ReplyId, domain.RetweetPrefix)

		return repo.GetReply(rootId, id)
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

		parentUser, err := userRepo.Get(parentTweet.UserId)
		if err != nil {
			return nil, err
		}

		if err = replyRepo.DeleteReply(rootId, parentTweet.Id, ev.ReplyId); err != nil {
			return nil, err
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
		if err != nil && !errors.Is(err, warpnet.ErrNodeIsOffline) {
			return nil, err
		}

		var possibleError event.ErrorResponse
		if _ = json.Unmarshal(replyDataResp, &possibleError); possibleError.Message != "" {
			log.Errorf("unmarshal other delete reply error response: %s", possibleError.Message)
		}

		return event.Accepted, nil
	}
}

func StreamGetRepliesHandler(
	repo ReplyStorer,
	userRepo ReplyUserFetcher,
	streamer ReplyStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetAllRepliesEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.ParentId == "" {
			return nil, warpnet.WarpError("empty parent id")
		}
		if ev.RootId == "" {
			return nil, warpnet.WarpError("empty root id")
		}

		rootId := strings.TrimPrefix(ev.RootId, domain.RetweetPrefix)
		parentId := strings.TrimPrefix(ev.ParentId, domain.RetweetPrefix)

		if parentId == streamer.NodeInfo().OwnerId {
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

		parentUser, err := userRepo.Get(parentId)
		if err != nil {
			return nil, err
		}

		replyDataResp, err := streamer.GenericStream(
			parentUser.NodeId,
			event.PUBLIC_GET_REPLIES,
			ev,
		)
		if err != nil && !errors.Is(err, warpnet.ErrNodeIsOffline) {
			return nil, err
		}

		var possibleError event.ErrorResponse
		if _ = json.Unmarshal(replyDataResp, &possibleError); possibleError.Message != "" {
			return nil, fmt.Errorf("unmarshal other delete reply error response: %s", possibleError.Message)
		}

		var repliesResp event.RepliesResponse
		if err := json.Unmarshal(replyDataResp, &repliesResp); err != nil {
			return nil, err
		}
		for _, reply := range repliesResp.Replies {
			if _, err := repo.AddReply(reply.Reply); err != nil {
				log.Errorf("failed to add reply to replies repo: %v", err)
			}
		}
		return repliesResp, nil
	}
}

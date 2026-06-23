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
	"strings"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

// Replies are tweets (domain.Tweet with ParentId set): they are created,
// fetched and deleted through the tweet handlers. The only reply-specific
// read left is the thread tree, served below.

type RepliesTreeStorer interface {
	GetRepliesTree(rootID, parentId string, limit *uint64, cursor *string) ([]domain.ReplyNode, string, error)
}

type ReplyUserFetcher interface {
	Get(userId string) (user domain.User, err error)
}

type ReplyStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

// forwardReplies asks the root tweet author's home node for the thread's
// replies when that node is not this one. ok=false means handle locally.
func forwardReplies(userRepo ReplyUserFetcher, streamer ReplyStreamer, ev event.GetAllRepliesEvent) (event.RepliesResponse, bool) {
	if ev.RootUserId == "" {
		return event.RepliesResponse{}, false
	}
	author, err := userRepo.Get(string(ev.RootUserId))
	if err != nil || author.NodeId == "" || author.NodeId == streamer.NodeInfo().ID.String() {
		return event.RepliesResponse{}, false
	}
	data, err := streamer.GenericStream(author.NodeId, event.PUBLIC_GET_REPLIES, ev)
	if err != nil {
		log.Errorf("get replies: forward to %s: %v", author.NodeId, err)
		return event.RepliesResponse{}, false
	}
	var resp event.RepliesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return event.RepliesResponse{}, false
	}
	return resp, true
}

func StreamGetRepliesHandler(repo RepliesTreeStorer, userRepo ReplyUserFetcher, streamer ReplyStreamer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetAllRepliesEvent
		err := json.Unmarshal(buf, &ev)
		if err != nil {
			return nil, err
		}
		if ev.RootId == "" {
			return nil, warpnet.WarpError("empty root id")
		}

		// Empty parent_id means top-level replies: treat it as the root.
		if ev.ParentId == "" {
			ev.ParentId = ev.RootId
		}

		rootId := strings.TrimPrefix(ev.RootId, domain.RetweetPrefix)
		parentId := strings.TrimPrefix(ev.ParentId, domain.RetweetPrefix)

		replies, cursor, err := repo.GetRepliesTree(rootId, parentId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		// Nothing locally: try the root author's home node.
		if len(replies) == 0 {
			if resp, ok := forwardReplies(userRepo, streamer, ev); ok {
				return resp, nil
			}
		}
		return event.RepliesResponse{
			Cursor:  cursor,
			Replies: replies,
			UserId:  &parentId,
		}, nil
	}
}

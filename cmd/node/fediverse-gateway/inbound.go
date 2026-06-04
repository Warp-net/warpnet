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

package main

import (
	"strings"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/event"
	stripper "github.com/grokify/html-strip-tags-go"
)

const (
	keyType   = "type"
	keyObject = "object"
	keyActor  = "actor"
)

// translateInbound maps a verified inbound ActivityPub activity to the Warpnet
// route + event the gateway should send to the owner's node, reusing Warpnet's
// existing handlers. Remote actors travel as ap:-prefixed base64url ids (the
// follower scheme); the owner and tweet are recovered from our own URLs.
// Announce (boost) and Delete are not handled yet.
func (g *gateway) translateInbound(raw map[string]any) (stream.WarpRoute, any, bool) {
	actor, _ := raw[keyActor].(string)
	if actor == "" {
		return "", nil, false
	}

	switch raw[keyType] {
	case "Like":
		owner, tweetID, ok := g.parseLocalStatus(stringField(raw, keyObject))
		if !ok {
			return "", nil, false
		}
		return event.PUBLIC_POST_LIKE, event.LikeEvent{
			TweetId: tweetID, UserId: encodeActorID(actor), OwnerId: owner,
		}, true

	case "Create":
		obj, _ := raw[keyObject].(map[string]any)
		if obj == nil {
			return "", nil, false
		}
		owner, parentID, ok := g.parseLocalStatus(stringField(obj, "inReplyTo"))
		if !ok {
			return "", nil, false
		}
		pid := parentID
		return event.PUBLIC_POST_REPLY, event.NewReplyEvent{
			CreatedAt:    time.Now(),
			Id:           randomToken(),
			ParentId:     &pid,
			ParentUserId: owner,
			RootId:       parentID,
			Text:         stripper.StripTags(stringField(obj, "content")),
			UserId:       encodeActorID(actor),
			Username:     actor,
		}, true

	case "Undo":
		obj, _ := raw[keyObject].(map[string]any)
		if obj == nil {
			return "", nil, false
		}
		switch obj[keyType] {
		case "Follow":
			owner := userFromActorURL(stringField(obj, keyObject))
			if owner == "" {
				return "", nil, false
			}
			return event.PUBLIC_POST_UNFOLLOW, event.NewUnfollowEvent{
				FollowerId: encodeActorID(actor), FollowingId: owner,
			}, true
		case "Like":
			owner, tweetID, ok := g.parseLocalStatus(stringField(obj, keyObject))
			if !ok {
				return "", nil, false
			}
			return event.PUBLIC_POST_UNLIKE, event.UnlikeEvent{
				TweetId: tweetID, UserId: encodeActorID(actor), OwnerId: owner,
			}, true
		}
	}
	return "", nil, false
}

// parseLocalStatus extracts the owner username and tweet id from one of our own
// status URLs (https://host/users/{user}/statuses/{id}).
func (g *gateway) parseLocalStatus(statusURL string) (owner, tweetID string, ok bool) {
	rest, found := strings.CutPrefix(statusURL, g.baseURL()+pathUsers)
	if !found {
		return "", "", false
	}
	owner, after, found := strings.Cut(rest, pathStatuses)
	if !found || owner == "" || after == "" {
		return "", "", false
	}
	tweetID = after
	if i := strings.IndexByte(tweetID, '/'); i >= 0 {
		tweetID = tweetID[:i]
	}
	return owner, tweetID, true
}

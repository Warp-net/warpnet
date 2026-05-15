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
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"

	"errors"
)

// UserSetStorer is the narrow surface block / mute handlers need from the
// owner-keyed user-id set repo (database/userset-repo.go).
type UserSetStorer interface {
	Add(ownerId, targetId string) error
	Remove(ownerId, targetId string) error
	List(ownerId string, limit *uint64, cursor *string) ([]string, string, error)
}

// BlockUserResolver looks up the target user so the handler can escalate
// the social block into a network-layer peer blocklist.
type BlockUserResolver interface {
	Get(userId string) (user domain.User, err error)
}

// PeerBlocklister talks to NodeRepo's libp2p-level blocklist (database/node-repo.go)
// so that blocking a user disconnects and refuses traffic from their node.
type PeerBlocklister interface {
	Blocklist(peerId string) error
}

func StreamBlockHandler(repo UserSetStorer, userRepo BlockUserResolver, nodeRepo PeerBlocklister) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.BlockEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.BlockerId == "" {
			return nil, warpnet.WarpError("block: empty blocker id")
		}
		if ev.BlockeeId == "" {
			return nil, warpnet.WarpError("block: empty blockee id")
		}
		if ev.BlockerId == ev.BlockeeId {
			return nil, warpnet.WarpError("block: cannot block yourself")
		}
		if err := repo.Add(ev.BlockerId, ev.BlockeeId); err != nil {
			return nil, err
		}

		// Escalate the social block to a peer-level blocklist so libp2p
		// refuses traffic from the blockee's node. Best-effort: if the
		// blockee's user record is missing or has no NodeId yet (e.g. they
		// are a discovered-but-unresolved peer), the social block still
		// stands.
		if userRepo != nil && nodeRepo != nil {
			u, uerr := userRepo.Get(ev.BlockeeId)
			switch {
			case errors.Is(uerr, database.ErrUserNotFound):
				log.Warnf("block: target user %s not yet known locally, peer block skipped", ev.BlockeeId)
			case uerr != nil:
				log.Warnf("block: lookup user %s: %v", ev.BlockeeId, uerr)
			case u.NodeId == "":
				log.Warnf("block: target user %s has no node id, peer block skipped", ev.BlockeeId)
			default:
				if err := nodeRepo.Blocklist(u.NodeId); err != nil {
					log.Warnf("block: peer blocklist for %s (%s): %v", ev.BlockeeId, u.NodeId, err)
				}
			}
		}
		return event.Accepted, nil
	}
}

func StreamUnblockHandler(repo UserSetStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UnblockEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.BlockerId == "" {
			return nil, warpnet.WarpError("unblock: empty blocker id")
		}
		if ev.BlockeeId == "" {
			return nil, warpnet.WarpError("unblock: empty blockee id")
		}
		if err := repo.Remove(ev.BlockerId, ev.BlockeeId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

func StreamGetBlocksHandler(repo UserSetStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetBlocksEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("blocks: empty user id")
		}
		ids, cur, err := repo.List(ev.UserId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		out := make([]domain.ID, 0, len(ids))
		for _, id := range ids {
			out = append(out, domain.ID(id))
		}
		return event.GetBlocksResponse{Ids: out, Cursor: cur}, nil
	}
}

func StreamMuteHandler(repo UserSetStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.MuteEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.MuterId == "" {
			return nil, warpnet.WarpError("mute: empty muter id")
		}
		if ev.MuteeId == "" {
			return nil, warpnet.WarpError("mute: empty mutee id")
		}
		if ev.MuterId == ev.MuteeId {
			return nil, warpnet.WarpError("mute: cannot mute yourself")
		}
		if err := repo.Add(ev.MuterId, ev.MuteeId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

func StreamUnmuteHandler(repo UserSetStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UnmuteEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.MuterId == "" {
			return nil, warpnet.WarpError("unmute: empty muter id")
		}
		if ev.MuteeId == "" {
			return nil, warpnet.WarpError("unmute: empty mutee id")
		}
		if err := repo.Remove(ev.MuterId, ev.MuteeId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

func StreamGetMutesHandler(repo UserSetStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetMutesEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("mutes: empty user id")
		}
		ids, cur, err := repo.List(ev.UserId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		out := make([]domain.ID, 0, len(ids))
		for _, id := range ids {
			out = append(out, domain.ID(id))
		}
		return event.GetMutesResponse{Ids: out, Cursor: cur}, nil
	}
}

type ConvMuteStorer interface {
	Mute(userId, tweetId string) error
	Unmute(userId, tweetId string) error
}

func StreamMuteConversationHandler(repo ConvMuteStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.MuteConversationEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("mute conversation: empty user id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("mute conversation: empty tweet id")
		}
		if err := repo.Mute(ev.UserId, ev.TweetId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

func StreamUnmuteConversationHandler(repo ConvMuteStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UnmuteConversationEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("unmute conversation: empty user id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("unmute conversation: empty tweet id")
		}
		if err := repo.Unmute(ev.UserId, ev.TweetId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

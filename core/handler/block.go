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

	"github.com/Warp-net/warpnet/core/fediverse"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

// BlocksStorer is the narrow surface block handlers need from BlocksRepo.
type BlocksStorer interface {
	Block(blockerId, blockeeId string) error
	Unblock(blockerId, blockeeId string) error
	List(blockerId string, limit *uint64, cursor *string) ([]string, string, error)
}

// MutesStorer is the narrow surface mute handlers need from MutesRepo.
type MutesStorer interface {
	Mute(muterId, muteeId string) error
	Unmute(muterId, muteeId string) error
	List(muterId string, limit *uint64, cursor *string) ([]string, string, error)
}

// BlockUserFetcher looks up the blockee so the handler can escalate the
// social block into a network-layer peer blocklist.
type BlockUserFetcher interface {
	Get(userId string) (user domain.User, err error)
}

// PeerBlocklister talks to NodeRepo's libp2p-level blocklist
// (database/node-repo.go) so blocking a user disconnects and refuses
// traffic from their node.
//
// A social block is permanent until the user undoes it, so we go
// straight to BlocklistPermanent (no exponential escalation) on
// Block, and BlocklistRemove on Unblock.
type PeerBlocklister interface {
	BlocklistPermanent(peerId string) error
	BlocklistRemove(peerId string) error
}

type BlockFollowStorer interface {
	IsFollowing(ownerId, otherUserId string) bool
	IsFollower(ownerId, otherUserId string) bool
	Unfollow(fromUserId, toUserId string) error
}

type BlockUnsubscriber interface {
	UnsubscribeUserUpdate(userId string) error
}

type BlockTweetDeleter interface {
	List(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error)
	Delete(userID, tweetID string) error
}

type BlockTimelineDeleter interface {
	DeleteTweetFromTimeline(userID, tweetID string) error
}

type BlockMediaDeleter interface {
	DeleteUserMedia(userId string) error
}

func StreamBlockHandler(
	repo BlocksStorer,
	userRepo BlockUserFetcher,
	nodeRepo PeerBlocklister,
	followRepo BlockFollowStorer,
	broadcaster BlockUnsubscriber,
	tweetRepo BlockTweetDeleter,
	timelineRepo BlockTimelineDeleter,
	mediaRepo BlockMediaDeleter,
) warpnet.WarpHandlerFunc {
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
		if err := repo.Block(ev.BlockerId, ev.BlockeeId); err != nil {
			return nil, err
		}

		escalateToPeerBlocklist(userRepo, nodeRepo, ev.BlockeeId)

		if err := unfollowBlockee(followRepo, broadcaster, ev.BlockerId, ev.BlockeeId); err != nil {
			return nil, fmt.Errorf("block: unfollow %s: %w", ev.BlockeeId, err)
		}
		if err := deleteBlockeeTweets(tweetRepo, timelineRepo, ev.BlockerId, ev.BlockeeId); err != nil {
			return nil, fmt.Errorf("block: delete tweets of %s: %w", ev.BlockeeId, err)
		}
		if err := mediaRepo.DeleteUserMedia(ev.BlockeeId); err != nil {
			return nil, fmt.Errorf("block: delete media of %s: %w", ev.BlockeeId, err)
		}
		return event.Accepted, nil
	}
}

func StreamUnblockHandler(repo BlocksStorer, userRepo BlockUserFetcher, nodeRepo PeerBlocklister) warpnet.WarpHandlerFunc {
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
		if err := repo.Unblock(ev.BlockerId, ev.BlockeeId); err != nil {
			return nil, err
		}

		removePeerBlocklist(userRepo, nodeRepo, ev.BlockeeId)
		return event.Accepted, nil
	}
}

func StreamGetBlocksHandler(repo BlocksStorer) warpnet.WarpHandlerFunc {
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

func StreamMuteHandler(repo MutesStorer) warpnet.WarpHandlerFunc {
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
		if err := repo.Mute(ev.MuterId, ev.MuteeId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

func StreamUnmuteHandler(repo MutesStorer) warpnet.WarpHandlerFunc {
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
		if err := repo.Unmute(ev.MuterId, ev.MuteeId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

func StreamGetMutesHandler(repo MutesStorer) warpnet.WarpHandlerFunc {
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

func escalateToPeerBlocklist(userRepo BlockUserFetcher, nodeRepo PeerBlocklister, blockeeId string) {
	if userRepo == nil || nodeRepo == nil {
		return
	}
	u, err := userRepo.Get(blockeeId)
	switch {
	case errors.Is(err, database.ErrUserNotFound):
		log.Warnf("block: target user %s not yet known locally, peer block skipped", blockeeId)
		return
	case err != nil:
		log.Warnf("block: lookup user %s: %v", blockeeId, err)
		return
	case u.NodeId == "":
		log.Warnf("block: target user %s has no node id, peer block skipped", blockeeId)
		return
	case fediverse.IsBridged(u.Network) || u.NodeId == fediverse.GatewayNodeID():
		log.Infof("block: target user %s is bridged, gateway node %s stays unblocked", blockeeId, u.NodeId)
		return
	}
	if err := nodeRepo.BlocklistPermanent(u.NodeId); err != nil {
		log.Warnf("block: peer blocklist for %s (%s): %v", blockeeId, u.NodeId, err)
	}
}

func removePeerBlocklist(userRepo BlockUserFetcher, nodeRepo PeerBlocklister, blockeeId string) {
	if userRepo == nil || nodeRepo == nil {
		return
	}
	u, err := userRepo.Get(blockeeId)
	switch {
	case errors.Is(err, database.ErrUserNotFound):
		return
	case err != nil:
		log.Warnf("unblock: lookup user %s: %v", blockeeId, err)
		return
	case u.NodeId == "":
		return
	case fediverse.IsBridged(u.Network) || u.NodeId == fediverse.GatewayNodeID():
		return
	}
	if err := nodeRepo.BlocklistRemove(u.NodeId); err != nil {
		log.Warnf("unblock: peer blocklist remove for %s (%s): %v", blockeeId, u.NodeId, err)
	}
}

func unfollowBlockee(followRepo BlockFollowStorer, broadcaster BlockUnsubscriber, blockerId, blockeeId string) error {
	if followRepo.IsFollowing(blockerId, blockeeId) {
		if err := followRepo.Unfollow(blockerId, blockeeId); err != nil {
			return err
		}
		if err := broadcaster.UnsubscribeUserUpdate(blockeeId); err != nil {
			log.Infof("block: unsubscribe from %s: %v", blockeeId, err)
		}
	}
	if followRepo.IsFollower(blockerId, blockeeId) {
		return followRepo.Unfollow(blockeeId, blockerId)
	}
	return nil
}

func deleteBlockeeTweets(tweetRepo BlockTweetDeleter, timelineRepo BlockTimelineDeleter, blockerId, blockeeId string) error {
	var (
		ids    []string
		cursor string
		limit  = uint64(20)
	)
	for {
		tweets, cur, err := tweetRepo.List(blockeeId, &limit, &cursor)
		if err != nil {
			return err
		}
		for _, tweet := range tweets {
			isRetweetedByBlockee := tweet.RetweetedBy != nil && *tweet.RetweetedBy == blockeeId
			if tweet.UserId == blockeeId || isRetweetedByBlockee {
				ids = append(ids, tweet.Id)
			}
		}
		if uint64(len(tweets)) < limit {
			break
		}
		cursor = cur
	}

	for _, id := range ids {
		if err := timelineRepo.DeleteTweetFromTimeline(blockerId, id); err != nil {
			return err
		}
		if err := tweetRepo.Delete(blockeeId, id); err != nil {
			return err
		}
	}
	return nil
}
